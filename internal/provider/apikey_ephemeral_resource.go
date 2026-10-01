// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ ephemeral.EphemeralResource              = &APIKeyEphemeralResource{}
	_ ephemeral.EphemeralResourceWithConfigure = &APIKeyEphemeralResource{}
)

func NewApiKeyEphemeralResource() ephemeral.EphemeralResource {
	return &APIKeyEphemeralResource{}
}

// APIKeyEphemeralResource provisions a secret API key for the current Terraform
// operation. The revealed key is returned only from Open and is not written to
// the plan or state.
type APIKeyEphemeralResource struct {
	client *api.ClientWithResponses
}

func (r *APIKeyEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apikey"
}

func (r *APIKeyEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Provisions a Supabase secret API key for the current Terraform operation. " +
			"The key value is kept in memory for that operation and is not written to the plan or state. " +
			"Requires Terraform 1.10 or later.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "API key identifier",
				Computed:            true,
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
				MarkdownDescription: "Description of the API key",
				Optional:            true,
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Type of the API key",
				Computed:            true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "API key. Available only during the current Terraform operation and not stored in state.",
				Computed:            true,
				Sensitive:           true,
			},
			"secret_jwt_template": schema.SingleNestedAttribute{
				MarkdownDescription: "Secret JWT template",
				Computed:            true,
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

func (r *APIKeyEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		r.client = client
	}
}

func (r *APIKeyEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var data ApiKeyResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			"Supabase API client is not configured.",
		)
		return
	}

	resp.Diagnostics.Append(openAPIKey(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "opened api key ephemeral resource")

	resp.Diagnostics.Append(resp.Result.Set(ctx, &data)...)
}

// apiKeyOpenMu serializes ephemeral opens in this provider process.
// Concurrent opens can otherwise both miss a key and create duplicates.
// It does not coordinate with managed supabase_apikey creates or with other processes.
var apiKeyOpenMu sync.Mutex

// openAPIKey creates the secret key when this project does not already have
// one with the configured name, then reveals it. Later opens reuse that key.
// The key is left in place after the operation so it can still authenticate
// requests; ephemeral resources are not destroyed when removed from configuration.
func openAPIKey(ctx context.Context, data *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	apiKeyOpenMu.Lock()
	defer apiKeyOpenMu.Unlock()

	keys, diags := listProjectAPIKeys(ctx, data.ProjectRef.ValueString(), client)
	if diags.HasError() {
		return diags
	}

	match, found, hasDefaultPublishable, diags := matchProjectAPIKeys(keys, data.Name.ValueString())
	if diags.HasError() {
		return diags
	}

	if !hasDefaultPublishable {
		if diags := ensureDefaultPublishableAPIKey(ctx, data.ProjectRef.ValueString(), client); diags.HasError() {
			return diags
		}
	}

	if !found {
		if diags := createSecretAPIKey(ctx, data, client); diags.HasError() {
			return diags
		}
	} else {
		data.Id = NullableToString(match.Id)
		data.Type = NullableToString(match.Type)
	}

	if diags := requireAPIKeyID(data.Id); diags.HasError() {
		return diags
	}

	if found && shouldUpdateAPIKeyDescription(data.Description, match.Description) {
		return updateAPIKeyDescription(ctx, data, client)
	}

	return readApiKeyDatabase(ctx, data, client)
}

func listProjectAPIKeys(ctx context.Context, projectRef string, client *api.ClientWithResponses) ([]api.ApiKeyResponse, diag.Diagnostics) {
	listResp, err := client.V1GetProjectApiKeysWithResponse(ctx, projectRef, &api.V1GetProjectApiKeysParams{})
	if err != nil {
		msg := fmt.Sprintf("Unable to read api keys, got error: %s", err)
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if listResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read api keys, got status %d: %s", listResp.StatusCode(), listResp.Body)
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return *listResp.JSON200, nil
}

func isDefaultPublishableAPIKey(key api.ApiKeyResponse) bool {
	keyType, ok := specifiedAPIKeyType(key)
	return ok && key.Name == "default" && keyType == api.ApiKeyResponseTypePublishable
}

func matchProjectAPIKeys(keys []api.ApiKeyResponse, name string) (match api.ApiKeyResponse, found bool, hasDefaultPublishable bool, diags diag.Diagnostics) {
	for _, key := range keys {
		if isDefaultPublishableAPIKey(key) {
			hasDefaultPublishable = true
		}
		keyType, ok := specifiedAPIKeyType(key)
		if !ok {
			continue
		}
		if key.Name != name || keyType != api.ApiKeyResponseTypeSecret {
			continue
		}
		if !key.Id.IsSpecified() || key.Id.IsNull() {
			continue
		}
		if found {
			return api.ApiKeyResponse{}, false, hasDefaultPublishable, diag.Diagnostics{diag.NewErrorDiagnostic(
				"Ambiguous API Key",
				fmt.Sprintf("Found multiple secret API keys named %q. Rename one of them so the ephemeral resource can select a single key.", name),
			)}
		}
		match = key
		found = true
	}
	return match, found, hasDefaultPublishable, nil
}

// updateAPIKeyDescription changes only the description. The managed resource
// update also writes the service-role JWT template, which would replace a
// custom template on an existing secret key.
func updateAPIKeyDescription(ctx context.Context, data *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1UpdateProjectApiKeyWithResponse(ctx, data.ProjectRef.ValueString(), uuid.MustParse(data.Id.ValueString()), &api.V1UpdateProjectApiKeyParams{Reveal: Ptr(true)}, api.UpdateApiKeyBody{
		Description: apiKeyDescription(data.Description),
	})
	if err != nil {
		msg := fmt.Sprintf("Unable to update apiKey, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to update apiKey, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	return readApiKeyDatabase(ctx, data, client)
}

func shouldUpdateAPIKeyDescription(desired types.String, current nullable.Nullable[string]) bool {
	if desired.IsNull() || desired.IsUnknown() {
		return false
	}
	if !current.IsSpecified() || current.IsNull() {
		return true
	}
	return current.MustGet() != desired.ValueString()
}

func requireAPIKeyID(id types.String) diag.Diagnostics {
	if id.IsNull() || id.IsUnknown() || id.ValueString() == "" {
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", "API key response did not include an id.")}
	}
	if _, err := uuid.Parse(id.ValueString()); err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", fmt.Sprintf("API key id %q is not a UUID.", id.ValueString()))}
	}
	return nil
}

func specifiedAPIKeyType(key api.ApiKeyResponse) (api.ApiKeyResponseType, bool) {
	if !key.Type.IsSpecified() || key.Type.IsNull() {
		return "", false
	}
	return key.Type.MustGet(), true
}

func apiKeyDescription(value types.String) nullable.Nullable[string] {
	if value.IsNull() || value.IsUnknown() {
		return nullable.Nullable[string]{}
	}
	return nullable.NewNullableWithValue(value.ValueString())
}

func createProjectAPIKey(ctx context.Context, client *api.ClientWithResponses, projectRef, what string, body api.CreateApiKeyBody) (*api.ApiKeyResponse, diag.Diagnostics) {
	httpResp, err := client.V1CreateProjectApiKeyWithResponse(ctx, projectRef, &api.V1CreateProjectApiKeyParams{Reveal: Ptr(true)}, body)
	if err != nil {
		msg := fmt.Sprintf("Unable to create %s, got error: %s", what, err)
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON201 == nil {
		msg := fmt.Sprintf("Unable to create %s, got status %d: %s", what, httpResp.StatusCode(), httpResp.Body)
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return httpResp.JSON201, nil
}

func ensureDefaultPublishableAPIKey(ctx context.Context, projectRef string, client *api.ClientWithResponses) diag.Diagnostics {
	_, diags := createProjectAPIKey(ctx, client, projectRef, "default publishable apiKey", api.CreateApiKeyBody{
		Name:              "default",
		Type:              api.CreateApiKeyBodyTypePublishable,
		Description:       nullable.Nullable[string]{},
		SecretJwtTemplate: nullable.Nullable[map[string]interface{}]{},
	})
	return diags
}

func createSecretAPIKey(ctx context.Context, data *ApiKeyResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	created, diags := createProjectAPIKey(ctx, client, data.ProjectRef.ValueString(), "apiKey", api.CreateApiKeyBody{
		Name:              data.Name.ValueString(),
		Type:              api.CreateApiKeyBodyTypeSecret,
		Description:       apiKeyDescription(data.Description),
		SecretJwtTemplate: nullable.NewNullableWithValue(map[string]interface{}{"role": "service_role"}),
	})
	if diags.HasError() {
		return diags
	}
	data.Id = NullableToString(created.Id)
	data.ApiKey = NullableToString(created.ApiKey)
	data.Type = NullableToString(created.Type)
	return nil
}
