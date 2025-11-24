// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &APIKeyResource{}
var _ resource.ResourceWithImportState = &APIKeyResource{}

func NewApiKeyResource() resource.Resource {
	return &APIKeyResource{}
}

// APIKeysDataSource defines the data source implementation.
type APIKeyResource struct {
	client *api.ClientWithResponses
}

var secretJwtTemplateAttrTypes = map[string]attr.Type{
	"role": types.StringType,
}

type ApiKeyDatabaseModel struct {
	Id                types.String `tfsdk:"id"`
	ProjectRef        types.String `tfsdk:"project_ref"`
	ApiKey            types.String `tfsdk:"api_key"`
	SecretJwtTemplate types.Object `tfsdk:"secret_jwt_template"`
	Name              types.String `tfsdk:"name"`
	Type              types.String `tfsdk:"type"`
	Description       types.String `tfsdk:"description"`
}

func (m ApiKeyDatabaseModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"role": types.StringType,
	}
}

// APIKeysDataSourceModel describes the data source data model.
type ApiKeyResourceModel struct {
	ProjectRef        types.String `tfsdk:"project_ref"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	Type              types.String `tfsdk:"type"`
	ApiKey            types.String `tfsdk:"api_key"`
	SecretJwtTemplate types.Object `tfsdk:"secret_jwt_template"`
	Id                types.String `tfsdk:"id"`
}

func (d *APIKeyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apikey"
}

func (d *APIKeyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "API Key resource",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "API key identifier",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the API key",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the API key",
				Optional:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Type of the API key",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "API key",
				Computed:            true,
				Sensitive:           true,
			},
			"secret_jwt_template": schema.SingleNestedAttribute{
				MarkdownDescription: "Secret JWT template",
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"role": schema.StringAttribute{
						MarkdownDescription: "Role of the secret JWT template",
						Computed:            true,
					},
				},
			},
		},
	}
}

func (d *APIKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *api.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (r *APIKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ApiKeyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(createApiKey(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *APIKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ApiKeyResourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(readApiKey(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *APIKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ApiKeyResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(updateApiKey(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *APIKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			`Expected import identifier in the format "project_ref/api_key_id".`,
		)
		return
	}

	projectRef := strings.TrimSpace(parts[0])
	apiKeyID := strings.TrimSpace(parts[1])
	if projectRef == "" || apiKeyID == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			"Both project_ref and api_key_id must be provided when importing. Example: myprojectref/3603c575-...",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_ref"), types.StringValue(projectRef))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(apiKeyID))...)
}

func (r *APIKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ApiKeyResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deleteApiKey(ctx, &data, r.client)...)
}

func readApiKey(ctx context.Context, state *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	return readApiKeyDatabase(ctx, state, client)
}

func readApiKeyDatabase(ctx context.Context, state *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1GetProjectApiKeyWithResponse(ctx, state.ProjectRef.ValueString(), uuid.MustParse(state.Id.ValueString()), &api.V1GetProjectApiKeyParams{Reveal: Ptr(true)})
	if err != nil {
		msg := fmt.Sprintf("Unable to read apiKey database, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read apiKey database, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	idValue := NullableToString(httpResp.JSON200.Id)
	apiKeyValue := NullableToString(httpResp.JSON200.ApiKey)
	typeValue := NullableEnumToString(httpResp.JSON200.Type)
	descriptionValue := NullableToString(httpResp.JSON200.Description)

	database := ApiKeyDatabaseModel{
		Id:          idValue,
		ApiKey:      apiKeyValue,
		Name:        types.StringValue(httpResp.JSON200.Name),
		Type:        typeValue,
		Description: descriptionValue,
	}

	var secretJwtTemplate types.Object
	if httpResp.JSON200.SecretJwtTemplate.IsSpecified() && !httpResp.JSON200.SecretJwtTemplate.IsNull() {
		templateMap := httpResp.JSON200.SecretJwtTemplate.MustGet()
		roleValue := ""
		if role, ok := templateMap["role"].(string); ok {
			roleValue = role
		}
		obj, diags := types.ObjectValue(secretJwtTemplateAttrTypes, map[string]attr.Value{
			"role": types.StringValue(roleValue),
		})
		if diags.HasError() {
			return diags
		}
		secretJwtTemplate = obj
	} else {
		obj, diags := types.ObjectValue(secretJwtTemplateAttrTypes, map[string]attr.Value{
			"role": types.StringNull(),
		})
		if diags.HasError() {
			return diags
		}
		secretJwtTemplate = obj
	}

	database.SecretJwtTemplate = secretJwtTemplate
	state.Id = database.Id
	state.ApiKey = database.ApiKey
	state.Name = database.Name
	state.Type = database.Type
	state.Description = database.Description
	state.SecretJwtTemplate = database.SecretJwtTemplate
	return nil
}

func createApiKey(ctx context.Context, plan *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	reveal := Ptr(true)
	resp, err := client.V1GetProjectApiKeysWithResponse(ctx, plan.ProjectRef.ValueString(), &api.V1GetProjectApiKeysParams{Reveal: reveal})
	if err != nil {
		msg := fmt.Sprintf("Unable to read apiKeys, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	// 1. Check if default publishable key exist, create it if it doesn't
	hasDefaultPublishable := false

	if resp.JSON200 != nil {
		for _, key := range *resp.JSON200 {
			if key.Name == "default" {
				if key.Type.IsSpecified() && !key.Type.IsNull() {
					keyType := key.Type.MustGet()
					if keyType == api.ApiKeyResponseTypePublishable {
						hasDefaultPublishable = true
					}
				}
			}
		}
	}

	if !hasDefaultPublishable {
		httpRespDefaultPublishable, errDefaultPublishable := client.V1CreateProjectApiKeyWithResponse(ctx, plan.ProjectRef.ValueString(), &api.V1CreateProjectApiKeyParams{Reveal: reveal}, api.CreateApiKeyBody{
			Name:              "default",
			Type:              api.CreateApiKeyBodyTypePublishable,
			Description:       nullable.Nullable[string]{},
			SecretJwtTemplate: nullable.Nullable[map[string]interface{}]{},
		})
		if errDefaultPublishable != nil {
			msg := fmt.Sprintf("Unable to create default publishable apiKey, got error: %s", errDefaultPublishable)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}
		if httpRespDefaultPublishable.JSON201 == nil {
			msg := fmt.Sprintf("Unable to create default publishable apiKey, got status %d: %s", httpRespDefaultPublishable.StatusCode(), httpRespDefaultPublishable.Body)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}
	}

	// 2. Create apiKey
	httpResp, err := client.V1CreateProjectApiKeyWithResponse(ctx, plan.ProjectRef.ValueString(), &api.V1CreateProjectApiKeyParams{Reveal: reveal}, api.CreateApiKeyBody{
		Name:              plan.Name.ValueString(),
		Type:              api.CreateApiKeyBodyTypeSecret,
		Description:       nullable.Nullable[string]{},
		SecretJwtTemplate: nullable.NewNullableWithValue(map[string]interface{}{"role": "service_role"}),
	})

	if err != nil {
		msg := fmt.Sprintf("Unable to create apiKey, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON201 == nil {
		msg := fmt.Sprintf("Unable to create apiKey, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	// Update computed fields from creation response
	plan.Id = NullableToString(httpResp.JSON201.Id)
	plan.ApiKey = NullableToString(httpResp.JSON201.ApiKey)
	plan.Type = NullableEnumToString(httpResp.JSON201.Type)

	obj, diags := types.ObjectValue(secretJwtTemplateAttrTypes, map[string]attr.Value{
		"role": types.StringValue("service_role"),
	})
	if diags.HasError() {
		return diags
	}
	plan.SecretJwtTemplate = obj

	return readApiKeyDatabase(ctx, plan, client)
}

func updateApiKey(ctx context.Context, plan *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	var secretJwtTemplate nullable.Nullable[map[string]interface{}]
	if plan.Type.ValueString() == string(api.ApiKeyResponseTypeSecret) {
		secretJwtTemplate = nullable.NewNullableWithValue(map[string]interface{}{"role": "service_role"})
	} else {
		secretJwtTemplate = nullable.Nullable[map[string]interface{}]{}
	}

	var description nullable.Nullable[string]
	if plan.Description.IsNull() || plan.Description.IsUnknown() {
		description = nullable.Nullable[string]{}
	} else {
		description = nullable.NewNullableWithValue(plan.Description.ValueString())
	}

	httpResp, err := client.V1UpdateProjectApiKeyWithResponse(ctx, plan.ProjectRef.ValueString(), uuid.MustParse(plan.Id.ValueString()), &api.V1UpdateProjectApiKeyParams{Reveal: Ptr(true)}, api.UpdateApiKeyBody{
		Name:              plan.Name.ValueStringPointer(),
		Description:       description,
		SecretJwtTemplate: secretJwtTemplate,
	})

	if err != nil {
		msg := fmt.Sprintf("Unable to update apiKey, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to update apiKey, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	plan.Description = NullableToString(httpResp.JSON200.Description)

	return readApiKeyDatabase(ctx, plan, client)
}

func deleteApiKey(ctx context.Context, state *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1DeleteProjectApiKeyWithResponse(ctx, state.ProjectRef.ValueString(), uuid.MustParse(state.Id.ValueString()), &api.V1DeleteProjectApiKeyParams{Reveal: Ptr(true)})
	if err != nil {
		msg := fmt.Sprintf("Unable to delete apiKey, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to delete apiKey, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}
