package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &EdgeFunctionSecretsResource{}
	_ resource.ResourceWithImportState = &EdgeFunctionSecretsResource{}
)

func NewEdgeFunctionSecretsResource() resource.Resource {
	return &EdgeFunctionSecretsResource{}
}

type EdgeFunctionSecretsResource struct {
	client *api.ClientWithResponses
}

type SecretModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

func (m SecretModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":  types.StringType,
		"value": types.StringType,
	}
}

type EdgeFunctionSecretsResourceModel struct {
	ProjectRef    types.String `tfsdk:"project_ref"`
	Secrets       types.Set    `tfsdk:"secrets"`
	SecretDigests types.Map    `tfsdk:"secret_digests"`
}

func (r *EdgeFunctionSecretsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_edge_function_secrets"
}

func (r *EdgeFunctionSecretsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Edge function secrets resource - manages multiple secrets for edge functions",
		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID",
				Required:            true,
			},
			"secrets": schema.SetNestedAttribute{
				MarkdownDescription: "Set of secrets for edge functions",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the secret (must not start with SUPABASE_ prefix)",
							Required:            true,
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "The secret value",
							Required:            true,
							Sensitive:           true,
						},
					},
				},
			},
			"secret_digests": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Map of secret name to SHA-256 digest of the secret value. Used to detect if a secret has been changed outside of Terraform management.",
			},
		},
	}
}

func (r *EdgeFunctionSecretsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *api.ClientWithResponses, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *EdgeFunctionSecretsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EdgeFunctionSecretsResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(createEdgeFunctionSecrets(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created edge function secrets")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionSecretsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EdgeFunctionSecretsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, diags := readEdgeFunctionSecrets(ctx, &data, r.client)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If resource was deleted out-of-band, remove from state
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	tflog.Trace(ctx, "read edge function secrets")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionSecretsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EdgeFunctionSecretsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// For simplicity, we delete all existing secrets and recreate them
	// This ensures the state matches the desired configuration
	resp.Diagnostics.Append(deleteEdgeFunctionSecrets(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(createEdgeFunctionSecrets(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated edge function secrets")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionSecretsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EdgeFunctionSecretsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deleteEdgeFunctionSecrets(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "deleted edge function secrets")
}

func (r *EdgeFunctionSecretsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: project_ref
	projectRef := req.ID

	var data EdgeFunctionSecretsResourceModel
	data.ProjectRef = types.StringValue(projectRef)

	found, diags := readEdgeFunctionSecrets(ctx, &data, r.client)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !found {
		resp.Diagnostics.AddError(
			"Resource Not Found",
			fmt.Sprintf("No secrets found for project %s", projectRef),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// computeSecretDigest returns the hex-encoded SHA-256 digest of the given string value.
func computeSecretDigest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func createEdgeFunctionSecrets(ctx context.Context, data *EdgeFunctionSecretsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	projectRef := data.ProjectRef.ValueString()

	// Parse secrets from the model
	var secrets []SecretModel
	diags := data.Secrets.ElementsAs(ctx, &secrets, false)
	if diags.HasError() {
		return diags
	}

	// Build the API request body
	var secretBody api.CreateSecretBody
	for _, secret := range secrets {
		secretBody = append(secretBody, struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}{
			Name:  secret.Name.ValueString(),
			Value: secret.Value.ValueString(),
		})
	}

	// Call the API
	httpResp, err := client.V1BulkCreateSecretsWithResponse(ctx, projectRef, secretBody)
	if err != nil {
		msg := fmt.Sprintf("Unable to create edge function secrets, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() != http.StatusOK && httpResp.StatusCode() != http.StatusCreated {
		msg := fmt.Sprintf("Unable to create edge function secrets, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("API Error", msg)}
	}

	// Compute and store SHA-256 digests for all secrets
	digestElements := make(map[string]attr.Value, len(secrets))
	for _, secret := range secrets {
		digestElements[secret.Name.ValueString()] = types.StringValue(computeSecretDigest(secret.Value.ValueString()))
	}
	secretDigests, mapDiags := types.MapValue(types.StringType, digestElements)
	if mapDiags.HasError() {
		return mapDiags
	}
	data.SecretDigests = secretDigests

	return nil
}

// Returns (true, nil) if secrets are found, (false, nil) if not found, or (false, diags) on error.
func readEdgeFunctionSecrets(ctx context.Context, data *EdgeFunctionSecretsResourceModel, client *api.ClientWithResponses) (bool, diag.Diagnostics) {
	projectRef := data.ProjectRef.ValueString()

	httpResp, err := client.V1ListAllSecretsWithResponse(ctx, projectRef)
	if err != nil {
		msg := fmt.Sprintf("Unable to read edge function secrets, got error: %s", err)
		return false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() == http.StatusNotFound {
		tflog.Trace(ctx, fmt.Sprintf("edge function secrets not found for project: %s", projectRef))
		return false, nil
	}

	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read edge function secrets, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return false, diag.Diagnostics{diag.NewErrorDiagnostic("API Error", msg)}
	}

	// Parse the secrets from state to get the values (API returns SHA-256 digest of secret values)
	var existingSecrets []SecretModel
	if !data.Secrets.IsNull() && !data.Secrets.IsUnknown() {
		diags := data.Secrets.ElementsAs(ctx, &existingSecrets, false)
		if diags.HasError() {
			// If we can't parse existing secrets during read, just use the API response
			existingSecrets = nil
		}
	}

	// Build a map of existing secret values
	valueMap := make(map[string]string)
	for _, secret := range existingSecrets {
		valueMap[secret.Name.ValueString()] = secret.Value.ValueString()
	}

	// Build a map of existing digests from state (may be null on first use or import)
	existingDigests := make(map[string]string)
	if !data.SecretDigests.IsNull() && !data.SecretDigests.IsUnknown() {
		_ = data.SecretDigests.ElementsAs(ctx, &existingDigests, false)
	}

	// Convert API response to our model and build the updated digest map
	// Filter out SUPABASE_ prefixed secrets as they cannot be managed by the provider
	secretModels := make([]SecretModel, 0, len(*httpResp.JSON200))
	newDigestElements := make(map[string]attr.Value, len(*httpResp.JSON200))
	for _, apiSecret := range *httpResp.JSON200 {
		// Skip secrets starting with SUPABASE_ as the API does not allow create/update/delete operations on them
		if strings.HasPrefix(apiSecret.Name, "SUPABASE_") {
			continue
		}

		apiDigest := apiSecret.Value // SHA-256 digest returned by the API
		secretValue := apiDigest     // Default: store API digest as value (signals drift to Terraform)

		if existingValue, exists := valueMap[apiSecret.Name]; exists {
			// Prefer the stored digest for comparison; fall back to computing sha256(value)
			localDigest, hasStoredDigest := existingDigests[apiSecret.Name]
			if !hasStoredDigest {
				localDigest = computeSecretDigest(existingValue)
			}
			if localDigest == apiDigest {
				// Digest matches – preserve the actual plaintext value from state
				secretValue = existingValue
			}
			// If no match, secretValue remains as the API digest to indicate drift
		}

		secretModels = append(secretModels, SecretModel{
			Name:  types.StringValue(apiSecret.Name),
			Value: types.StringValue(secretValue),
		})
		// Always record the API's digest as the authoritative remote value
		newDigestElements[apiSecret.Name] = types.StringValue(apiDigest)
	}

	// Convert to a set
	secretType := types.ObjectType{
		AttrTypes: SecretModel{}.AttributeTypes(),
	}
	secretSet, setDiags := types.SetValueFrom(ctx, secretType, secretModels)
	if setDiags.HasError() {
		return false, setDiags
	}

	// Build the updated digest map
	newSecretDigests, mapDiags := types.MapValue(types.StringType, newDigestElements)
	if mapDiags.HasError() {
		return false, mapDiags
	}

	data.Secrets = secretSet
	data.SecretDigests = newSecretDigests

	return true, nil
}

func deleteEdgeFunctionSecrets(ctx context.Context, data *EdgeFunctionSecretsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	projectRef := data.ProjectRef.ValueString()

	// Get all current secrets to delete
	var secrets []SecretModel
	diags := data.Secrets.ElementsAs(ctx, &secrets, false)
	if diags.HasError() {
		return diags
	}

	// Build list of secret names to delete
	var secretNames []string
	for _, secret := range secrets {
		secretNames = append(secretNames, secret.Name.ValueString())
	}

	if len(secretNames) == 0 {
		// Nothing to delete
		return nil
	}

	// Call the API to delete secrets
	httpResp, err := client.V1BulkDeleteSecretsWithResponse(ctx, projectRef, secretNames)
	if err != nil {
		msg := fmt.Sprintf("Unable to delete edge function secrets, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() == http.StatusNotFound {
		tflog.Trace(ctx, fmt.Sprintf("edge function secrets already deleted for project: %s", projectRef))
		return nil
	}

	if httpResp.StatusCode() != http.StatusOK && httpResp.StatusCode() != http.StatusNoContent {
		msg := fmt.Sprintf("Unable to delete edge function secrets, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("API Error", msg)}
	}

	return nil
}
