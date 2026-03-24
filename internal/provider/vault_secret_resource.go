// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &VaultSecretResource{}
	_ resource.ResourceWithImportState = &VaultSecretResource{}
)

func NewVaultSecretResource() resource.Resource {
	return &VaultSecretResource{}
}

// VaultSecretResource defines the resource implementation.
type VaultSecretResource struct {
	client *api.ClientWithResponses
}

// VaultSecretResourceModel describes the resource data model.
type VaultSecretResourceModel struct {
	Id          types.String `tfsdk:"id"`
	ProjectRef  types.String `tfsdk:"project_ref"`
	Value       types.String `tfsdk:"value"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (r *VaultSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_secret"
}

func (r *VaultSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Vault Secret resource for storing encrypted secrets using Supabase Vault",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID identifier for the vault secret",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The secret value to be encrypted and stored",
				Required:            true,
				Sensitive:           true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the secret",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the secret",
				Optional:            true,
			},
		},
	}
}

func (r *VaultSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		r.client = client
	}
}

func (r *VaultSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VaultSecretResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build SQL query with escaped values
	value := escapeSQLLiteralValue(data.Value.ValueString())
	name := escapeSQLLiteralValue(data.Name.ValueString())
	description := escapeSQLLiteral(data.Description.ValueStringPointer())

	query := fmt.Sprintf("SELECT vault.create_secret(%s, %s, %s)", value, name, description)

	tflog.Debug(ctx, "Creating vault secret", map[string]interface{}{
		"project_ref": data.ProjectRef.ValueString(),
	})

	// Execute SQL query
	httpResp, err := r.client.V1RunAQueryWithResponse(ctx, data.ProjectRef.ValueString(), api.V1RunAQueryJSONRequestBody{
		Query: query,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to create vault secret: %s", err.Error()),
		)
		return
	}

	if httpResp.StatusCode() >= 400 {
		resp.Diagnostics.AddError(
			"API Error",
			fmt.Sprintf("Unable to create vault secret, status code: %d, body: %s",
				httpResp.StatusCode(), string(httpResp.Body)),
		)
		return
	}

	// Parse response to extract UUID
	// Try array-of-objects format first (TypeScript client format)
	var resultObjects []map[string]interface{}
	if err := json.Unmarshal(httpResp.Body, &resultObjects); err == nil && len(resultObjects) > 0 {
		// Extract UUID from first row - the column name is "create_secret"
		var uuid string
		for _, v := range resultObjects[0] {
			if s, ok := v.(string); ok {
				uuid = s
				break
			}
		}
		if uuid != "" {
			data.Id = types.StringValue(uuid)
		} else {
			resp.Diagnostics.AddError(
				"Parse Error",
				"Unable to extract UUID from create_secret response",
			)
			return
		}
	} else {
		// Fall back to array-of-arrays format (actual API format)
		var resultArrays [][]interface{}
		if err := json.Unmarshal(httpResp.Body, &resultArrays); err != nil {
			resp.Diagnostics.AddError(
				"Parse Error",
				fmt.Sprintf("Unable to parse create_secret response: %s", err.Error()),
			)
			return
		}

		if len(resultArrays) == 0 || len(resultArrays[0]) == 0 {
			resp.Diagnostics.AddError(
				"API Error",
				"create_secret returned empty result",
			)
			return
		}

		// Extract UUID from first row, first column
		uuid, ok := resultArrays[0][0].(string)
		if !ok {
			resp.Diagnostics.AddError(
				"Parse Error",
				fmt.Sprintf("Expected UUID string, got: %T", resultArrays[0][0]),
			)
			return
		}

		data.Id = types.StringValue(uuid)
	}

	tflog.Debug(ctx, "Created vault secret", map[string]interface{}{
		"id": data.Id.ValueString(),
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VaultSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VaultSecretResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build SQL query to read decrypted secret
	id := escapeSQLLiteralValue(data.Id.ValueString())
	query := fmt.Sprintf("SELECT decrypted_secret FROM vault.decrypted_secrets WHERE id = %s", id)

	tflog.Debug(ctx, "Reading vault secret", map[string]interface{}{
		"id": data.Id.ValueString(),
	})

	// Execute SQL query
	httpResp, err := r.client.V1RunAQueryWithResponse(ctx, data.ProjectRef.ValueString(), api.V1RunAQueryJSONRequestBody{
		Query: query,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read vault secret: %s", err.Error()),
		)
		return
	}

	if httpResp.StatusCode() >= 400 {
		resp.Diagnostics.AddError(
			"API Error",
			fmt.Sprintf("Unable to read vault secret, status code: %d, body: %s",
				httpResp.StatusCode(), string(httpResp.Body)),
		)
		return
	}

	// Parse response
	// Try array-of-objects format first (TypeScript client format)
	var resultObjects []map[string]interface{}
	if err := json.Unmarshal(httpResp.Body, &resultObjects); err == nil {
		// If no rows returned, the secret was deleted
		if len(resultObjects) == 0 {
			resp.State.RemoveResource(ctx)
			return
		}

		// Extract decrypted secret value from the "decrypted_secret" column
		secretValue, ok := resultObjects[0]["decrypted_secret"].(string)
		if !ok {
			resp.Diagnostics.AddError(
				"Parse Error",
				fmt.Sprintf("Expected secret value string in 'decrypted_secret' column, got: %T", resultObjects[0]["decrypted_secret"]),
			)
			return
		}
		data.Value = types.StringValue(secretValue)
	} else {
		// Fall back to array-of-arrays format (actual API format)
		var resultArrays [][]interface{}
		if err := json.Unmarshal(httpResp.Body, &resultArrays); err != nil {
			resp.Diagnostics.AddError(
				"Parse Error",
				fmt.Sprintf("Unable to parse read response: %s", err.Error()),
			)
			return
		}

		// If no rows returned, the secret was deleted
		if len(resultArrays) == 0 {
			resp.State.RemoveResource(ctx)
			return
		}

		if len(resultArrays[0]) == 0 {
			resp.Diagnostics.AddError(
				"API Error",
				"Read query returned empty row",
			)
			return
		}

		// Extract decrypted secret value
		secretValue, ok := resultArrays[0][0].(string)
		if !ok {
			resp.Diagnostics.AddError(
				"Parse Error",
				fmt.Sprintf("Expected secret value string, got: %T", resultArrays[0][0]),
			)
			return
		}
		data.Value = types.StringValue(secretValue)
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VaultSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data VaultSecretResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build SQL query with escaped values
	id := escapeSQLLiteralValue(data.Id.ValueString())
	value := escapeSQLLiteralValue(data.Value.ValueString())
	name := escapeSQLLiteralValue(data.Name.ValueString())
	description := escapeSQLLiteral(data.Description.ValueStringPointer())

	query := fmt.Sprintf("SELECT vault.update_secret(%s, %s, %s, %s)", id, value, name, description)

	tflog.Debug(ctx, "Updating vault secret", map[string]interface{}{
		"id": data.Id.ValueString(),
	})

	// Execute SQL query
	httpResp, err := r.client.V1RunAQueryWithResponse(ctx, data.ProjectRef.ValueString(), api.V1RunAQueryJSONRequestBody{
		Query: query,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to update vault secret: %s", err.Error()),
		)
		return
	}

	if httpResp.StatusCode() >= 400 {
		resp.Diagnostics.AddError(
			"API Error",
			fmt.Sprintf("Unable to update vault secret, status code: %d, body: %s",
				httpResp.StatusCode(), string(httpResp.Body)),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VaultSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VaultSecretResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build SQL query with escaped ID
	id := escapeSQLLiteralValue(data.Id.ValueString())
	query := fmt.Sprintf("DELETE FROM vault.secrets WHERE id = %s", id)

	tflog.Debug(ctx, "Deleting vault secret", map[string]interface{}{
		"id": data.Id.ValueString(),
	})

	// Execute SQL query
	httpResp, err := r.client.V1RunAQueryWithResponse(ctx, data.ProjectRef.ValueString(), api.V1RunAQueryJSONRequestBody{
		Query: query,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to delete vault secret: %s", err.Error()),
		)
		return
	}

	if httpResp.StatusCode() >= 400 {
		resp.Diagnostics.AddError(
			"API Error",
			fmt.Sprintf("Unable to delete vault secret, status code: %d, body: %s",
				httpResp.StatusCode(), string(httpResp.Body)),
		)
		return
	}
}

func (r *VaultSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: project_ref:secret_id
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import ID in format 'project_ref:secret_id', got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_ref"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// escapeSQLLiteral escapes a string pointer for safe use in SQL queries.
// Returns 'NULL' for nil pointers, or a properly escaped and quoted string literal.
// This function doubles single quotes to prevent SQL injection.
func escapeSQLLiteral(s *string) string {
	if s == nil {
		return "NULL"
	}
	value := *s
	// Escape single quotes by doubling them (PostgreSQL standard)
	escaped := strings.ReplaceAll(value, "'", "''")
	return fmt.Sprintf("'%s'", escaped)
}

// escapeSQLLiteralValue escapes a non-pointer string value for safe use in SQL queries.
// Returns a properly escaped and quoted string literal.
func escapeSQLLiteralValue(s string) string {
	// Escape single quotes by doubling them (PostgreSQL standard)
	escaped := strings.ReplaceAll(s, "'", "''")
	return fmt.Sprintf("'%s'", escaped)
}
