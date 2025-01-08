// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ProjectResource{}
var _ resource.ResourceWithImportState = &ProjectResource{}

func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

// ProjectResource defines the resource implementation.
type ProjectResource struct {
	client *api.ClientWithResponses
}

// ProjectResourceModel describes the resource data model.
type ProjectResourceModel struct {
	OrganizationId   types.String `tfsdk:"organization_id"`
	Name             types.String `tfsdk:"name"`
	DatabasePassword types.String `tfsdk:"database_password"`
	Region           types.String `tfsdk:"region"`
	InstanceSize     types.String `tfsdk:"instance_size"`
	Id               types.String `tfsdk:"id"`
}

func (r *ProjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Project resource",

		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "Reference to the organization",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the project",
				Required:            true,
			},
			"database_password": schema.StringAttribute{
				MarkdownDescription: "Password for the project database",
				Required:            true,
				Sensitive:           true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region where the project is located",
				Required:            true,
			},
			"instance_size": schema.StringAttribute{
				MarkdownDescription: "Desired instance size of the project",
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

func (r *ProjectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ProjectResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(createProject(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "create project")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ProjectResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read project")

	resp.Diagnostics.Append(readProject(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ProjectResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: allow api to update project resource
	msg := fmt.Sprintf("Update is not supported for project resource: %s", data.Id.ValueString())
	resp.Diagnostics.Append(diag.NewErrorDiagnostic("Client Error", msg))
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ProjectResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deleteProject(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "delete project")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func createProject(ctx context.Context, data *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	body := api.V1CreateProjectBodyDto{
		OrganizationId: data.OrganizationId.ValueString(),
		Name:           data.Name.ValueString(),
		DbPass:         data.DatabasePassword.ValueString(),
		Region:         api.V1CreateProjectBodyDtoRegion(data.Region.ValueString()),
	}
	if !data.InstanceSize.IsNull() {
		body.DesiredInstanceSize = Ptr(api.V1CreateProjectBodyDtoDesiredInstanceSize(data.InstanceSize.ValueString()))
	}

	httpResp, err := client.V1CreateAProjectWithResponse(ctx, body)
	if err != nil {
		msg := fmt.Sprintf("Unable to create project, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.JSON201 == nil {
		msg := fmt.Sprintf("Unable to create project, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	data.Id = types.StringValue(httpResp.JSON201.Id)
	return nil
}

func readProject(ctx context.Context, data *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1ListAllProjectsWithResponse(ctx)
	if err != nil {
		msg := fmt.Sprintf("Unable to read project, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read project, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	for _, project := range *httpResp.JSON200 {
		if project.Id == data.Id.ValueString() {
			data.OrganizationId = types.StringValue(project.OrganizationId)
			data.Name = types.StringValue(project.Name)
			data.Region = types.StringValue(project.Region)
			return nil
		}
	}

	// Not finding a project means our local state is stale. Return no error to allow TF to refresh its state.
	tflog.Trace(ctx, fmt.Sprintf("project not found: %s", data.Id.ValueString()))
	return nil
}

func deleteProject(ctx context.Context, data *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1DeleteAProjectWithResponse(ctx, data.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to delete project, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() == http.StatusNotFound {
		tflog.Trace(ctx, fmt.Sprintf("project not found: %s", data.Id.ValueString()))
		return nil
	}

	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to delete project, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	return nil
}
