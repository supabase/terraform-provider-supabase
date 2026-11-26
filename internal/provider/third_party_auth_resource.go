package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/internal/errs"
	"github.com/supabase/terraform-provider-supabase/internal/jsonvalue"
	"github.com/supabase/terraform-provider-supabase/internal/planmodifiers"
)

var (
	_ resource.Resource                     = &ThirdPartyAuthResource{}
	_ resource.ResourceWithConfigValidators = &ThirdPartyAuthResource{}
	_ resource.ResourceWithImportState      = &ThirdPartyAuthResource{}
)

func NewThirdPartyAuthResource() resource.Resource {
	return &ThirdPartyAuthResource{}
}

type ThirdPartyAuthResource struct {
	client *api.ClientWithResponses
}

type ThirdPartyAuthResourceModel struct {
	ProjectRef    types.String         `tfsdk:"project_ref"`
	OIDCIssuerURL types.String         `tfsdk:"oidc_issuer_url"`
	JWKSURL       types.String         `tfsdk:"jwks_url"`
	CustomJWKS    jsontypes.Normalized `tfsdk:"custom_jwks"`
	ID            types.String         `tfsdk:"id"`
	Type          types.String         `tfsdk:"type"`
	ResolvedJWKS  jsontypes.Normalized `tfsdk:"resolved_jwks"`
	InsertedAt    types.String         `tfsdk:"inserted_at"`
	UpdatedAt     types.String         `tfsdk:"updated_at"`
	ResolvedAt    types.String         `tfsdk:"resolved_at"`
	Timeouts      timeouts.Value       `tfsdk:"timeouts"`
}

// publicJWKSValidator rejects custom_jwks values that aren't a JWKS of public keys.
type publicJWKSValidator struct{}

func (v publicJWKSValidator) Description(ctx context.Context) string {
	return v.MarkdownDescription(ctx)
}

func (v publicJWKSValidator) MarkdownDescription(_ context.Context) string {
	return "Ensures custom_jwks contains public JWKS material only."
}

func (v publicJWKSValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if err := validatePublicJWKS([]byte(req.ConfigValue.ValueString())); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Public JWKS", err.Error())
	}
}

func (r *ThirdPartyAuthResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_third_party_auth"
}

func (r *ThirdPartyAuthResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("oidc_issuer_url"),
			path.MatchRoot("jwks_url"),
			path.MatchRoot("custom_jwks"),
		),
	}
}

func (r *ThirdPartyAuthResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Registers a third-party auth provider with a Supabase project, so the project accepts JSON Web Tokens (JWTs) the provider issues. " +
			"The provider must sign its JWTs with asymmetric keys and include a `kid` header. " +
			"See [Third-party auth](https://supabase.com/docs/guides/auth/third-party/overview) in the Supabase docs. " +
			"Configure exactly one of `oidc_issuer_url`, `jwks_url` or `custom_jwks`. Changing any of them replaces the integration.",
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
			}),
		},
		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Reference ID of the Supabase project, for example `abcdefghijklmnopqrst`. " +
					"Find it in the project's dashboard URL or under **Project Settings** > **General**. Changing it replaces the integration.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"oidc_issuer_url": schema.StringAttribute{
				MarkdownDescription: "OpenID Connect (OIDC) issuer URL of the provider. It must use HTTPS. " +
					"Supabase reads the provider's signing keys from the issuer's discovery document.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"jwks_url": schema.StringAttribute{
				MarkdownDescription: "URL of the provider's JSON Web Key Set (JWKS).",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"custom_jwks": schema.StringAttribute{
				CustomType: jsontypes.NormalizedType{},
				MarkdownDescription: "The provider's public signing keys as a JSON Web Key Set, for example built with `jsonencode()`. " +
					"Private or symmetric key material is rejected. Reformatting the JSON doesn't replace the integration.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					planmodifiers.RequiresReplaceIfJSONChanged(),
				},
				Validators: []validator.String{
					publicJWKSValidator{},
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the integration. To import an existing integration, find its UUID with the Management API's " +
					"[list endpoint](https://supabase.com/docs/reference/api/v1-list-project-tpa-integrations).",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Integration type, as reported by Supabase.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"resolved_jwks": schema.StringAttribute{
				CustomType: jsontypes.NormalizedType{},
				MarkdownDescription: "The signing keys Supabase fetched from `oidc_issuer_url` or `jwks_url`, as JSON. " +
					"Supabase checks for new keys periodically, so a key rotation doesn't appear immediately. " +
					"If the keys include private or symmetric key material, this attribute is null, so that material never reaches the Terraform state.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"inserted_at": schema.StringAttribute{
				MarkdownDescription: "When the integration was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "When the integration was last updated.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"resolved_at": schema.StringAttribute{
				MarkdownDescription: "When Supabase last fetched `resolved_jwks`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ThirdPartyAuthResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		r.client = client
	}
}

func (r *ThirdPartyAuthResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ThirdPartyAuthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := data.Timeouts.Create(ctx, defaultWaitTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Both waits share the configured timeout instead of each getting the full duration.
	deadline := time.Now().Add(createTimeout)
	resp.Diagnostics.Append(waitForProjectActive(ctx, data.ProjectRef.ValueString(), r.client, time.Until(deadline))...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(waitForAuthServiceActive(ctx, data.ProjectRef.ValueString(), r.client, time.Until(deadline))...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := createThirdPartyAuth(ctx, &data, r.client); err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}

	tflog.Trace(ctx, "created third-party auth integration")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ThirdPartyAuthResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ThirdPartyAuthResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := readThirdPartyAuth(ctx, &data, r.client)
	if errs.NotFound(err) {
		tflog.Info(ctx, "third-party auth integration not found, removing it from state")
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}

	tflog.Trace(ctx, "read third-party auth integration")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ThirdPartyAuthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ThirdPartyAuthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sameJWKS, err := sameJSONValue(plan.CustomJWKS.StringValue, state.CustomJWKS.StringValue)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("custom_jwks"), "Semantic JSON Equality Error", fmt.Sprintf("Unable to compare JSON values: %s", err))
		return
	}

	// Every configurable attribute requires replacement, so Terraform only calls Update for timeout changes
	// and formatting-only custom_jwks changes. This check guards against an in-place change the API can't make.
	if !plan.ProjectRef.Equal(state.ProjectRef) || !plan.OIDCIssuerURL.Equal(state.OIDCIssuerURL) ||
		!plan.JWKSURL.Equal(state.JWKSURL) || !sameJWKS {
		resp.Diagnostics.AddError(
			"Update Not Supported",
			"The Supabase Management API does not support updating third-party auth integrations. Changing any configurable attribute should replace this resource.",
		)
		return
	}

	// Neither change needs an API call, so only the state changes.
	state.CustomJWKS = plan.CustomJWKS
	state.Timeouts = plan.Timeouts

	tflog.Trace(ctx, "updated third-party auth integration in state")

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ThirdPartyAuthResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ThirdPartyAuthResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := deleteThirdPartyAuth(ctx, &data, r.client); err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}

	tflog.Trace(ctx, "deleted third-party auth integration")
}

// ImportState sets the IDs from the import ID. Terraform then calls Read, which fills in the rest
// and reports a missing integration.
func (r *ThirdPartyAuthResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID in format '<project_ref>/<third_party_auth_id>', got: %s", req.ID))
		return
	}
	projectRef, id := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if _, err := uuid.Parse(id); err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected import ID in format '<project_ref>/<third_party_auth_id>' with a UUID as the ID, got: %s", req.ID))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_ref"), projectRef)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// createThirdPartyAuth creates the integration and copies the response into data.
func createThirdPartyAuth(ctx context.Context, data *ThirdPartyAuthResourceModel, client *api.ClientWithResponses) error {
	body, err := buildThirdPartyAuthCreateBody(data)
	if err != nil {
		return err
	}

	httpResp, err := client.V1CreateProjectTpaIntegrationWithResponse(ctx, data.ProjectRef.ValueString(), body)
	if err != nil {
		return fmt.Errorf("unable to create third-party auth integration, got error: %w", err)
	}
	if httpResp.JSON201 == nil {
		return fmt.Errorf("unable to create third-party auth integration, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
	}

	return setThirdPartyAuthState(ctx, data, *httpResp.JSON201)
}

// readThirdPartyAuth reads the integration into data. It returns an errs.NotFoundErr when the integration does not exist.
func readThirdPartyAuth(ctx context.Context, data *ThirdPartyAuthResourceModel, client *api.ClientWithResponses) error {
	id, err := parseThirdPartyAuthID(data.ID.ValueString())
	if err != nil {
		return err
	}

	httpResp, err := client.V1GetProjectTpaIntegrationWithResponse(ctx, data.ProjectRef.ValueString(), id)
	if err != nil {
		return fmt.Errorf("unable to read third-party auth integration, got error: %w", err)
	}
	if httpResp.StatusCode() == http.StatusNotFound {
		return errs.NewNotFoundErr("third-party auth integration", data.ID.ValueString())
	}
	if httpResp.JSON200 == nil {
		return fmt.Errorf("unable to read third-party auth integration, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
	}

	return setThirdPartyAuthState(ctx, data, *httpResp.JSON200)
}

// deleteThirdPartyAuth deletes the integration. An integration that is already gone counts as deleted.
func deleteThirdPartyAuth(ctx context.Context, data *ThirdPartyAuthResourceModel, client *api.ClientWithResponses) error {
	id, err := parseThirdPartyAuthID(data.ID.ValueString())
	if err != nil {
		return err
	}

	httpResp, err := client.V1DeleteProjectTpaIntegrationWithResponse(ctx, data.ProjectRef.ValueString(), id)
	if err != nil {
		return fmt.Errorf("unable to delete third-party auth integration, got error: %w", err)
	}
	if httpResp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if httpResp.StatusCode() != http.StatusOK {
		return fmt.Errorf("unable to delete third-party auth integration, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
	}

	return nil
}

// buildThirdPartyAuthCreateBody builds the create request from the planned key source.
// The config validators already ensure exactly one source is set.
func buildThirdPartyAuthCreateBody(data *ThirdPartyAuthResourceModel) (api.V1CreateProjectTpaIntegrationJSONRequestBody, error) {
	var body api.V1CreateProjectTpaIntegrationJSONRequestBody
	if !data.OIDCIssuerURL.IsNull() {
		body.OidcIssuerUrl = Ptr(data.OIDCIssuerURL.ValueString())
	}
	if !data.JWKSURL.IsNull() {
		body.JwksUrl = Ptr(data.JWKSURL.ValueString())
	}
	if !data.CustomJWKS.IsNull() {
		var customJWKS any
		if err := json.Unmarshal([]byte(data.CustomJWKS.ValueString()), &customJWKS); err != nil {
			return body, fmt.Errorf("unable to parse custom_jwks, got error: %w", err)
		}
		body.CustomJwks = &customJWKS
	}
	return body, nil
}

// setThirdPartyAuthState copies an API response into data.
func setThirdPartyAuthState(ctx context.Context, data *ThirdPartyAuthResourceModel, tpa api.ThirdPartyAuth) error {
	data.ID = types.StringValue(tpa.Id.String())
	data.Type = types.StringValue(tpa.Type)
	data.OIDCIssuerURL = NullableToString(tpa.OidcIssuerUrl)
	data.JWKSURL = NullableToString(tpa.JwksUrl)
	data.InsertedAt = types.StringValue(tpa.InsertedAt)
	data.UpdatedAt = types.StringValue(tpa.UpdatedAt)
	data.ResolvedAt = NullableToString(tpa.ResolvedAt)

	var err error
	if data.CustomJWKS, err = publicJWKSFromResponse(ctx, "custom_jwks", tpa.CustomJwks); err != nil {
		return err
	}
	if data.ResolvedJWKS, err = publicJWKSFromResponse(ctx, "resolved_jwks", tpa.ResolvedJwks); err != nil {
		return err
	}
	return nil
}

// publicJWKSFromResponse converts a JWKS returned by the API into a state value.
// Private or symmetric key material never reaches the state: the value becomes null and a warning is logged.
// The rest of the document is stored as returned, without the stricter checks applied to configuration.
func publicJWKSFromResponse(ctx context.Context, attribute string, value nullable.Nullable[any]) (jsontypes.Normalized, error) {
	if !value.IsSpecified() || value.IsNull() {
		return jsontypes.NewNormalizedNull(), nil
	}

	raw, err := json.Marshal(value.MustGet())
	if err != nil {
		return jsontypes.NewNormalizedNull(), fmt.Errorf("unable to encode %s from the API, got error: %w", attribute, err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return jsontypes.NewNormalizedNull(), fmt.Errorf("unable to decode %s from the API, got error: %w", attribute, err)
	}
	if location, member, found := privateJWKMember(decoded, ""); found {
		tflog.Warn(ctx, "Dropping JWKS with private key material from the API response", map[string]any{
			"attribute": attribute,
			"location":  location,
			"member":    member,
		})
		return jsontypes.NewNormalizedNull(), nil
	}
	return jsontypes.NewNormalizedValue(string(raw)), nil
}

func parseThirdPartyAuthID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid third-party auth integration ID %q: %w", value, err)
	}
	return id, nil
}

// sameJSONValue reports whether two string values hold the same JSON, ignoring formatting.
// A null or unknown value only matches an identical value.
func sameJSONValue(left, right types.String) (bool, error) {
	if left.Equal(right) {
		return true, nil
	}
	if left.IsNull() || left.IsUnknown() || right.IsNull() || right.IsUnknown() {
		return false, nil
	}

	return jsonvalue.Equal(left.ValueString(), right.ValueString())
}

// jwkPrivateMembers are the JWK members that hold private or symmetric key material.
var jwkPrivateMembers = []string{"d", "p", "q", "dp", "dq", "qi", "oth", "k"}

// privateJWKMember finds the first private or symmetric key member in a decoded JSON document. It returns the
// location of the object that holds it, such as keys[0], and the member's name. It searches every object, not only
// the keys array, so it also finds key material in a document that isn't a well-formed JWKS.
func privateJWKMember(value any, location string) (string, string, bool) {
	switch v := value.(type) {
	case map[string]any:
		for _, member := range jwkPrivateMembers {
			if _, ok := v[member]; ok {
				return location, member, true
			}
		}
		for _, name := range slices.Sorted(maps.Keys(v)) {
			child := name
			if location != "" {
				child = location + "." + name
			}
			if found, member, ok := privateJWKMember(v[name], child); ok {
				return found, member, true
			}
		}
	case []any:
		for i, item := range v {
			if found, member, ok := privateJWKMember(item, fmt.Sprintf("%s[%d]", location, i)); ok {
				return found, member, true
			}
		}
	}
	return "", "", false
}

// validatePublicJWKS checks that a custom_jwks value is a JWKS object with at least one key and no private key material.
func validatePublicJWKS(data []byte) error {
	var jwks map[string]any
	if err := json.Unmarshal(data, &jwks); err != nil {
		return fmt.Errorf("custom_jwks must be valid JSON: %w", err)
	}

	keys, ok := jwks["keys"].([]any)
	if !ok {
		return fmt.Errorf("custom_jwks must be a JWKS object with a keys array")
	}
	if len(keys) == 0 {
		return fmt.Errorf("custom_jwks.keys must contain at least one public JWK")
	}
	for i, key := range keys {
		if _, ok := key.(map[string]any); !ok {
			return fmt.Errorf("custom_jwks.keys[%d] must be an object", i)
		}
	}

	if location, member, found := privateJWKMember(jwks, "custom_jwks"); found {
		return fmt.Errorf("%s contains private or symmetric JWK member %q; provide public JWKS material only", location, member)
	}
	return nil
}
