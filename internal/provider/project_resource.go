// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	Plan             types.String `tfsdk:"plan"`
	ProjectRef       types.String `tfsdk:"project_ref"`
}

func (r *ProjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Project resource",

		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project identifier",
				Computed:            true,
			},
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
			"plan": schema.StringAttribute{
				MarkdownDescription: "Plan for the project",
				Required:            true,
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

	resp.Diagnostics.Append(diag.NewErrorDiagnostic("Client Error", "Update is not supported for this resource"))
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
	resource.ImportStatePassthroughID(ctx, path.Root("project_ref"), req, resp)
}

func createProject(ctx context.Context, data *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.CreateProjectWithResponse(ctx, api.CreateProjectJSONRequestBody{
		OrganizationId: data.OrganizationId.ValueString(),
		Name:           data.Name.ValueString(),
		DbPass:         data.DatabasePassword.ValueString(),
		Region:         api.CreateProjectBodyRegion(data.Region.ValueString()),
		Plan:           api.CreateProjectBodyPlan(data.Plan.ValueString()),
	})

	if err != nil {
		msg := fmt.Sprintf("Unable to create project, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.JSON200 == nil && httpResp.JSON201 == nil {
		msg := fmt.Sprintf("Unable to update api settings, got error: %s", httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	respBody := httpResp.JSON200
	if respBody == nil {
		respBody = httpResp.JSON201
	}

	data.ProjectRef = types.StringValue(respBody.Id)
	return nil
}

func readProject(ctx context.Context, data *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.GetProjectsWithResponse(ctx)

	if err != nil {
		msg := fmt.Sprintf("Unable to read project, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read project, got error: %s", httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}


	for _, project := range *httpResp.JSON200 {
		if project.Id == data.ProjectRef.ValueString() {
			data.OrganizationId = types.StringValue(project.OrganizationId)
			data.Name = types.StringValue(project.Name)
			data.Region = types.StringValue(project.Region)
			data.ProjectRef = types.StringValue(project.Id)
			return nil
		}
	}

	return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", "Project not found")}
}

func deleteProject(ctx context.Context, data *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.DeleteProjectWithResponse(ctx, data.ProjectRef.ValueString())

	if err != nil {
		msg := fmt.Sprintf("Unable to delete project, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to delete project, got error: %s", httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	return nil
}
