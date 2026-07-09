// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	Id            types.String         `tfsdk:"id"`
	Type          types.String         `tfsdk:"type"`
	ResolvedJWKS  jsontypes.Normalized `tfsdk:"resolved_jwks"`
	InsertedAt    types.String         `tfsdk:"inserted_at"`
	UpdatedAt     types.String         `tfsdk:"updated_at"`
	ResolvedAt    types.String         `tfsdk:"resolved_at"`
	Timeouts      timeouts.Value       `tfsdk:"timeouts"`
}

type publicJWKSValidator struct{}

// RequiresReplace uses raw string equality, not jsontypes.Normalized semantic
// equality, so compare JSON here to avoid replacement for formatting only changes.
type semanticJSONRequiresReplaceModifier struct{}

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
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Public JWKS",
			err.Error(),
		)
	}
}

func (m semanticJSONRequiresReplaceModifier) Description(ctx context.Context) string {
	return m.MarkdownDescription(ctx)
}

func (m semanticJSONRequiresReplaceModifier) MarkdownDescription(_ context.Context) string {
	return "Requires replacement when the JSON value changes semantically."
}

func (m semanticJSONRequiresReplaceModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	if req.PlanValue.Equal(req.StateValue) {
		return
	}

	if req.PlanValue.IsUnknown() || req.StateValue.IsUnknown() || req.PlanValue.IsNull() || req.StateValue.IsNull() {
		resp.RequiresReplace = true
		return
	}

	equal, err := jsonStringsEqual(req.PlanValue.ValueString(), req.StateValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Semantic JSON Equality Error",
			fmt.Sprintf("Unable to compare JSON values: %s", err),
		)
		return
	}

	if equal {
		return
	}

	resp.RequiresReplace = true
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
		MarkdownDescription: "Third-party auth resource",
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
			}),
		},
		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"oidc_issuer_url": schema.StringAttribute{
				MarkdownDescription: "OIDC issuer URL. Exactly one of `oidc_issuer_url`, `jwks_url`, or `custom_jwks` must be configured.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"jwks_url": schema.StringAttribute{
				MarkdownDescription: "JWKS URL. Exactly one of `oidc_issuer_url`, `jwks_url`, or `custom_jwks` must be configured.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"custom_jwks": schema.StringAttribute{
				CustomType: jsontypes.NormalizedType{},
				MarkdownDescription: "Custom public JWKS as serialised JSON. Exactly one of `oidc_issuer_url`, `jwks_url`, or `custom_jwks` must be configured. " +
					"This field follows Terraform provider industry practice for public verification keys and is not marked sensitive; do not include private or symmetric JWK material.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					semanticJSONRequiresReplaceModifier{},
				},
				Validators: []validator.String{
					publicJWKSValidator{},
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Third-party auth integration identifier",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Third-party auth integration type",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"resolved_jwks": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Resolved JWKS as serialised JSON",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"inserted_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp when the integration was created",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp when the integration was last updated",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"resolved_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp when JWKS was last resolved",
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

	resp.Diagnostics.Append(waitForProjectActive(ctx, data.ProjectRef.ValueString(), r.client, createTimeout)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(waitForAuthServiceActive(ctx, data.ProjectRef.ValueString(), r.client, createTimeout)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(createThirdPartyAuth(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created third party auth")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ThirdPartyAuthResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ThirdPartyAuthResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, diags := readThirdPartyAuth(ctx, &data, r.client)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	tflog.Trace(ctx, "read third party auth")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ThirdPartyAuthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ThirdPartyAuthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.ProjectRef.Equal(state.ProjectRef) ||
		!plan.OIDCIssuerURL.Equal(state.OIDCIssuerURL) ||
		!plan.JWKSURL.Equal(state.JWKSURL) {
		resp.Diagnostics.AddError(
			"Update Not Supported",
			"The Supabase Management API does not support updating third-party auth integrations. Changing any configurable attribute should replace this resource.",
		)
		return
	}

	if !plan.CustomJWKS.Equal(state.CustomJWKS) {
		if plan.CustomJWKS.IsUnknown() || state.CustomJWKS.IsUnknown() || plan.CustomJWKS.IsNull() || state.CustomJWKS.IsNull() {
			resp.Diagnostics.AddError(
				"Update Not Supported",
				"The Supabase Management API does not support updating third-party auth integrations. Changing any configurable attribute should replace this resource.",
			)
			return
		}

		equal, err := jsonStringsEqual(plan.CustomJWKS.ValueString(), state.CustomJWKS.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("custom_jwks"),
				"Semantic JSON Equality Error",
				fmt.Sprintf("Unable to compare JSON values: %s", err),
			)
			return
		}
		if !equal {
			resp.Diagnostics.AddError(
				"Update Not Supported",
				"The Supabase Management API does not support updating third-party auth integrations. Changing any configurable attribute should replace this resource.",
			)
			return
		}

		state.CustomJWKS = plan.CustomJWKS
	}

	// Timeout-only and semantic-only custom_jwks changes do not require a Supabase API call.
	state.Timeouts = plan.Timeouts

	tflog.Trace(ctx, "updated third party auth local state")

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ThirdPartyAuthResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ThirdPartyAuthResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deleteThirdPartyAuth(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "deleted third party auth")
}

func (r *ThirdPartyAuthResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import ID in format 'project_ref/tpa_id', got: %s", req.ID),
		)
		return
	}

	data := ThirdPartyAuthResourceModel{
		ProjectRef: types.StringValue(strings.TrimSpace(parts[0])),
		Id:         types.StringValue(strings.TrimSpace(parts[1])),
		Timeouts: timeouts.Value{
			Object: types.ObjectNull(map[string]attr.Type{
				"create": types.StringType,
			}),
		},
	}

	found, diags := readThirdPartyAuth(ctx, &data, r.client)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Resource Not Found",
			fmt.Sprintf("Third-party auth integration %s does not exist in project %s", data.Id.ValueString(), data.ProjectRef.ValueString()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func createThirdPartyAuth(ctx context.Context, data *ThirdPartyAuthResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	body, diags := buildThirdPartyAuthCreateBody(data)
	if diags.HasError() {
		return diags
	}

	httpResp, err := client.V1CreateProjectTpaIntegrationWithResponse(ctx, data.ProjectRef.ValueString(), body)
	if err != nil {
		msg := fmt.Sprintf("Unable to create third-party auth integration, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON201 == nil {
		msg := fmt.Sprintf("Unable to create third-party auth integration, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	return setThirdPartyAuthState(data, *httpResp.JSON201)
}

func readThirdPartyAuth(ctx context.Context, data *ThirdPartyAuthResourceModel, client *api.ClientWithResponses) (bool, diag.Diagnostics) {
	tpaID, diags := parseThirdPartyAuthUUID(data.Id.ValueString())
	if diags.HasError() {
		return false, diags
	}

	httpResp, err := client.V1GetProjectTpaIntegrationWithResponse(ctx, data.ProjectRef.ValueString(), tpaID)
	if err != nil {
		msg := fmt.Sprintf("Unable to read third-party auth integration, got error: %s", err)
		return false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.StatusCode() == http.StatusNotFound {
		return false, nil
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read third-party auth integration, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if diags := setThirdPartyAuthState(data, *httpResp.JSON200); diags.HasError() {
		return false, diags
	}
	return true, nil
}

func deleteThirdPartyAuth(ctx context.Context, data *ThirdPartyAuthResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	tpaID, diags := parseThirdPartyAuthUUID(data.Id.ValueString())
	if diags.HasError() {
		return diags
	}

	httpResp, err := client.V1DeleteProjectTpaIntegrationWithResponse(ctx, data.ProjectRef.ValueString(), tpaID)
	if err != nil {
		msg := fmt.Sprintf("Unable to delete third-party auth integration, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to delete third-party auth integration, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	return nil
}

func buildThirdPartyAuthCreateBody(data *ThirdPartyAuthResourceModel) (api.V1CreateProjectTpaIntegrationJSONRequestBody, diag.Diagnostics) {
	body := api.V1CreateProjectTpaIntegrationJSONRequestBody{}
	sourceCount := 0

	if data.OIDCIssuerURL.IsUnknown() {
		return body, diag.Diagnostics{diag.NewErrorDiagnostic(
			"Unknown Third-Party Auth Source",
			"The provider cannot create a third-party auth integration with an unknown oidc_issuer_url value.",
		)}
	}
	if !data.OIDCIssuerURL.IsNull() {
		sourceCount++
		body.OidcIssuerUrl = Ptr(data.OIDCIssuerURL.ValueString())
	}

	if data.JWKSURL.IsUnknown() {
		return body, diag.Diagnostics{diag.NewErrorDiagnostic(
			"Unknown Third-Party Auth Source",
			"The provider cannot create a third-party auth integration with an unknown jwks_url value.",
		)}
	}
	if !data.JWKSURL.IsNull() {
		sourceCount++
		body.JwksUrl = Ptr(data.JWKSURL.ValueString())
	}

	if data.CustomJWKS.IsUnknown() {
		return body, diag.Diagnostics{diag.NewErrorDiagnostic(
			"Unknown Third-Party Auth Source",
			"The provider cannot create a third-party auth integration with an unknown custom_jwks value.",
		)}
	}
	if !data.CustomJWKS.IsNull() {
		sourceCount++
		if err := validatePublicJWKS([]byte(data.CustomJWKS.ValueString())); err != nil {
			return body, diag.Diagnostics{diag.NewErrorDiagnostic("Invalid Public JWKS", err.Error())}
		}

		var customJWKS any
		if diags := data.CustomJWKS.Unmarshal(&customJWKS); diags.HasError() {
			return body, diags
		}
		body.CustomJwks = &customJWKS
	}

	if sourceCount != 1 {
		return body, diag.Diagnostics{diag.NewErrorDiagnostic(
			"Invalid Third-Party Auth Source",
			"Exactly one of oidc_issuer_url, jwks_url, or custom_jwks must be configured with a known non-null value.",
		)}
	}

	return body, nil
}

func setThirdPartyAuthState(data *ThirdPartyAuthResourceModel, tpa api.ThirdPartyAuth) diag.Diagnostics {
	var err error

	data.Id = types.StringValue(tpa.Id.String())
	data.Type = types.StringValue(tpa.Type)
	data.OIDCIssuerURL = NullableToString(tpa.OidcIssuerUrl)
	data.JWKSURL = NullableToString(tpa.JwksUrl)
	data.InsertedAt = types.StringValue(tpa.InsertedAt)
	data.UpdatedAt = types.StringValue(tpa.UpdatedAt)
	data.ResolvedAt = NullableToString(tpa.ResolvedAt)

	if data.CustomJWKS, err = nullablePublicJWKSAnyToNormalized(tpa.CustomJwks); err != nil {
		msg := fmt.Sprintf("Unable to parse custom JWKS, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if data.ResolvedJWKS, err = nullablePublicJWKSAnyToNormalized(tpa.ResolvedJwks); err != nil {
		msg := fmt.Sprintf("Unable to parse resolved JWKS, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	return nil
}

func parseThirdPartyAuthUUID(value string) (uuid.UUID, diag.Diagnostics) {
	targetID, err := uuid.Parse(value)
	if err != nil {
		msg := fmt.Sprintf("Invalid third-party auth integration ID %q: %s", value, err)
		return uuid.Nil, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return targetID, nil
}

func nullableAnyToNormalized(value nullable.Nullable[interface{}]) (jsontypes.Normalized, error) {
	if !value.IsSpecified() || value.IsNull() {
		return jsontypes.NewNormalizedNull(), nil
	}

	return anyToNormalized(value.MustGet())
}

func nullablePublicJWKSAnyToNormalized(value nullable.Nullable[interface{}]) (jsontypes.Normalized, error) {
	normalized, err := nullableAnyToNormalized(value)
	if err != nil || normalized.IsNull() {
		return normalized, err
	}

	if err := validatePublicJWKS([]byte(normalized.ValueString())); err != nil {
		return jsontypes.NewNormalizedNull(), err
	}

	return normalized, nil
}

func anyToNormalized(value any) (jsontypes.Normalized, error) {
	if value == nil {
		return jsontypes.NewNormalizedNull(), nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return jsontypes.NewNormalizedNull(), err
	}
	return jsontypes.NewNormalizedValue(string(data)), nil
}

func jsonStringsEqual(left, right string) (bool, error) {
	normalizedLeft, err := normalizeJSON(left)
	if err != nil {
		return false, err
	}

	normalizedRight, err := normalizeJSON(right)
	if err != nil {
		return false, err
	}

	return bytes.Equal(normalizedLeft, normalizedRight), nil
}

func normalizeJSON(value string) ([]byte, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}

	return json.Marshal(decoded)
}

func validatePublicJWKS(data []byte) error {
	var jwks map[string]any
	if err := json.Unmarshal(data, &jwks); err != nil {
		return fmt.Errorf("custom_jwks must be valid JSON: %w", err)
	}

	keysValue, ok := jwks["keys"]
	if !ok {
		return fmt.Errorf("custom_jwks must be a JWKS object with a keys array")
	}

	keys, ok := keysValue.([]any)
	if !ok {
		return fmt.Errorf("custom_jwks.keys must be an array")
	}
	if len(keys) == 0 {
		return fmt.Errorf("custom_jwks.keys must contain at least one public JWK")
	}

	privateMembers := map[string]struct{}{
		"d":   {},
		"p":   {},
		"q":   {},
		"dp":  {},
		"dq":  {},
		"qi":  {},
		"oth": {},
		"k":   {},
	}

	for i, keyValue := range keys {
		key, ok := keyValue.(map[string]any)
		if !ok {
			return fmt.Errorf("custom_jwks.keys[%d] must be an object", i)
		}

		for member := range privateMembers {
			if _, ok := key[member]; ok {
				return fmt.Errorf("custom_jwks.keys[%d] contains private or symmetric JWK member %q; provide public JWKS material only", i, member)
			}
		}
	}

	return nil
}
