// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &SettingsResource{}
var _ resource.ResourceWithImportState = &SettingsResource{}

func NewSettingsResource() resource.Resource {
	return &SettingsResource{}
}

// SettingsResource defines the resource implementation.
type SettingsResource struct {
	client *api.ClientWithResponses
}

// SettingsResourceModel describes the resource data model.
type SettingsResourceModel struct {
	ProjectRef types.String         `tfsdk:"project_ref"`
	Pooler     jsontypes.Normalized `tfsdk:"pooler"`
	Storage    jsontypes.Normalized `tfsdk:"storage"`
	Auth       jsontypes.Normalized `tfsdk:"auth"`
	Api        jsontypes.Normalized `tfsdk:"api"`
	Id         types.String         `tfsdk:"id"`
}

func (r *SettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_settings"
}

func (r *SettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Settings resource",

		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID",
				Required:            true,
			},
			"pooler": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Pooler settings as serialised JSON",
				Optional:            true,
			},
			"storage": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Storage settings as serialised JSON",
				Optional:            true,
			},
			"auth": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Auth settings as [serialised JSON](https://api.supabase.com/api/v1#/projects%20config/updateV1AuthConfig)",
				Optional:            true,
			},
			"api": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "API settings as [serialised JSON](https://api.supabase.com/api/v1#/services/updatePostgRESTConfig)",
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
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.ClientWithResponses)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *SettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SettingsResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	if !data.Api.IsNull() {
		resp.Diagnostics.Append(updateApiConfig(ctx, &data, r.client)...)
	}
	if !data.Auth.IsNull() {
		resp.Diagnostics.Append(updateAuthConfig(ctx, &data, r.client)...)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// For the purposes of this example code, hardcoding a response value to
	// save into the Terraform state.
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

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	if !data.Api.IsNull() {
		resp.Diagnostics.Append(readApiConfig(ctx, &data, r.client)...)
	}
	if !data.Auth.IsNull() {
		resp.Diagnostics.Append(readAuthConfig(ctx, &data, r.client)...)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	data.ProjectRef = data.Id

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SettingsResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	if !data.Api.IsNull() {
		resp.Diagnostics.Append(updateApiConfig(ctx, &data, r.client)...)
	}
	if !data.Auth.IsNull() {
		resp.Diagnostics.Append(updateAuthConfig(ctx, &data, r.client)...)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SettingsResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete example, got error: %s", err))
	//     return
	// }
}

func (r *SettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	data := SettingsResourceModel{Id: types.StringValue(req.ID)}

	resp.Diagnostics.Append(readApiConfig(ctx, &data, r.client)...)
	resp.Diagnostics.Append(readAuthConfig(ctx, &data, r.client)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func readApiConfig(ctx context.Context, state *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.GetPostgRESTConfigWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read api settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Deleted project is an orphan resource, not returning error so it can be destroyed.
	switch httpResp.StatusCode() {
	case http.StatusNotFound, http.StatusNotAcceptable:
		return nil
	default:
		break
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read api settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	partial := make(map[string]interface{})
	if diags := state.Api.Unmarshal(&partial); !diags.HasError() {
		mergeConfig(*httpResp.JSON200, partial)
	} else {
		importConfig(*httpResp.JSON200, partial)
	}

	value, err := json.Marshal(partial)
	if err != nil {
		msg := fmt.Sprintf("Unable to read api settings, got marshal error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	state.Api = jsontypes.NewNormalizedValue(string(value))
	return nil
}

func updateApiConfig(ctx context.Context, plan *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	var body api.UpdatePostgrestConfigBody
	if diags := plan.Api.Unmarshal(&body); diags.HasError() {
		return diags
	}

	httpResp, err := client.UpdatePostgRESTConfigWithResponse(ctx, plan.ProjectRef.ValueString(), body)
	if err != nil {
		msg := fmt.Sprintf("Unable to update api settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to update api settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	partial := make(map[string]interface{})
	if diags := plan.Api.Unmarshal(&partial); diags.HasError() {
		return diags
	}
	mergeConfig(*httpResp.JSON200, partial)

	value, err := json.Marshal(partial)
	if err != nil {
		msg := fmt.Sprintf("Unable to update api settings, got marshal error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	plan.Api = jsontypes.NewNormalizedValue(string(value))
	return nil
}

func readAuthConfig(ctx context.Context, state *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.GetV1AuthConfigWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read auth settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Deleted project is an orphan resource, not returning error so it can be destroyed.
	switch httpResp.StatusCode() {
	case http.StatusNotFound, http.StatusNotAcceptable:
		return nil
	default:
		break
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read auth settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	partial := make(map[string]interface{})
	if diags := state.Auth.Unmarshal(&partial); !diags.HasError() {
		mergeConfig(*httpResp.JSON200, partial)
	} else {
		importConfig(*httpResp.JSON200, partial)
	}

	value, err := json.Marshal(partial)
	if err != nil {
		msg := fmt.Sprintf("Unable to read api settings, got marshal error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	state.Auth = jsontypes.NewNormalizedValue(string(value))
	return nil
}

func updateAuthConfig(ctx context.Context, plan *SettingsResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	var body api.UpdateAuthConfigBody
	if diags := plan.Auth.Unmarshal(&body); diags.HasError() {
		return diags
	}

	httpResp, err := client.UpdateV1AuthConfigWithResponse(ctx, plan.ProjectRef.ValueString(), body)
	if err != nil {
		msg := fmt.Sprintf("Unable to update auth settings, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to update auth settings, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	partial := make(map[string]interface{})
	if diags := plan.Auth.Unmarshal(&partial); diags.HasError() {
		return diags
	}
	mergeConfig(*httpResp.JSON200, partial)

	value, err := json.Marshal(partial)
	if err != nil {
		msg := fmt.Sprintf("Unable to update auth settings, got marshal error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	plan.Auth = jsontypes.NewNormalizedValue(string(value))
	return nil
}

func mergeConfig(source any, target map[string]interface{}) {
	v := reflect.ValueOf(source)
	t := reflect.TypeOf(source)
	for i := 0; i < v.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		k := strings.Split(tag, ",")[0]
		if _, ok := target[k]; ok {
			target[k] = v.Field(i).Interface()
		}
	}
}

func importConfig(source any, target map[string]interface{}) {
	v := reflect.ValueOf(source)
	t := reflect.TypeOf(source)
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Ptr || !f.IsNil() {
			tag := t.Field(i).Tag.Get("json")
			k := strings.Split(tag, ",")[0]
			target[k] = f.Interface()
		}
	}
}
