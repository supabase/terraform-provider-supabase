// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &EdgeFunctionResource{}
var _ resource.ResourceWithImportState = &EdgeFunctionResource{}

func NewEdgeFunctionResource() resource.Resource {
	return &EdgeFunctionResource{}
}

// EdgeFunctionResource defines the resource implementation.
type EdgeFunctionResource struct {
	client *api.ClientWithResponses
}

// EdgeFunctionResourceModel describes the resource data model.
type EdgeFunctionResourceModel struct {
	ProjectRef types.String `tfsdk:"project_ref"`
	Slug       types.String `tfsdk:"slug"`
	Name       types.String `tfsdk:"name"`
	Body       types.String `tfsdk:"body"`
	VerifyJWT  types.Bool   `tfsdk:"verify_jwt"`
	ImportMap  types.Bool   `tfsdk:"import_map"`
	// Computed fields
	Id      types.String `tfsdk:"id"`
	Version types.Int64  `tfsdk:"version"`
	Status  types.String `tfsdk:"status"`
}

func (r *EdgeFunctionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_edge_function"
}

func (r *EdgeFunctionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Supabase Edge Function resource",

		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference identifier",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: "Function slug (identifier)",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Function display name",
				Optional:            true,
			},
			"body": schema.StringAttribute{
				MarkdownDescription: "Function code body (base64 encoded)",
				Required:            true,
			},
			"verify_jwt": schema.BoolAttribute{
				MarkdownDescription: "Require JWT verification for function invocations",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"import_map": schema.BoolAttribute{
				MarkdownDescription: "Enable import map support",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Function identifier",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: "Function version",
				Computed:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Function status (ACTIVE, DEPLOYING, ERROR, REMOVED)",
				Computed:            true,
			},
		},
	}
}

func (r *EdgeFunctionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *EdgeFunctionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EdgeFunctionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Decode base64 body
	body, err := base64.StdEncoding.DecodeString(data.Body.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Input", fmt.Sprintf("Unable to decode base64 body: %s", err))
		return
	}

	// Prepare function name and slug
	slug := data.Slug.ValueString()
	name := slug
	if !data.Name.IsNull() {
		name = data.Name.ValueString()
	}

	// Create function using body and params
	params := &api.V1CreateAFunctionParams{
		Slug:      &slug,
		Name:      &name,
		VerifyJwt: data.VerifyJWT.ValueBoolPointer(),
		ImportMap: data.ImportMap.ValueBoolPointer(),
	}

	createReq := api.V1CreateFunctionBody{
		Slug: data.Slug.ValueString(),
		Name: name,
		Body: string(body),
	}
	if data.VerifyJWT.ValueBoolPointer() != nil {
		createReq.VerifyJwt = data.VerifyJWT.ValueBoolPointer()
	}

	httpResp, err := r.client.V1CreateAFunctionWithResponse(ctx, data.ProjectRef.ValueString(), params, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create edge function, got error: %s", err))
		return
	}

	if httpResp.JSON201 == nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to create edge function, got status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}

	// Update state with response
	data.Id = types.StringValue(httpResp.JSON201.Id)
	data.Version = types.Int64Value(int64(httpResp.JSON201.Version))
	data.Status = types.StringValue(string(httpResp.JSON201.Status))
	data.Name = types.StringValue(httpResp.JSON201.Name)

	tflog.Trace(ctx, "created edge function resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EdgeFunctionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get function details
	httpResp, err := r.client.V1GetAFunctionWithResponse(ctx, data.ProjectRef.ValueString(), data.Slug.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read edge function, got error: %s", err))
		return
	}

	if httpResp.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if httpResp.JSON200 == nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to read edge function, got status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}

	// Update state
	data.Id = types.StringValue(httpResp.JSON200.Id)
	data.Version = types.Int64Value(int64(httpResp.JSON200.Version))
	data.Status = types.StringValue(string(httpResp.JSON200.Status))
	data.Name = types.StringValue(httpResp.JSON200.Name)

	if httpResp.JSON200.VerifyJwt != nil {
		data.VerifyJWT = types.BoolValue(*httpResp.JSON200.VerifyJwt)
	}
	if httpResp.JSON200.ImportMap != nil {
		data.ImportMap = types.BoolValue(*httpResp.JSON200.ImportMap)
	}

	// Note: We don't update the body from the API as it would require a separate call
	// and the body is typically large. The body in state represents what was deployed.

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EdgeFunctionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Decode base64 body
	body, err := base64.StdEncoding.DecodeString(data.Body.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Input", fmt.Sprintf("Unable to decode base64 body: %s", err))
		return
	}

	// Prepare function name
	name := data.Slug.ValueString()
	if !data.Name.IsNull() {
		name = data.Name.ValueString()
	}

	// Update function
	params := &api.V1UpdateAFunctionParams{
		VerifyJwt: data.VerifyJWT.ValueBoolPointer(),
		ImportMap: data.ImportMap.ValueBoolPointer(),
	}

	updateReq := api.V1UpdateFunctionBody{
		Body: ptrString(string(body)),
		Name: &name,
	}
	if data.VerifyJWT.ValueBoolPointer() != nil {
		updateReq.VerifyJwt = data.VerifyJWT.ValueBoolPointer()
	}

	httpResp, err := r.client.V1UpdateAFunctionWithResponse(ctx, data.ProjectRef.ValueString(), data.Slug.ValueString(), params, updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update edge function, got error: %s", err))
		return
	}

	if httpResp.JSON200 == nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to update edge function, got status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}

	// Update state with response
	data.Version = types.Int64Value(int64(httpResp.JSON200.Version))
	data.Status = types.StringValue(string(httpResp.JSON200.Status))
	data.Name = types.StringValue(httpResp.JSON200.Name)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EdgeFunctionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := r.client.V1DeleteAFunctionWithResponse(ctx, data.ProjectRef.ValueString(), data.Slug.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete edge function, got error: %s", err))
		return
	}

	if httpResp.StatusCode() != http.StatusNoContent && httpResp.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Unable to delete edge function, got status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}
}

func (r *EdgeFunctionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: project_ref:slug
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Helper function to create string pointer
func ptrString(s string) *string {
	return &s
}
