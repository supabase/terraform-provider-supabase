// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
				MarkdownDescription: "Organization slug (found in the Supabase dashboard URL or organization settings)",
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
				Validators:          []validator.String{stringvalidator.LengthAtLeast(4)},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region where the project is located",
				Required:            true,
			},
			"instance_size": schema.StringAttribute{
				MarkdownDescription: "Desired instance size of the project",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(api.V1CreateProjectBodyDesiredInstanceSizeLarge),
						string(api.V1CreateProjectBodyDesiredInstanceSizeMedium),
						string(api.V1CreateProjectBodyDesiredInstanceSizeMicro),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN12xlarge),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN16xlarge),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN24xlarge),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN24xlargeHighMemory),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN24xlargeOptimizedCpu),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN24xlargeOptimizedMemory),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN2xlarge),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN48xlarge),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN48xlargeHighMemory),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN48xlargeOptimizedCpu),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN48xlargeOptimizedMemory),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN4xlarge),
						string(api.V1CreateProjectBodyDesiredInstanceSizeN8xlarge),
						string(api.V1CreateProjectBodyDesiredInstanceSizeNano),
						string(api.V1CreateProjectBodyDesiredInstanceSizePico),
						string(api.V1CreateProjectBodyDesiredInstanceSizeSmall),
						string(api.V1CreateProjectBodyDesiredInstanceSizeXlarge),
					),
				},
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

	tflog.Trace(ctx, "create project")
	resp.Diagnostics.Append(createProject(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read up to date project")
	resp.Diagnostics.Append(readProject(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

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
	var plan, state ProjectResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// required attributes
	if !plan.Name.Equal(state.Name) {
		resp.Diagnostics.Append(updateName(ctx, &plan, r.client)...)
	}
	if !plan.DatabasePassword.Equal(state.DatabasePassword) {
		resp.Diagnostics.Append(updateDatabasePassword(ctx, &plan, r.client)...)
	}
	if !plan.Region.Equal(state.Region) {
		resp.Diagnostics.AddAttributeError(path.Root("region"), "Client Error", "Update is not supported for this attribute")
		return
	}
	if !plan.OrganizationId.Equal(state.OrganizationId) {
		resp.Diagnostics.AddAttributeError(path.Root("organization_id"), "Client Error", "Update is not supported for this attribute")
		return
	}

	// optional attributes
	if !plan.InstanceSize.IsNull() && !plan.InstanceSize.Equal(state.InstanceSize) {
		resp.Diagnostics.Append(updateInstanceSize(ctx, &plan, r.client)...)
	}

	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
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
	regionSelection := api.V1CreateProjectBodyRegionSelection0{
		Type: api.Specific,
		Code: api.V1CreateProjectBodyRegionSelection0Code(data.Region.ValueString()),
	}

	region := api.V1CreateProjectBody_RegionSelection{}
	if err := region.FromV1CreateProjectBodyRegionSelection0(regionSelection); err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"Internal Error",
			fmt.Sprintf("Failed to configure region selection: %s", err),
		)}
	}
	body := api.V1CreateAProjectJSONRequestBody{
		OrganizationSlug: data.OrganizationId.ValueString(),
		Name:             data.Name.ValueString(),
		DbPass:           data.DatabasePassword.ValueString(),
		RegionSelection:  &region,
	}
	if !data.InstanceSize.IsUnknown() && !data.InstanceSize.IsNull() {
		body.DesiredInstanceSize = Ptr(api.V1CreateProjectBodyDesiredInstanceSize(data.InstanceSize.ValueString()))
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

	// Wait for project to be fully provisioned
	if diags := waitForProjectActive(ctx, data.Id.ValueString(), client); diags.HasError() {
		return diags
	}

	return nil
}

func readProject(ctx context.Context, data *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	projectResp, err := client.V1GetProjectWithResponse(ctx, data.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read project, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if projectResp.StatusCode() == http.StatusNotFound {
		return nil
	}

	if projectResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read project, got status %d: %s", projectResp.StatusCode(), projectResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	project := projectResp.JSON200
	data.OrganizationId = types.StringValue(project.OrganizationId)
	data.Name = types.StringValue(project.Name)
	data.Region = types.StringValue(project.Region)
	data.InstanceSize = types.StringNull()

	addonsResp, err := client.V1ListProjectAddonsWithResponse(ctx, project.Id)
	if err != nil {
		msg := fmt.Sprintf("Unable to read project addons, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if addonsResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read project addons, got error: %s", string(addonsResp.Body))
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	for _, addon := range addonsResp.JSON200.SelectedAddons {
		if addon.Type != api.ComputeInstance {
			continue
		}

		val, err := addon.Variant.Id.AsListProjectAddonsResponseSelectedAddonsVariantId0()
		if err != nil {
			msg := fmt.Sprintf("Unable to read compute instance addon, got error: %s", err)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}

		data.InstanceSize = types.StringValue(strings.TrimPrefix(string(val), "ci_"))
		break
	}

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

func updateInstanceSize(ctx context.Context, plan *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	addon := api.ApplyProjectAddonBody_AddonVariant{}
	variant := api.ApplyProjectAddonBodyAddonVariant0("ci_" + plan.InstanceSize.ValueString())
	if err := addon.FromApplyProjectAddonBodyAddonVariant0(variant); err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"Internal Error",
			fmt.Sprintf("Failed to configure instance size: %s", err),
		)}
	}
	body := api.V1ApplyProjectAddonJSONRequestBody{
		AddonType:    api.ApplyProjectAddonBodyAddonTypeComputeInstance,
		AddonVariant: addon,
	}

	httpResp, err := client.V1ApplyProjectAddonWithResponse(ctx, plan.Id.ValueString(), body)
	if err != nil {
		msg := fmt.Sprintf("Unable to update project, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to update project, got error: %s", string(httpResp.Body))
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	// Wait for project to be active after resize
	if diags := waitForProjectActive(ctx, plan.Id.ValueString(), client); diags.HasError() {
		return diags
	}

	return nil
}

func updateName(ctx context.Context, plan *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1UpdateAProjectWithResponse(ctx, plan.Id.ValueString(), api.V1UpdateProjectBody{
		Name: plan.Name.ValueString(),
	})
	if err != nil {
		msg := fmt.Sprintf("Unable to update project name, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to update project name, got error: %s", string(httpResp.Body))
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	return nil
}

func updateDatabasePassword(ctx context.Context, plan *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1UpdateDatabasePasswordWithResponse(ctx, plan.Id.ValueString(), api.V1UpdatePasswordBody{
		Password: plan.DatabasePassword.ValueString(),
	})
	if err != nil {
		msg := fmt.Sprintf("Unable to update database password, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to update database password, got error: %s", string(httpResp.Body))
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	return nil
}
