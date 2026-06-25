package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

var (
	_ resource.Resource                = &AuthSettingsResource{}
	_ resource.ResourceWithImportState = &AuthSettingsResource{}
)

func NewAuthSettingsResource() resource.Resource {
	return &AuthSettingsResource{}
}

type AuthSettingsResource struct {
	client *api.ClientWithResponses
}

func (r *AuthSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_settings"
}

func (r *AuthSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		r.client = client
	}
}

func (r *AuthSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AuthSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, defaultWaitTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectRef := plan.ProjectRef.ValueString()
	resp.Diagnostics.Append(waitForProjectActive(ctx, projectRef, r.client, createTimeout)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(waitForServicesActive(ctx, projectRef, r.client, createTimeout)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := modelToAuthConfigBody(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := r.client.V1UpdateAuthServiceConfigWithResponse(ctx, projectRef, body)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update auth config: %s", err))
		return
	}
	if httpResp.JSON200 == nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unexpected status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}

	state, diags := authConfigResponseToModel(ctx, httpResp.JSON200, &plan, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Id = state.ProjectRef
	tflog.Trace(ctx, "created auth_settings resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *AuthSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior AuthSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectRef := prior.ProjectRef.ValueString()
	httpResp, err := r.client.V1GetAuthServiceConfigWithResponse(ctx, projectRef)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read auth config: %s", err))
		return
	}
	if httpResp.StatusCode() == http.StatusNotFound || httpResp.StatusCode() == http.StatusNotAcceptable {
		resp.State.RemoveResource(ctx)
		return
	}
	if httpResp.JSON200 == nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unexpected status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}

	state, diags := authConfigResponseToModel(ctx, httpResp.JSON200, &prior, true)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Id = state.ProjectRef
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *AuthSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AuthSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, defaultWaitTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectRef := plan.ProjectRef.ValueString()
	resp.Diagnostics.Append(waitForProjectActive(ctx, projectRef, r.client, updateTimeout)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(waitForServicesActive(ctx, projectRef, r.client, updateTimeout)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := modelToAuthConfigBody(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := r.client.V1UpdateAuthServiceConfigWithResponse(ctx, projectRef, body)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update auth config: %s", err))
		return
	}
	if httpResp.JSON200 == nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unexpected status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}

	state, diags := authConfigResponseToModel(ctx, httpResp.JSON200, &plan, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Id = state.ProjectRef
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *AuthSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Auth config cannot be deleted; leaving it as-is on destroy is intentional.
}

func (r *AuthSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project_ref"), req, resp)
}
