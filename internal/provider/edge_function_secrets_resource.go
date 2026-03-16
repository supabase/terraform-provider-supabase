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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &EdgeFunctionSecretsResource{}
	_ resource.ResourceWithImportState = &EdgeFunctionSecretsResource{}
)

const supabasePrefix = "SUPABASE_"

// secretDigestsPlanModifier computes SHA-256 digests from secret values during plan phase
// to make secret_digests known before apply.
type secretDigestsPlanModifier struct{}

func (m secretDigestsPlanModifier) Description(_ context.Context) string {
	return "Computes SHA-256 digests from secret values during plan to make digests known before apply."
}

func (m secretDigestsPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m secretDigestsPlanModifier) PlanModifyMap(ctx context.Context, req planmodifier.MapRequest, resp *planmodifier.MapResponse) {
	// If the entire plan is null (resource being destroyed), nothing to do
	if req.Plan.Raw.IsNull() {
		return
	}

	// Get the planned secrets attribute
	var secrets types.Set
	diags := req.Plan.GetAttribute(ctx, path.Root("secrets"), &secrets)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If secrets is unknown or null, we can't compute digests yet
	if secrets.IsUnknown() || secrets.IsNull() {
		return
	}

	// Parse secrets from the plan
	var secretModels []SecretModel
	diags = secrets.ElementsAs(ctx, &secretModels, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	secretDigests, mapDiags := computeSecretDigestsMap(secretModels)

	resp.Diagnostics.Append(mapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set the planned value for secret_digests
	resp.PlanValue = secretDigests
}

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
							MarkdownDescription: "Name of the secret. Must not start with the `SUPABASE_` prefix — such names are reserved internally by Supabase, cannot be created, updated, or deleted via the API, and are automatically filtered out from reads and imports.",
							Required:            true,
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "The plaintext secret value. Stored in Terraform state when managed by Terraform. After an import this field will be `null` because the Supabase API only returns SHA-256 digests — see the Import section below.",
							Required:            true,
							Sensitive:           true,
						},
					},
				},
			},
			"secret_digests": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Map of secret name to the SHA-256 hex digest of the secret value. Computed by the provider at plan time (when plaintext values are known) and updated after each apply. Used to detect drift when a secret has been changed outside of Terraform: if the digest returned by the API no longer matches the locally computed digest, the provider marks the affected secret value as null so Terraform will plan an update.",
				PlanModifiers: []planmodifier.Map{
					secretDigestsPlanModifier{},
				},
			},
		},
	}
}

func (r *EdgeFunctionSecretsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		r.client = client
	}
}

func (r *EdgeFunctionSecretsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EdgeFunctionSecretsResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(createOrUpdateEdgeFunctionSecrets(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created edge function secrets")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionSecretsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EdgeFunctionSecretsResourceModel

	// Read Terraform state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, diags := readEdgeFunctionSecretsForRead(ctx, &data, r.client)
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

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionSecretsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EdgeFunctionSecretsResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create/update secrets using the bulk create endpoint which handles upserts
	resp.Diagnostics.Append(createOrUpdateEdgeFunctionSecrets(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated edge function secrets")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionSecretsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EdgeFunctionSecretsResourceModel

	// Read Terraform state data into the model
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
	resp.Diagnostics.AddWarning(
		"Secret Values Not Returned",
		"The Supabase management API only returns SHA-256 hashes of secret values. "+
			"After import, Terraform will show a plan to update these secrets "+
			"to match the values defined in your configuration.",
	)
	projectRef := req.ID

	var data EdgeFunctionSecretsResourceModel
	data.ProjectRef = types.StringValue(projectRef)

	found, diags := readEdgeFunctionSecretsForImport(ctx, &data, r.client)
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

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// computeSecretDigest returns the hex-encoded SHA-256 digest of the given string value.
func computeSecretDigest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func computeSecretDigestsMap(secretModels []SecretModel) (types.Map, diag.Diagnostics) {
	// Compute and store SHA-256 digests for all secrets
	digestElements := make(map[string]attr.Value, len(secretModels))
	for _, secret := range secretModels {
		if secret.Value.IsUnknown() {
			continue
		}

		digestElements[secret.Name.ValueString()] = types.StringValue(computeSecretDigest(secret.Value.ValueString()))
	}

	// Build the digest map
	return types.MapValue(types.StringType, digestElements)
}

// buildSecretSetAndDigestMap converts secretModels and newDigestElements into
// types.Set and types.Map respectively, returning diagnostics on error.
func buildSecretSetAndDigestMap(ctx context.Context, secretModels []SecretModel, newDigestElements map[string]attr.Value) (types.Set, types.Map, diag.Diagnostics) {
	// Ensure newDigestElements is non-nil
	if newDigestElements == nil {
		newDigestElements = make(map[string]attr.Value)
	}

	// Convert to a set
	secretType := types.ObjectType{
		AttrTypes: SecretModel{}.AttributeTypes(),
	}
	secretSet, setDiags := types.SetValueFrom(ctx, secretType, secretModels)
	if setDiags.HasError() {
		return types.SetNull(secretType), types.MapNull(types.StringType), setDiags
	}

	// Build the updated digest map
	newSecretDigests, mapDiags := types.MapValue(types.StringType, newDigestElements)
	if mapDiags.HasError() {
		return types.SetNull(secretType), types.MapNull(types.StringType), mapDiags
	}

	return secretSet, newSecretDigests, nil
}

func createOrUpdateEdgeFunctionSecrets(ctx context.Context, data *EdgeFunctionSecretsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	projectRef := data.ProjectRef.ValueString()

	// Parse secretModels from the model
	var secretModels []SecretModel
	diags := data.Secrets.ElementsAs(ctx, &secretModels, false)
	if diags.HasError() {
		return diags
	}

	// Build the API request body
	var createSecretBody api.CreateSecretBody
	for _, secret := range secretModels {
		if secret.Value.IsNull() || secret.Value.IsUnknown() {
			// Skip secrets where we don't have a plaintext value (imported-only digests).
			continue
		}

		createSecretBody = append(createSecretBody, struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}{
			Name:  secret.Name.ValueString(),
			Value: secret.Value.ValueString(),
		})
	}

	// If there are no secrets to create/update, return early
	if len(createSecretBody) == 0 {
		return nil
	}

	// Call the API
	httpResp, err := client.V1BulkCreateSecretsWithResponse(ctx, projectRef, createSecretBody)
	if err != nil {
		msg := fmt.Sprintf("Unable to create/update edge function secrets, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() != http.StatusOK && httpResp.StatusCode() != http.StatusCreated {
		msg := fmt.Sprintf("Unable to create/update edge function secrets, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("API Error", msg)}
	}

	return nil
}

// fetchEdgeFunctionSecrets calls the API to fetch secrets and handles common error cases.
// Returns (secrets, found, diagnostics) where:
// - secrets is the list from the API (nil if error or not found)
// - found is false only when 404 is returned
// - diagnostics contains any errors encountered.
func fetchEdgeFunctionSecrets(ctx context.Context, projectRef string, client *api.ClientWithResponses) (*[]api.SecretResponse, diag.Diagnostics) {
	httpResp, err := client.V1ListAllSecretsWithResponse(ctx, projectRef)
	if err != nil {
		msg := fmt.Sprintf("Unable to read edge function secrets, got error: %s", err)
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() == http.StatusNotFound {
		tflog.Trace(ctx, fmt.Sprintf("edge function secrets not found for project: %s", projectRef))
		return nil, nil
	}

	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read edge function secrets, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("API Error", msg)}
	}

	return httpResp.JSON200, nil
}

// readEdgeFunctionSecretsForImport populates state from ALL non-SUPABASE_ secrets returned by the API.
// Returns (true, nil) if secrets are found, (false, nil) if not found, or (false, diags) on error.
func readEdgeFunctionSecretsForImport(ctx context.Context, data *EdgeFunctionSecretsResourceModel, client *api.ClientWithResponses) (bool, diag.Diagnostics) {
	projectRef := data.ProjectRef.ValueString()

	apiSecrets, diags := fetchEdgeFunctionSecrets(ctx, projectRef, client)
	if diags.HasError() || apiSecrets == nil {
		return false, diags
	}

	secretModels := make([]SecretModel, 0)
	newDigestElements := make(map[string]attr.Value)

	for _, apiSecret := range *apiSecrets {
		// Skip secrets starting with SUPABASE_ as the API does not allow create/update/delete operations on them
		// So we don't want to import them
		if strings.HasPrefix(apiSecret.Name, supabasePrefix) {
			continue
		}

		secretModels = append(secretModels, SecretModel{
			Name: types.StringValue(apiSecret.Name),
			// On import, the value field contains the digest not the actual secret, so store null as the value
			Value: types.StringNull(),
		})
		newDigestElements[apiSecret.Name] = types.StringValue(apiSecret.Value)
	}

	secretSet, newSecretDigests, buildDiags := buildSecretSetAndDigestMap(ctx, secretModels, newDigestElements)
	if buildDiags.HasError() {
		return false, buildDiags
	}

	data.Secrets = secretSet
	data.SecretDigests = newSecretDigests

	return true, nil
}

// readEdgeFunctionSecretsForRead reconciles only the secrets declared in the Terraform configuration.
// This prevents absorbing unmanaged secrets into state.
// Returns (true, nil) if secrets are found, (false, nil) if not found, or (false, diags) on error.
func readEdgeFunctionSecretsForRead(ctx context.Context, data *EdgeFunctionSecretsResourceModel, client *api.ClientWithResponses) (bool, diag.Diagnostics) {
	projectRef := data.ProjectRef.ValueString()

	apiSecrets, diags := fetchEdgeFunctionSecrets(ctx, projectRef, client)
	if diags.HasError() || apiSecrets == nil {
		return false, diags
	}

	secretModels := make([]SecretModel, 0)
	newDigestElements := make(map[string]attr.Value)

	// Parse the secrets from state to get the values (API returns SHA-256 digest of secret values)
	var existingSecrets []SecretModel
	if !data.Secrets.IsNull() && !data.Secrets.IsUnknown() {
		_ = data.Secrets.ElementsAs(ctx, &existingSecrets, false)
	}

	// Build a map of existing digests from state (may be null on first use or import)
	existingDigests := make(map[string]string)
	if !data.SecretDigests.IsNull() && !data.SecretDigests.IsUnknown() {
		_ = data.SecretDigests.ElementsAs(ctx, &existingDigests, false)
	}

	// Build a map of API secrets for quick lookup
	apiSecretsMap := make(map[string]string) // name -> digest
	for _, apiSecret := range *apiSecrets {
		apiSecretsMap[apiSecret.Name] = apiSecret.Value
	}

	// We only read secrets already existing in state to avoid reading secrets added out of band
	for _, existingSecret := range existingSecrets {
		secretName := existingSecret.Name.ValueString()

		apiDigest, existsInAPI := apiSecretsMap[secretName]
		if !existsInAPI {
			// Secret is in state but not in API - it was deleted out-of-band
			// Don't add it to the new state (will cause Terraform to detect drift)
			continue
		}

		// Secret exists in both state and API
		secretValue := types.StringNull() // Default: use nil to signal drift to Terraform

		existingValue := existingSecret.Value.ValueString()
		// Prefer the stored digest for comparison; fall back to computing sha256(value)
		localDigest, hasStoredDigest := existingDigests[secretName]
		if !hasStoredDigest {
			localDigest = computeSecretDigest(existingValue)
		}
		if localDigest == apiDigest {
			// Digest matches – preserve the actual plaintext value from state
			// BUT: only if we actually have plaintext (not null/unknown after import/drift)
			if !existingSecret.Value.IsNull() && !existingSecret.Value.IsUnknown() {
				secretValue = types.StringValue(existingValue)
			}
			// Otherwise, secretValue remains null even though digest matches
		}
		// If no match, secretValue remains null to signal drift

		secretModels = append(secretModels, SecretModel{
			Name:  types.StringValue(secretName),
			Value: secretValue,
		})
		// Always record the API's digest as the authoritative remote value
		newDigestElements[secretName] = types.StringValue(apiDigest)
	}

	secretSet, newSecretDigests, buildDiags := buildSecretSetAndDigestMap(ctx, secretModels, newDigestElements)
	if buildDiags.HasError() {
		return false, buildDiags
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
