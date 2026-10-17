// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &VaultSecretResource{}
	_ resource.ResourceWithImportState = &VaultSecretResource{}
	_ planmodifier.String              = nullIfEmptyStringModifier{}
)

// SQL follows github.com/supabase/cli/pkg/vault.UpsertVaultSecrets.
// Description is included because vault.create_secret and vault.update_secret accept it.
// The CLI helper leaves description at the function default.
const (
	createVaultSecretSQL     = "SELECT vault.create_secret($1, $2, $3) AS id"                                                                //nolint:gosec // G101: SQL text, not a credential.
	readVaultSecretByIDSQL   = "SELECT id::text AS id, name, description, decrypted_secret FROM vault.decrypted_secrets WHERE id = $1::uuid" //nolint:gosec // G101: SQL text, not a credential.
	readVaultSecretByNameSQL = "SELECT id::text AS id, name, description, decrypted_secret FROM vault.decrypted_secrets WHERE name = $1"     //nolint:gosec // G101: SQL text, not a credential.
	updateVaultSecretSQL     = "SELECT vault.update_secret($1::uuid, $2, $3, $4)"                                                            //nolint:gosec // G101: SQL text, not a credential.
	// Vault has no delete function (https://github.com/supabase/vault/issues/32).
	// The SELECT keeps the response a JSON row array. A bare DELETE can come back as a command tag.
	deleteVaultSecretSQL = "WITH deleted AS (DELETE FROM vault.secrets WHERE id = $1::uuid RETURNING id) SELECT id::text AS id FROM deleted" //nolint:gosec // G101: SQL text, not a credential.
)

// nullIfEmptyStringModifier stores an empty description as null so an omitted
// value and a blank database description stay in sync.
type nullIfEmptyStringModifier struct{}

func (m nullIfEmptyStringModifier) Description(context.Context) string {
	return "Treats an empty string as null."
}

func (m nullIfEmptyStringModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m nullIfEmptyStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if req.PlanValue.ValueString() == "" {
		resp.PlanValue = types.StringNull()
	}
}

func NewVaultSecretResource() resource.Resource {
	return &VaultSecretResource{}
}

type VaultSecretResource struct {
	client *api.ClientWithResponses
}

type VaultSecretResourceModel struct {
	ProjectRef  types.String `tfsdk:"project_ref"`
	Name        types.String `tfsdk:"name"`
	Value       types.String `tfsdk:"value"`
	Description types.String `tfsdk:"description"`
	Id          types.String `tfsdk:"id"`
}

func (r *VaultSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_secret"
}

func (r *VaultSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a secret in Supabase Vault. " +
			"Create and update run `vault.create_secret` and `vault.update_secret` through the Management API database query endpoint, following `UpsertVaultSecrets` in the Supabase CLI package. " +
			"Destroy deletes the `vault.secrets` row. Vault's SQL API exposes create and update only.",
		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID. Changing this recreates the secret.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Unique secret name. Changing this updates the existing secret in place.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "Plaintext secret passed to `vault.create_secret` and `vault.update_secret`. Stored in Terraform state and refreshed from `vault.decrypted_secrets`.",
				Required:            true,
				Sensitive:           true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description stored with the secret. A blank description is treated as unset.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					nullIfEmptyStringModifier{},
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Vault secret UUID returned by `vault.create_secret`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rows, projectNotFound, diags := runDatabaseQuery(ctx, r.client, data.ProjectRef.ValueString(), createVaultSecretSQL, []any{
		data.Value.ValueString(),
		data.Name.ValueString(),
		descriptionParam(data.Description),
	})
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(projectNotFoundError(data.ProjectRef.ValueString(), projectNotFound)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, diags := vaultCreateSecretID(rows)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Id = types.StringValue(id)

	tflog.Trace(ctx, "created vault secret", map[string]any{
		"project_ref": data.ProjectRef.ValueString(),
		"id":          id,
		"name":        data.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VaultSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VaultSecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, diags := readVaultSecretByID(ctx, r.client, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	tflog.Trace(ctx, "read vault secret", map[string]any{
		"project_ref": data.ProjectRef.ValueString(),
		"id":          data.Id.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VaultSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data VaultSecretResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, projectNotFound, diags := runDatabaseQuery(ctx, r.client, data.ProjectRef.ValueString(), updateVaultSecretSQL, []any{
		data.Id.ValueString(),
		data.Value.ValueString(),
		data.Name.ValueString(),
		descriptionParam(data.Description),
	})
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(projectNotFoundError(data.ProjectRef.ValueString(), projectNotFound)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated vault secret", map[string]any{
		"project_ref": data.ProjectRef.ValueString(),
		"id":          data.Id.ValueString(),
		"name":        data.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VaultSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VaultSecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, projectNotFound, diags := runDatabaseQuery(ctx, r.client, data.ProjectRef.ValueString(), deleteVaultSecretSQL, []any{
		data.Id.ValueString(),
	})
	if projectNotFound {
		tflog.Trace(ctx, "project not found, vault secret already gone", map[string]any{
			"project_ref": data.ProjectRef.ValueString(),
			"id":          data.Id.ValueString(),
		})
		return
	}
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "deleted vault secret", map[string]any{
		"project_ref": data.ProjectRef.ValueString(),
		"id":          data.Id.ValueString(),
	})
}

func (r *VaultSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectRef, secretRef, importDiag := parseVaultSecretImportID(req.ID)
	if importDiag != nil {
		resp.Diagnostics.Append(importDiag)
		return
	}

	data := VaultSecretResourceModel{
		ProjectRef: types.StringValue(projectRef),
	}

	var found bool
	var diags diag.Diagnostics
	if uuid.Validate(secretRef) == nil {
		data.Id = types.StringValue(secretRef)
		found, diags = readVaultSecretByID(ctx, r.client, &data)
	} else {
		found, diags = readVaultSecretByName(ctx, r.client, &data, secretRef)
	}
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Resource Not Found",
			fmt.Sprintf("No vault secret found for import ID %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func descriptionParam(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

func parseVaultSecretImportID(importID string) (string, string, diag.Diagnostic) {
	projectRef, secretRef, ok := strings.Cut(strings.TrimSpace(importID), "/")
	projectRef = strings.TrimSpace(projectRef)
	secretRef = strings.TrimSpace(secretRef)
	if !ok || projectRef == "" || secretRef == "" {
		return "", "", diag.NewErrorDiagnostic(
			"Unexpected Import Identifier",
			"Import a vault secret with the project reference and either the secret name or its UUID, separated by a slash.\n\n"+
				"Example: myprojectref/stripe_secret_key\n"+
				"Example: myprojectref/7095d222-efe5-4cd5-b5c6-5755b451e223",
		)
	}
	return projectRef, secretRef, nil
}

func readVaultSecretByID(ctx context.Context, client *api.ClientWithResponses, data *VaultSecretResourceModel) (bool, diag.Diagnostics) {
	return readVaultSecret(ctx, client, data, readVaultSecretByIDSQL, data.Id.ValueString())
}

func readVaultSecretByName(ctx context.Context, client *api.ClientWithResponses, data *VaultSecretResourceModel, name string) (bool, diag.Diagnostics) {
	return readVaultSecret(ctx, client, data, readVaultSecretByNameSQL, name)
}

func readVaultSecret(ctx context.Context, client *api.ClientWithResponses, data *VaultSecretResourceModel, query string, arg any) (bool, diag.Diagnostics) {
	rows, projectNotFound, diags := runDatabaseQuery(ctx, client, data.ProjectRef.ValueString(), query, []any{arg})
	if projectNotFound {
		tflog.Trace(ctx, "project not found, dropping vault secret", map[string]any{
			"project_ref": data.ProjectRef.ValueString(),
		})
		return false, nil
	}
	if diags.HasError() {
		return false, diags
	}

	row, found, diags := oneVaultRow(rows)
	if diags.HasError() || !found {
		return false, diags
	}
	return true, applyVaultSecretRow(data, row)
}

func oneVaultRow(rows []map[string]any) (map[string]any, bool, diag.Diagnostics) {
	switch len(rows) {
	case 0:
		return nil, false, nil
	case 1:
		return rows[0], true, nil
	default:
		return nil, false, diag.Diagnostics{diag.NewErrorDiagnostic(
			"API Error",
			fmt.Sprintf("Expected one vault secret, got %d rows.", len(rows)),
		)}
	}
}

func applyVaultSecretRow(data *VaultSecretResourceModel, row map[string]any) diag.Diagnostics {
	id, ok := jsonString(row, "id")
	if !ok || id == "" {
		return diag.Diagnostics{diag.NewErrorDiagnostic("API Error", "Vault secret query did not return an id.")}
	}
	secret, ok := jsonString(row, "decrypted_secret")
	if !ok {
		return diag.Diagnostics{diag.NewErrorDiagnostic("API Error", "Vault secret query did not return decrypted_secret.")}
	}

	name, ok := jsonString(row, "name")
	if !ok || name == "" {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"API Error",
			"Vault secret has no name. This resource can only manage named secrets.",
		)}
	}

	data.Id = types.StringValue(id)
	data.Name = types.StringValue(name)
	if description, ok := jsonString(row, "description"); ok && description != "" {
		data.Description = types.StringValue(description)
	} else {
		data.Description = types.StringNull()
	}
	data.Value = types.StringValue(secret)
	return nil
}

func vaultCreateSecretID(rows []map[string]any) (string, diag.Diagnostics) {
	row, found, diags := oneVaultRow(rows)
	if diags.HasError() {
		return "", diags
	}
	if !found {
		return "", diag.Diagnostics{diag.NewErrorDiagnostic("API Error", "vault.create_secret did not return a secret id.")}
	}
	if id, ok := jsonString(row, "id"); ok && id != "" {
		return id, nil
	}
	if id, ok := jsonString(row, "create_secret"); ok && id != "" {
		return id, nil
	}
	return "", diag.Diagnostics{diag.NewErrorDiagnostic("API Error", "vault.create_secret did not return a secret id.")}
}

func jsonString(row map[string]any, key string) (string, bool) {
	value, ok := row[key]
	if !ok || value == nil {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func projectNotFoundError(projectRef string, projectNotFound bool) diag.Diagnostics {
	if !projectNotFound {
		return nil
	}
	return diag.Diagnostics{diag.NewErrorDiagnostic(
		"API Error",
		fmt.Sprintf("Project %q was not found.", projectRef),
	)}
}

func runDatabaseQuery(ctx context.Context, client *api.ClientWithResponses, projectRef, query string, parameters []any) ([]map[string]any, bool, diag.Diagnostics) {
	// V1RunQueryBody.Parameters matches POST /v1/projects/{ref}/database/query.
	// Placeholders stay in the SQL so secret values are bound, not interpolated.
	body := api.V1RunQueryBody{Query: query}
	if len(parameters) > 0 {
		params := append([]any{}, parameters...)
		body.Parameters = &params
	}

	// Query text uses placeholders. Parameters are omitted so secret values stay out of logs.
	tflog.Debug(ctx, "running project database query", map[string]any{
		"project_ref": projectRef,
		"query":       query,
	})

	httpResp, err := client.V1RunAQueryWithResponse(ctx, projectRef, body)
	if err != nil {
		msg := fmt.Sprintf("Unable to query project database, got error: %s", err)
		return nil, false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	switch httpResp.StatusCode() {
	case http.StatusNotFound:
		// A deleted project is an orphan, matching the other project-scoped resources.
		return nil, true, nil
	case http.StatusOK, http.StatusCreated:
	default:
		msg := fmt.Sprintf("Unable to query project database, got status %d: %s", httpResp.StatusCode(), truncateBody(httpResp.Body))
		return nil, false, diag.Diagnostics{diag.NewErrorDiagnostic("API Error", msg)}
	}

	trimmed := strings.TrimSpace(string(httpResp.Body))
	if trimmed == "" || trimmed == "null" {
		return []map[string]any{}, false, nil
	}

	var rows []map[string]any
	if err := json.Unmarshal(httpResp.Body, &rows); err != nil {
		msg := fmt.Sprintf("Unable to parse project database query response, got status %d: %s", httpResp.StatusCode(), truncateBody(httpResp.Body))
		return nil, false, diag.Diagnostics{diag.NewErrorDiagnostic("API Error", msg)}
	}
	return rows, false, nil
}

func truncateBody(body []byte) string {
	const maxBody = 512
	if len(body) <= maxBody {
		return string(body)
	}
	return string(body[:maxBody]) + "..."
}
