// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &APIKeyResource{}
	_ resource.ResourceWithImportState = &APIKeyResource{}
	_ resource.ResourceWithModifyPlan  = &APIKeyResource{}
)

func NewApiKeyResource() resource.Resource {
	return &APIKeyResource{}
}

// APIKeysDataSource defines the data source implementation.
type APIKeyResource struct {
	client *api.ClientWithResponses
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

// Role a new secret key uses when secret_jwt_template.role is not configured.
const defaultSecretJwtRole = "service_role"

var secretJwtTemplateAttrTypes = map[string]attr.Type{
	"role": types.StringType,
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
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-z_][a-z0-9_]*$`),
						"Name must start with a lowercase letter or an underscore, followed only by lowercase alphanumeric characters or underscore",
					),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the API key. When not set, the key keeps its current description.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
				MarkdownDescription: "JWT template of a secret API key. Requests made with the key use the Postgres `role` set here.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"role": schema.StringAttribute{
						MarkdownDescription: "Postgres role that requests made with the key run as, such as a custom role created in a migration. A new key uses `" + defaultSecretJwtRole + "` when this is not set. Only secret keys accept a role. Changing a configured role replaces the key; removing it from configuration keeps the current role.",
						Optional:            true,
						Computed:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							stringplanmodifier.RequiresReplaceIf(
								requiresReplaceSecretJwtRole,
								"Changing a configured role replaces the key.",
								"Changing a configured role replaces the key.",
							),
						},
					},
				},
			},
		},
	}
}

// Only a configured role that differs from a known role replaces the key. A key imported without a
// role in config keeps its role, and a key whose role isn't in state yet (state written by an
// earlier provider version, not refreshed since) isn't replaced on that alone.
func requiresReplaceSecretJwtRole(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = !req.ConfigValue.IsNull() && !req.StateValue.IsNull()
}

// ModifyPlan rejects a role on a publishable key: the provider only creates secret keys, so
// replacing an imported publishable key to apply a role would delete it.
func (r *APIKeyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var keyType types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("type"), &keyType)...)
	var configTemplate types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("secret_jwt_template"), &configTemplate)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateSecretJwtRoleForType(keyType, secretJwtRoleFromObject(configTemplate)); err != "" {
		resp.Diagnostics.AddAttributeError(path.Root("secret_jwt_template").AtName("role"), "Invalid Attribute Configuration", err)
	}
}

func validateSecretJwtRoleForType(keyType, configRole types.String) string {
	if configRole.IsNull() || keyType.IsNull() || keyType.IsUnknown() {
		return ""
	}
	if keyType.ValueString() == string(api.ApiKeyResponseTypeSecret) {
		return ""
	}
	return fmt.Sprintf("secret_jwt_template.role can only be set on a secret API key, but this key is of type %q. Remove secret_jwt_template from the configuration.", keyType.ValueString())
}

func (d *APIKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		d.client = client
	}
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

	found, diags := readApiKey(ctx, &data, r.client)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The key was deleted outside Terraform, so the next plan creates it again.
	if !found {
		resp.State.RemoveResource(ctx)
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
	projectRef, apiKeyID, diag := resolveAPIKeyImportID(ctx, r.client, req.ID)
	if diag != nil {
		resp.Diagnostics.Append(diag)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_ref"), types.StringValue(projectRef))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(apiKeyID))...)
}

func resolveAPIKeyImportID(ctx context.Context, client *api.ClientWithResponses, importID string) (projectRef, apiKeyID string, _ diag.Diagnostic) {
	parts := strings.Split(importID, "/")
	switch len(parts) {
	case 3:
		projectRef, keyName, keyType := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
		apiKeyID, diag := importAPIKeyByNameAndType(ctx, client, projectRef, keyName, keyType)
		return projectRef, apiKeyID, diag
	case 2:
		projectRef, keyRef := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		apiKeyID, diag := importAPIKeyByNameOrID(ctx, client, projectRef, keyRef)
		return projectRef, apiKeyID, diag
	default:
		return "", "", diag.NewErrorDiagnostic(
			"Unexpected Import Identifier",
			`This resource supports multiple identifier formats.

Provide a project reference and the name of the API key.
Example: myprojectref/myprojectkey

If multiple keys in the project use the same name, a type of key to import must also be provided.
Example: myprojectref/myprojectkey/publishable or myprojectref/myprojectkey/secret

Alternatively, a project reference and a UUID of the API key can be used.
Example: myprojectref/00000000-0000-0000-0000-000000000000`,
		)
	}
}

func importAPIKeyByNameOrID(ctx context.Context, client *api.ClientWithResponses, projectRef, keyRef string) (string, diag.Diagnostic) {
	if projectRef == "" || keyRef == "" {
		return "", diag.NewErrorDiagnostic(
			"Unexpected Import Identifier",
			`Both project_ref and api_key_name must be provided when importing.
Example: myprojectref/myprojectkey

Alternatively, a project_ref and api_key_id can be specified.
Example: myprojectref/00000000-0000-0000-0000-000000000000`,
		)
	}

	if id, err := uuid.Parse(keyRef); err == nil {
		return id.String(), nil
	}

	httpResp, err := client.V1GetProjectApiKeysWithResponse(ctx, projectRef, &api.V1GetProjectApiKeysParams{})
	if err != nil {
		msg := fmt.Sprintf("Unable to read api keys, got error: %s", err)
		return "", diag.NewErrorDiagnostic("Client Error", msg)
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read api keys, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return "", diag.NewErrorDiagnostic("Client Error", msg)
	}

	foundKeyID := ""
	keyRef = strings.ToLower(keyRef)
	for _, val := range *httpResp.JSON200 {
		if !val.Id.IsSpecified() || val.Id.IsNull() {
			continue
		}
		if keyRef != val.Name {
			continue
		}

		if foundKeyID != "" {
			return "", diag.NewErrorDiagnostic(
				"Ambiguous Import Identifier",
				`Found multiple keys in the project that match the provided import identifier.
Please use a more specific identifier like myprojectref/mykeyname/type or myprojectref/00000000-0000-0000-0000-000000000000`,
			)
		}

		foundKeyID = val.Id.MustGet()
	}

	if foundKeyID == "" {
		return "", diag.NewErrorDiagnostic(
			"Import Error",
			"Did not find a key matching the provided identifier.",
		)
	}

	return foundKeyID, nil
}

func importAPIKeyByNameAndType(ctx context.Context, client *api.ClientWithResponses, projectRef, keyName, keyType string) (string, diag.Diagnostic) {
	if projectRef == "" || keyName == "" || keyType == "" {
		return "", diag.NewErrorDiagnostic(
			"Unexpected Import Identifier",
			"Both project_ref, api_key_name, and api_key_type must be provided when importing. Example: myprojectref/key-name/publishable",
		)
	}

	apiKeyType := api.ApiKeyResponseType(keyType)
	switch apiKeyType {
	case api.ApiKeyResponseTypePublishable, api.ApiKeyResponseTypeSecret:
	default:
		return "", diag.NewErrorDiagnostic(
			"Unexpected Import Identifier",
			fmt.Sprintf("Unexpected API key type provided: `%s`. Must be either publishable or secret.", keyType),
		)
	}

	httpResp, err := client.V1GetProjectApiKeysWithResponse(ctx, projectRef, &api.V1GetProjectApiKeysParams{})
	if err != nil {
		msg := fmt.Sprintf("Unable to read api keys, got error: %s", err)
		return "", diag.NewErrorDiagnostic("Client Error", msg)
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read api keys, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return "", diag.NewErrorDiagnostic("Client Error", msg)
	}

	keyName = strings.ToLower(keyName)
	for _, val := range *httpResp.JSON200 {
		if !val.Type.IsSpecified() || val.Type.IsNull() {
			continue
		}
		if !val.Id.IsSpecified() || val.Id.IsNull() {
			continue
		}

		respType := val.Type.MustGet()
		if respType != apiKeyType {
			continue
		}

		// key names in the response are always lowercase, enforced API side
		if val.Name != keyName {
			continue
		}

		return val.Id.MustGet(), nil
	}

	return "", diag.NewErrorDiagnostic("Import Error", "Specified API key wasn't found in the project.")
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

func readApiKey(ctx context.Context, state *ApiKeyResourceModel, client *api.ClientWithResponses) (bool, diag.Diagnostics) {
	return readApiKeyDatabase(ctx, state, client)
}

// The API answers 406 with this message for a key that doesn't exist.
const apiKeyNotFoundMessage = "Failed to find API key"

func isApiKeyNotFound(statusCode int, body []byte) bool {
	switch statusCode {
	case http.StatusNotFound:
		return true
	case http.StatusNotAcceptable:
		return strings.Contains(string(body), apiKeyNotFoundMessage)
	}
	return false
}

func readApiKeyDatabase(ctx context.Context, state *ApiKeyResourceModel, client *api.ClientWithResponses) (bool, diag.Diagnostics) {
	httpResp, err := client.V1GetProjectApiKeyWithResponse(ctx, state.ProjectRef.ValueString(), uuid.MustParse(state.Id.ValueString()), &api.V1GetProjectApiKeyParams{Reveal: Ptr(true)})
	if err != nil {
		msg := fmt.Sprintf("Unable to read apiKey database, got error: %s", err)
		return false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if isApiKeyNotFound(httpResp.StatusCode(), httpResp.Body) {
		tflog.Trace(ctx, fmt.Sprintf("api key not found: %s/%s", state.ProjectRef.ValueString(), state.Id.ValueString()))
		return false, nil
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read apiKey database, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	idValue := NullableToString(httpResp.JSON200.Id)
	typeValue := NullableToString(httpResp.JSON200.Type)

	role := secretJwtRoleFromResponse(httpResp.JSON200.SecretJwtTemplate)
	if typeValue.ValueString() == string(api.ApiKeyResponseTypeSecret) {
		// Reading a single key doesn't return its JWT template, so the role comes from the list of keys.
		listedRole, diags := readApiKeySecretJwtRole(ctx, state.ProjectRef.ValueString(), idValue.ValueString(), client)
		if diags.HasError() {
			return false, diags
		}
		if !listedRole.IsNull() {
			role = listedRole
		}
	}
	secretJwtTemplate, diags := types.ObjectValue(secretJwtTemplateAttrTypes, map[string]attr.Value{
		"role": role,
	})
	if diags.HasError() {
		return false, diags
	}

	state.Id = idValue
	state.ApiKey = NullableToString(httpResp.JSON200.ApiKey)
	state.Name = types.StringValue(httpResp.JSON200.Name)
	state.Type = typeValue
	state.Description = NullableToString(httpResp.JSON200.Description)
	state.SecretJwtTemplate = secretJwtTemplate
	return true, nil
}

func readApiKeySecretJwtRole(ctx context.Context, projectRef, apiKeyID string, client *api.ClientWithResponses) (types.String, diag.Diagnostics) {
	httpResp, err := client.V1GetProjectApiKeysWithResponse(ctx, projectRef, &api.V1GetProjectApiKeysParams{Reveal: Ptr(false)})
	if err != nil {
		msg := fmt.Sprintf("Unable to read apiKeys, got error: %s", err)
		return types.StringNull(), diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read apiKeys, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return types.StringNull(), diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	for _, key := range *httpResp.JSON200 {
		if key.Id.IsSpecified() && !key.Id.IsNull() && strings.EqualFold(key.Id.MustGet(), apiKeyID) {
			return secretJwtRoleFromResponse(key.SecretJwtTemplate), nil
		}
	}
	return types.StringNull(), nil
}

func secretJwtRoleFromResponse(template nullable.Nullable[map[string]interface{}]) types.String {
	if !template.IsSpecified() || template.IsNull() {
		return types.StringNull()
	}
	if role, ok := template.MustGet()["role"].(string); ok && role != "" {
		return types.StringValue(role)
	}
	return types.StringNull()
}

// Returns the role set in the plan or state, unknown when the template itself is unknown.
func secretJwtRoleFromObject(template types.Object) types.String {
	if template.IsUnknown() {
		return types.StringUnknown()
	}
	if template.IsNull() {
		return types.StringNull()
	}
	role, ok := template.Attributes()["role"].(types.String)
	if !ok {
		return types.StringNull()
	}
	return role
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
	role := defaultSecretJwtRole
	if configured := secretJwtRoleFromObject(plan.SecretJwtTemplate); !configured.IsNull() && !configured.IsUnknown() {
		role = configured.ValueString()
	}

	httpResp, err := client.V1CreateProjectApiKeyWithResponse(ctx, plan.ProjectRef.ValueString(), &api.V1CreateProjectApiKeyParams{Reveal: reveal}, api.CreateApiKeyBody{
		Name:              plan.Name.ValueString(),
		Type:              api.CreateApiKeyBodyTypeSecret,
		Description:       nullableDescription(plan.Description),
		SecretJwtTemplate: nullable.NewNullableWithValue(map[string]interface{}{"role": role}),
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
	plan.Type = NullableToString(httpResp.JSON201.Type)

	obj, diags := types.ObjectValue(secretJwtTemplateAttrTypes, map[string]attr.Value{
		"role": types.StringValue(role),
	})
	if diags.HasError() {
		return diags
	}
	plan.SecretJwtTemplate = obj

	return readApiKeyAfterWrite(ctx, plan, client)
}

func readApiKeyAfterWrite(ctx context.Context, plan *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	planned := plan.SecretJwtTemplate
	found, diags := readApiKeyDatabase(ctx, plan, client)
	if diags.HasError() {
		return diags
	}
	if !found {
		msg := fmt.Sprintf("API key %s was not found after it was written", plan.Id.ValueString())
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// State written by an earlier provider version has a null role, and a plan made without a refresh
	// carries it over. Terraform requires the applied value to match that plan, so the role stays
	// null until the next refresh reads it.
	if !planned.IsUnknown() && secretJwtRoleFromObject(planned).IsNull() {
		plan.SecretJwtTemplate = planned
	}
	return nil
}

func updateApiKey(ctx context.Context, plan *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	// A role change replaces the key, so an update sends the current role back unchanged
	// and never rebinds a key with a custom role to service_role.
	var secretJwtTemplate nullable.Nullable[map[string]interface{}]
	if plan.Type.ValueString() == string(api.ApiKeyResponseTypeSecret) {
		role := secretJwtRoleFromObject(plan.SecretJwtTemplate)
		if role.IsNull() || role.IsUnknown() {
			// State written by an earlier provider version has no role: send the key's current one.
			currentRole, diags := readApiKeySecretJwtRole(ctx, plan.ProjectRef.ValueString(), plan.Id.ValueString(), client)
			if diags.HasError() {
				return diags
			}
			role = currentRole
		}
		if !role.IsNull() {
			secretJwtTemplate = nullable.NewNullableWithValue(map[string]interface{}{"role": role.ValueString()})
		}
	}

	httpResp, err := client.V1UpdateProjectApiKeyWithResponse(ctx, plan.ProjectRef.ValueString(), uuid.MustParse(plan.Id.ValueString()), &api.V1UpdateProjectApiKeyParams{Reveal: Ptr(true)}, api.UpdateApiKeyBody{
		Name:              plan.Name.ValueStringPointer(),
		Description:       nullableDescription(plan.Description),
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

	return readApiKeyAfterWrite(ctx, plan, client)
}

func nullableDescription(description types.String) nullable.Nullable[string] {
	if description.IsNull() || description.IsUnknown() {
		return nullable.Nullable[string]{}
	}
	return nullable.NewNullableWithValue(description.ValueString())
}

func deleteApiKey(ctx context.Context, state *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1DeleteProjectApiKeyWithResponse(ctx, state.ProjectRef.ValueString(), uuid.MustParse(state.Id.ValueString()), &api.V1DeleteProjectApiKeyParams{Reveal: Ptr(true)})
	if err != nil {
		msg := fmt.Sprintf("Unable to delete apiKey, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if isApiKeyNotFound(httpResp.StatusCode(), httpResp.Body) {
		tflog.Trace(ctx, fmt.Sprintf("api key already deleted: %s/%s", state.ProjectRef.ValueString(), state.Id.ValueString()))
		return nil
	}
	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to delete apiKey, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}
