// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/supabase/cli/apps/cli-go/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &SettingsResource{}
	_ resource.ResourceWithImportState = &SettingsResource{}
)

// Backend returns this 400 body fragment for unsupported projects.
const sslEnforcementUnsupported = "not allowed to configure SSL enforcements"

const (
	sslEnforcementStatusPending = "SSL_ENFORCEMENT_PENDING"
	sslEnforcementStatusApplied = "SSL_ENFORCEMENT_APPLIED"
)

func isSslEnforcementUnsupported(body []byte) bool {
	return strings.Contains(string(body), sslEnforcementUnsupported)
}

func NewSettingsResource() resource.Resource {
	return &SettingsResource{}
}

// SettingsResource defines the resource implementation.
type SettingsResource struct {
	client *api.ClientWithResponses
}

// SettingsResourceModel describes the resource data model.
type SettingsResourceModel struct {
	ProjectRef     types.String         `tfsdk:"project_ref"`
	Database       jsontypes.Normalized `tfsdk:"database"`
	Pooler         jsontypes.Normalized `tfsdk:"pooler"`
	Network        jsontypes.Normalized `tfsdk:"network"`
	Storage        jsontypes.Normalized `tfsdk:"storage"`
	Auth           jsontypes.Normalized `tfsdk:"auth"`
	Api            jsontypes.Normalized `tfsdk:"api"`
	SslEnforcement types.Bool           `tfsdk:"ssl_enforcement"`
	Id             types.String         `tfsdk:"id"`
	Timeouts       timeouts.Value       `tfsdk:"timeouts"`
}

func (r *SettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_settings"
}

func (r *SettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Settings resource",

		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: false,
			}),
		},
		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID",
				Required:            true,
			},
			"database": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Database settings as [serialised JSON](https://api.supabase.com/api/v1#/projects%20config/updateConfig)",
				Optional:            true,
			},
			"pooler": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Pooler settings as serialised JSON",
				Optional:            true,
			},
			"network": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Network settings as serialised JSON",
				Optional:            true,
			},
			"storage": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Storage settings as serialised JSON",
				Optional:            true,
			},
			"auth": schema.StringAttribute{
				CustomType: jsontypes.NormalizedType{},
				MarkdownDescription: "Auth settings as [serialised JSON](https://api.supabase.com/api/v1#/projects%20config/updateV1AuthConfig).\n\n" +
					"~> Several fields are returned as a hash by the API rather than plaintext. The provider preserves these fields from prior state, so out-of-band changes will not appear in `terraform plan`.\n\n" +
					"Affected fields: `smtp_pass`, `sms_twilio_auth_token`, `sms_twilio_verify_auth_token`, `sms_messagebird_access_key`, `sms_textlocal_api_key`, `sms_vonage_api_secret`, " +
					"`security_captcha_secret`, `external_apple_secret`, `external_azure_secret`, `external_bitbucket_secret`, `external_discord_secret`, `external_facebook_secret`, " +
					"`external_figma_secret`, `external_github_secret`, `external_gitlab_secret`, `external_google_secret`, `external_kakao_secret`, `external_keycloak_secret`, " +
					"`external_linkedin_oidc_secret`, `external_notion_secret`, `external_slack_oidc_secret`, `external_slack_secret`, `external_spotify_secret`, `external_twitch_secret`, " +
					"`external_twitter_secret`, `external_workos_secret`, `external_x_secret`, `external_zoom_secret`, `hook_custom_access_token_secrets`, `hook_mfa_verification_attempt_secrets`, " +
					"`hook_password_verification_attempt_secrets`, `hook_send_email_secrets`, `hook_send_sms_secrets`.",
				Optional: true,
			},
			"api": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "API settings as [serialised JSON](https://api.supabase.com/api/v1#/services/updatePostgRESTConfig)",
				Optional:            true,
			},
			"ssl_enforcement": schema.BoolAttribute{
				MarkdownDescription: "Enforce SSL on all database connections. See the [SSL enforcement API](https://api.supabase.com/api/v1#tag/database/put/v1/projects/%7Bref%7D/ssl-enforcement).",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Project identifier",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *SettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		r.client = client
	}
}

func (r *SettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SettingsResourceModel

	// Read Terraform plan data into the model
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

	resp.Diagnostics.Append(waitForServicesActive(ctx, data.ProjectRef.ValueString(), r.client, createTimeout)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Initial settings are always created together with the project resource.
	// We can simply apply partial updates here based on the given TF plan.
	if !data.Database.IsNull() {
		resp.Diagnostics.Append(updateDatabaseConfig(ctx, &data, r.client)...)
	}
	if !data.Network.IsNull() {
		resp.Diagnostics.Append(updateNetworkConfig(ctx, &data, r.client)...)
	}
	if !data.Api.IsNull() {
		resp.Diagnostics.Append(updateApiConfig(ctx, &data, r.client)...)
	}
	if !data.Auth.IsNull() {
		resp.Diagnostics.Append(updateAuthConfig(ctx, &data, r.client)...)
	}
	if !data.Storage.IsNull() {
		resp.Diagnostics.Append(updateStorageConfig(ctx, &data, r.client)...)
	}
	if !data.SslEnforcement.IsNull() && !data.SslEnforcement.IsUnknown() {
		resp.Diagnostics.Append(updateSslEnforcementConfig(ctx, &data, r.client, createTimeout)...)
	}
	// TODO: update all settings above concurrently
	if resp.Diagnostics.HasError() {
		return
	}

	data.Id = data.ProjectRef

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SettingsResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If an existing state has not been imported or created from a TF plan before,
	// skip loading them because we are not interested in managing them through TF.
	if !data.Database.IsNull() {
		resp.Diagnostics.Append(readDatabaseConfig(ctx, &data, r.client)...)
	}
	if !data.Network.IsNull() {
		resp.Diagnostics.Append(readNetworkConfig(ctx, &data, r.client)...)
	}
	if !data.Api.IsNull() {
		resp.Diagnostics.Append(readApiConfig(ctx, &data, r.client)...)
	}
	if !data.Auth.IsNull() {
		resp.Diagnostics.Append(readAuthConfig(ctx, &data, r.client)...)
	}
	if !data.Storage.IsNull() {
		resp.Diagnostics.Append(readStorageConfig(ctx, &data, r.client)...)
	}
	if !data.SslEnforcement.IsNull() {
		resp.Diagnostics.Append(readSslEnforcementConfig(ctx, &data, r.client)...)
	}
	// TODO: read all settings above concurrently
	if resp.Diagnostics.HasError() {
		return
	}

	data.ProjectRef = data.Id

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var planData, stateData SettingsResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read Terraform state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &stateData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, diags := planData.Timeouts.Update(ctx, defaultWaitTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(waitForProjectActive(ctx, planData.ProjectRef.ValueString(), r.client, updateTimeout)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(waitForServicesActive(ctx, planData.ProjectRef.ValueString(), r.client, updateTimeout)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only update settings that are present in the plan and have actually changed.
	// This respects lifecycle.ignore_changes and avoids no-op API calls.
	if !planData.Database.IsNull() && !planData.Database.Equal(stateData.Database) {
		resp.Diagnostics.Append(updateDatabaseConfig(ctx, &planData, r.client)...)
	}
	if !planData.Network.IsNull() && !planData.Network.Equal(stateData.Network) {
		resp.Diagnostics.Append(updateNetworkConfig(ctx, &planData, r.client)...)
	}
	if !planData.Api.IsNull() && !planData.Api.Equal(stateData.Api) {
		resp.Diagnostics.Append(updateApiConfig(ctx, &planData, r.client)...)
	}
	if !planData.Auth.IsNull() && !planData.Auth.Equal(stateData.Auth) {
		resp.Diagnostics.Append(updateAuthConfig(ctx, &planData, r.client)...)
	}
	if !planData.Storage.IsNull() && !planData.Storage.Equal(stateData.Storage) {
		resp.Diagnostics.Append(updateStorageConfig(ctx, &planData, r.client)...)
	}
	if !planData.SslEnforcement.IsNull() && !planData.SslEnforcement.IsUnknown() && !planData.SslEnforcement.Equal(stateData.SslEnforcement) {
		resp.Diagnostics.Append(updateSslEnforcementConfig(ctx, &planData, r.client, updateTimeout)...)
	}
	// TODO: update all settings above concurrently
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &planData)...)
}

func (r *SettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SettingsResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Simply fallthrough since there is no API to delete / reset settings.
}

func (r *SettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	data := SettingsResourceModel{
		Id: types.StringValue(req.ID),
		Timeouts: timeouts.Value{
			Object: types.ObjectNull(map[string]attr.Type{
				"create": types.StringType,
				"update": types.StringType,
			}),
		},
	}

	// Read all configs from API when importing so it's easier to pick
	// individual fields to manage through TF.
	resp.Diagnostics.Append(readDatabaseConfig(ctx, &data, r.client)...)
	resp.Diagnostics.Append(readNetworkConfig(ctx, &data, r.client)...)
	resp.Diagnostics.Append(readApiConfig(ctx, &data, r.client)...)
	resp.Diagnostics.Append(readAuthConfig(ctx, &data, r.client)...)
	resp.Diagnostics.Append(readStorageConfig(ctx, &data, r.client)...)
	resp.Diagnostics.Append(readSslEnforcementConfig(ctx, &data, r.client)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func readApiConfig(ctx context.Context, state *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1GetPostgrestServiceConfigWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read api settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Deleted project is an orphan resource, not returning error so it can be destroyed.
	switch httpResp.StatusCode() {
	case http.StatusNotFound, http.StatusNotAcceptable:
		return nil
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read api settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// TODO: API doesn't support updating jwt secret
	httpResp.JSON200.JwtSecret = nil
	if state.Api, err = parseConfig(state.Api, *httpResp.JSON200); err != nil {
		msg := fmt.Sprintf("Unable to read api settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}

func updateApiConfig(ctx context.Context, plan *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	var body api.V1UpdatePostgrestConfigBody
	if diags := plan.Api.Unmarshal(&body); diags.HasError() {
		return diags
	}

	httpResp, err := client.V1UpdatePostgrestServiceConfigWithResponse(ctx, plan.ProjectRef.ValueString(), body)
	if err != nil {
		msg := fmt.Sprintf("Unable to update api settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to update api settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if plan.Api, err = parseConfig(plan.Api, *httpResp.JSON200); err != nil {
		msg := fmt.Sprintf("Unable to update api settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}

func readAuthConfig(ctx context.Context, state *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1GetAuthServiceConfigWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read auth settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Deleted project is an orphan resource, not returning error so it can be destroyed.
	switch httpResp.StatusCode() {
	case http.StatusNotFound, http.StatusNotAcceptable:
		return nil
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read auth settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// API treats sensitive fields as write-only, preserve them from state
	var stateBody LocalAuthConfig
	if !state.Auth.IsNull() {
		if diags := state.Auth.Unmarshal(&stateBody); diags.HasError() {
			return diags
		}
	}
	// Convert response to UpdateAuthConfigBody type for consistent marshaling
	resultBody := convertAuthResponse(ctx, httpResp.JSON200)
	// Override sensitive fields with values from state
	copySensitiveFields(stateBody.UpdateAuthConfigBody, &resultBody)

	if state.Auth, err = parseConfig(state.Auth, resultBody); err != nil {
		msg := fmt.Sprintf("Unable to read auth settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}

func updateAuthConfig(ctx context.Context, plan *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	var body api.UpdateAuthConfigBody
	if diags := plan.Auth.Unmarshal(&body); diags.HasError() {
		return diags
	}

	httpResp, err := client.V1UpdateAuthServiceConfigWithResponse(ctx, plan.ProjectRef.ValueString(), body)
	if err != nil {
		msg := fmt.Sprintf("Unable to update auth settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to update auth settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Convert response to UpdateAuthConfigBody type for consistent marshaling
	resultBody := convertAuthResponse(ctx, httpResp.JSON200)
	// Copy over sensitive fields from TF plan
	copySensitiveFields(body, &resultBody)

	if plan.Auth, err = parseConfig(plan.Auth, resultBody); err != nil {
		msg := fmt.Sprintf("Unable to update auth settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}

func readDatabaseConfig(ctx context.Context, state *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1GetPostgresConfigWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read database settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Deleted project is an orphan resource, not returning error so it can be destroyed.
	switch httpResp.StatusCode() {
	case http.StatusNotFound, http.StatusNotAcceptable:
		return nil
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read database settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if state.Database, err = parseConfig(state.Database, *httpResp.JSON200); err != nil {
		msg := fmt.Sprintf("Unable to read database settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}

func updateDatabaseConfig(ctx context.Context, plan *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	var body api.UpdatePostgresConfigBody
	if diags := plan.Database.Unmarshal(&body); diags.HasError() {
		return diags
	}

	httpResp, err := client.V1UpdatePostgresConfigWithResponse(ctx, plan.ProjectRef.ValueString(), body)
	if err != nil {
		msg := fmt.Sprintf("Unable to update database settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to update database settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if plan.Database, err = parseConfig(plan.Database, *httpResp.JSON200); err != nil {
		msg := fmt.Sprintf("Unable to update database settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}

func readSslEnforcementConfig(ctx context.Context, state *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1GetSslEnforcementConfigWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read SSL enforcement config, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Deleted project is an orphan resource, not returning error so it can be destroyed.
	switch httpResp.StatusCode() {
	case http.StatusNotFound, http.StatusNotAcceptable:
		return nil
	case http.StatusBadRequest:
		// Preserve the managed value for unsupported projects.
		if isSslEnforcementUnsupported(httpResp.Body) {
			tflog.Warn(ctx, "SSL enforcement not supported, preserving existing state", map[string]any{
				"project_ref": state.Id.ValueString(),
				"body":        string(httpResp.Body),
			})
			return nil
		}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read SSL enforcement config, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	state.SslEnforcement = types.BoolValue(httpResp.JSON200.CurrentConfig.Database)
	return nil
}

func updateSslEnforcementConfig(ctx context.Context, plan *SettingsResourceModel, client *api.ClientWithResponses, timeout time.Duration) diag.Diagnostics {
	desired := plan.SslEnforcement.ValueBool()
	var body api.SslEnforcementRequest
	body.RequestedConfig.Database = desired

	projectRef := plan.ProjectRef.ValueString()
	httpResp, err := client.V1UpdateSslEnforcementConfigWithResponse(ctx, projectRef, body)
	if err != nil {
		msg := fmt.Sprintf("Unable to update SSL enforcement config, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.StatusCode() == http.StatusBadRequest {
		if isSslEnforcementUnsupported(httpResp.Body) {
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error",
				"SSL enforcement is not supported on this project. Upgrade your plan or check your project type.")}
		}
		msg := fmt.Sprintf("Unable to update SSL enforcement config, got status 400: %s", httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to update SSL enforcement config, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200.AppliedSuccessfully && httpResp.JSON200.CurrentConfig.Database == desired {
		plan.SslEnforcement = types.BoolValue(desired)
		return nil
	}
	stateConf := &retry.StateChangeConf{
		Pending: []string{sslEnforcementStatusPending},
		Target:  []string{sslEnforcementStatusApplied},
		Timeout: timeout,
		Refresh: func() (any, string, error) {
			getResp, err := client.V1GetSslEnforcementConfigWithResponse(ctx, projectRef)
			if err != nil {
				return nil, "", fmt.Errorf("failed polling SSL enforcement: %w", err)
			}
			if getResp.JSON200 == nil {
				return nil, "", fmt.Errorf("unexpected status %d: %s", getResp.StatusCode(), getResp.Body)
			}
			tflog.Debug(ctx, "Polling SSL enforcement convergence", map[string]any{
				"project_ref":          projectRef,
				"applied_successfully": getResp.JSON200.AppliedSuccessfully,
				"current_database":     getResp.JSON200.CurrentConfig.Database,
				"desired":              desired,
			})
			if getResp.JSON200.AppliedSuccessfully && getResp.JSON200.CurrentConfig.Database == desired {
				return getResp.JSON200, sslEnforcementStatusApplied, nil
			}
			return getResp.JSON200, sslEnforcementStatusPending, nil
		},
	}
	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic("SSL Enforcement Timeout",
			fmt.Sprintf("SSL enforcement did not converge for project %s: %s", projectRef, err))}
	}
	plan.SslEnforcement = types.BoolValue(desired)
	// Wait for ancillary services after the database reboot.
	return waitForServicesActive(ctx, projectRef, client, timeout)
}

func parseConfig(field jsontypes.Normalized, config any) (jsontypes.Normalized, error) {
	partial := make(map[string]any)
	if diags := field.Unmarshal(&partial); !diags.HasError() {
		pickConfig(config, partial)
	} else {
		// Handle errors when state is null or unknown
		copyConfig(config, partial)
	}
	value, err := json.Marshal(partial)
	if err != nil {
		return field, fmt.Errorf("failed to parse config: %w", err)
	}
	return jsontypes.NewNormalizedValue(string(value)), nil
}

func pickConfig(source any, target map[string]any) {
	v := reflect.ValueOf(source)
	t := reflect.TypeOf(source)
	for i := 0; i < v.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		k := strings.Split(tag, ",")[0]
		// Check that tag is picked by target
		targetVal, ok := target[k]
		if !ok {
			continue
		}
		sourceField := v.Field(i)
		// Skip null or unspecified Nullable fields to preserve user's original value.
		// This prevents the API returning null from overwriting user-configured values.
		if nullable, ok := sourceField.Interface().(interface {
			IsNull() bool
			IsSpecified() bool
		}); ok {
			if nullable.IsNull() || !nullable.IsSpecified() {
				continue
			}
		}
		// Recursively merge nested structs so user values survive when API omits fields.
		if targetMap, isMap := targetVal.(map[string]any); isMap {
			if sourceField.Kind() == reflect.Pointer {
				if sourceField.IsNil() {
					continue
				}
				sourceField = sourceField.Elem()
			}
			if sourceField.Kind() == reflect.Struct {
				pickConfig(sourceField.Interface(), targetMap)
				continue
			}
		}
		target[k] = sourceField.Interface()
	}
}

func copyConfig(source any, target map[string]any) {
	v := reflect.ValueOf(source)
	t := reflect.TypeOf(source)
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)

		if f.Kind() == reflect.Pointer && f.IsNil() {
			continue
		}

		if v, ok := f.Interface().(interface {
			IsNull() bool
			IsSpecified() bool
		}); ok {
			if !v.IsSpecified() {
				continue
			}

			if v.IsNull() {
				continue
			}
		}

		// Add omitempty tag by default
		tag := t.Field(i).Tag.Get("json")
		k := strings.Split(tag, ",")[0]
		target[k] = f.Interface()
	}
}

type LocalAuthConfig struct {
	api.UpdateAuthConfigBody
}

// convertAuthResponse converts AuthConfigResponse to UpdateAuthConfigBody using JSON marshaling.
// This ensures we use consistent JSON tags (with omitempty) for marshaling.
func convertAuthResponse(ctx context.Context, resp *api.AuthConfigResponse) api.UpdateAuthConfigBody {
	// Marshal the response to JSON and unmarshal into UpdateAuthConfigBody
	// This handles field mapping and type conversions automatically
	data, err := json.Marshal(resp)
	if err != nil {
		tflog.Error(ctx, "Failed to marshal auth config response", map[string]interface{}{
			"error": err.Error(),
		})
		return api.UpdateAuthConfigBody{}
	}

	var body api.UpdateAuthConfigBody
	if err := json.Unmarshal(data, &body); err != nil {
		tflog.Error(ctx, "Failed to unmarshal auth config to UpdateAuthConfigBody", map[string]interface{}{
			"error": err.Error(),
		})
		return api.UpdateAuthConfigBody{}
	}

	return body
}

// copySensitiveFields copies sensitive field values from source to target.
func copySensitiveFields(source api.UpdateAuthConfigBody, target *api.UpdateAuthConfigBody) {
	// Email provider secrets
	target.SmtpPass = source.SmtpPass
	// SMS provider secrets
	target.SmsTwilioAuthToken = source.SmsTwilioAuthToken
	target.SmsTwilioVerifyAuthToken = source.SmsTwilioVerifyAuthToken
	target.SmsMessagebirdAccessKey = source.SmsMessagebirdAccessKey
	target.SmsTextlocalApiKey = source.SmsTextlocalApiKey
	target.SmsVonageApiSecret = source.SmsVonageApiSecret
	// Captcha provider secrets
	target.SecurityCaptchaSecret = source.SecurityCaptchaSecret
	// External provider secrets
	target.ExternalAppleSecret = source.ExternalAppleSecret
	target.ExternalAzureSecret = source.ExternalAzureSecret
	target.ExternalBitbucketSecret = source.ExternalBitbucketSecret
	target.ExternalDiscordSecret = source.ExternalDiscordSecret
	target.ExternalFacebookSecret = source.ExternalFacebookSecret
	target.ExternalFigmaSecret = source.ExternalFigmaSecret
	target.ExternalGithubSecret = source.ExternalGithubSecret
	target.ExternalGitlabSecret = source.ExternalGitlabSecret
	target.ExternalGoogleSecret = source.ExternalGoogleSecret
	target.ExternalKakaoSecret = source.ExternalKakaoSecret
	target.ExternalKeycloakSecret = source.ExternalKeycloakSecret
	target.ExternalLinkedinOidcSecret = source.ExternalLinkedinOidcSecret
	target.ExternalNotionSecret = source.ExternalNotionSecret
	target.ExternalSlackOidcSecret = source.ExternalSlackOidcSecret
	target.ExternalSlackSecret = source.ExternalSlackSecret
	target.ExternalSpotifySecret = source.ExternalSpotifySecret
	target.ExternalTwitchSecret = source.ExternalTwitchSecret
	target.ExternalTwitterSecret = source.ExternalTwitterSecret
	target.ExternalWorkosSecret = source.ExternalWorkosSecret
	target.ExternalZoomSecret = source.ExternalZoomSecret
	target.ExternalXSecret = source.ExternalXSecret
	// Hook provider secrets
	target.HookCustomAccessTokenSecrets = source.HookCustomAccessTokenSecrets
	target.HookMfaVerificationAttemptSecrets = source.HookMfaVerificationAttemptSecrets
	target.HookPasswordVerificationAttemptSecrets = source.HookPasswordVerificationAttemptSecrets
	target.HookSendEmailSecrets = source.HookSendEmailSecrets
	target.HookSendSmsSecrets = source.HookSendSmsSecrets
}

type NetworkConfig struct {
	Restrictions []string `json:"restrictions,omitempty"`
}

func readNetworkConfig(ctx context.Context, state *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1GetNetworkRestrictionsWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read network settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Deleted project is an orphan resource, not returning error so it can be destroyed.
	switch httpResp.StatusCode() {
	case http.StatusNotFound, http.StatusNotAcceptable:
		return nil
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read network settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	var network NetworkConfig
	if v4 := httpResp.JSON200.Config.DbAllowedCidrs; v4 != nil {
		network.Restrictions = append(network.Restrictions, *v4...)
	}
	if v6 := httpResp.JSON200.Config.DbAllowedCidrsV6; v6 != nil {
		network.Restrictions = append(network.Restrictions, *v6...)
	}

	if state.Network, err = parseConfig(state.Network, network); err != nil {
		msg := fmt.Sprintf("Unable to read network settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}

func updateNetworkConfig(ctx context.Context, plan *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	var network NetworkConfig
	if diags := plan.Network.Unmarshal(&network); diags.HasError() {
		return diags
	}

	body := api.NetworkRestrictionsRequest{
		DbAllowedCidrs:   &[]string{},
		DbAllowedCidrsV6: &[]string{},
	}
	for _, cidr := range network.Restrictions {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			msg := fmt.Sprintf("Invalid CIDR provided for network restrictions: %s", err)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}
		if ip.IsPrivate() {
			msg := fmt.Sprintf("Private IP provided for network restrictions: %s", cidr)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}
		if ip.To4() != nil {
			*body.DbAllowedCidrs = append(*body.DbAllowedCidrs, cidr)
		} else {
			*body.DbAllowedCidrsV6 = append(*body.DbAllowedCidrsV6, cidr)
		}
	}

	httpResp, err := client.V1UpdateNetworkRestrictionsWithResponse(ctx, plan.ProjectRef.ValueString(), body)
	if err != nil {
		msg := fmt.Sprintf("Unable to update network settings: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON201 == nil {
		msg := fmt.Sprintf("Unable to update network settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if plan.Network, err = parseConfig(plan.Network, network); err != nil {
		msg := fmt.Sprintf("Unable to update network settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}

func readStorageConfig(ctx context.Context, state *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	// Use ProjectRef if Id is not set (during Create), otherwise use Id (during Read/Import)
	projectRef := state.Id.ValueString()
	if projectRef == "" {
		projectRef = state.ProjectRef.ValueString()
	}

	httpResp, err := client.V1GetStorageConfigWithResponse(ctx, projectRef)
	if err != nil {
		msg := fmt.Sprintf("Unable to read storage settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Deleted project is an orphan resource, not returning error so it can be destroyed.
	switch httpResp.StatusCode() {
	case http.StatusNotFound, http.StatusNotAcceptable:
		return nil
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read storage settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if state.Storage, err = parseConfig(state.Storage, *httpResp.JSON200); err != nil {
		msg := fmt.Sprintf("Unable to read storage settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}

func updateStorageConfig(ctx context.Context, plan *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	var body api.UpdateStorageConfigBody
	if diags := plan.Storage.Unmarshal(&body); diags.HasError() {
		return diags
	}

	httpResp, err := client.V1UpdateStorageConfigWithResponse(ctx, plan.ProjectRef.ValueString(), body)
	if err != nil {
		msg := fmt.Sprintf("Unable to update storage settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to update storage settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	// Read back the updated config to get the actual state with correct field names
	return readStorageConfig(ctx, plan, client)
}
