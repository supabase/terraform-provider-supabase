// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
)

func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

// ProjectResource defines the resource implementation.
type ProjectResource struct {
	client *api.ClientWithResponses
}

// ProjectResourceModel describes the resource data model.
type ProjectResourceModel struct {
	OrganizationId            types.String   `tfsdk:"organization_id"`
	Name                      types.String   `tfsdk:"name"`
	DatabasePassword          types.String   `tfsdk:"database_password"`
	DatabasePasswordWo        types.String   `tfsdk:"database_password_wo"`
	DatabasePasswordWoVersion types.Int64    `tfsdk:"database_password_wo_version"`
	Region                    types.String   `tfsdk:"region"`
	InstanceSize              types.String   `tfsdk:"instance_size"`
	Id                        types.String   `tfsdk:"id"`
	LegacyApiKeysEnabled      types.Bool     `tfsdk:"legacy_api_keys_enabled"`
	Timeouts                  timeouts.Value `tfsdk:"timeouts"`
}

func (r *ProjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Project resource",

		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
			}),
		},
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
				MarkdownDescription: "Password for the project database. Exactly one of `database_password` or " +
					"`database_password_wo` must be set. This value is persisted in Terraform state in plaintext; " +
					"prefer `database_password_wo` to keep it out of state.",
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(4),
					stringvalidator.ExactlyOneOf(path.MatchRoot("database_password_wo")),
				},
			},
			"database_password_wo": schema.StringAttribute{
				MarkdownDescription: "Write-only password for the project database, for example `ephemeral.random_password.db.result`. " +
					"Exactly one of `database_password` or `database_password_wo` must be set. Unlike `database_password` " +
					"this value is never persisted to Terraform state. Must be paired with `database_password_wo_version`, " +
					"which is what triggers a rotation.",
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(4),
					stringvalidator.AlsoRequires(path.MatchRoot("database_password_wo_version")),
				},
			},
			"database_password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version counter for `database_password_wo`. Increment it to rotate the database password. " +
					"A write-only value is absent from state, so this counter is what the provider compares to detect a rotation.",
				Optional: true,
				Validators: []validator.Int64{
					int64validator.AlsoRequires(path.MatchRoot("database_password_wo")),
				},
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
			"legacy_api_keys_enabled": schema.BoolAttribute{
				MarkdownDescription: "Controls whether `anon` and `service_role` JWT-based api keys should be enabled. " +
					"Please note: these keys are no longer recommended " +
					"([more information here](https://supabase.com/docs/guides/api/api-keys#why-are-anon-and-servicerole-jwt-based-keys-no-longer-recommended)).",
				DeprecationMessage: "Deprecated. This field will be removed once the transition to publishable and secret keys is complete.",
				Optional:           true,
				Computed:           true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ProjectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		r.client = client
	}
}

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ProjectResourceModel

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

	dbPass, diags := resolveDatabasePassword(ctx, req.Config, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "create project")
	resp.Diagnostics.Append(createProject(ctx, &data, r.client, createTimeout, dbPass)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.LegacyApiKeysEnabled.IsNull() && !data.LegacyApiKeysEnabled.IsUnknown() {
		resp.Diagnostics.Append(updateLegacyAPIKeysEnabled(ctx, &data, r.client)...)
		if resp.Diagnostics.HasError() {
			return
		}
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

	updateTimeout, diags := plan.Timeouts.Update(ctx, defaultWaitTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// required attributes
	if !plan.Name.Equal(state.Name) {
		resp.Diagnostics.Append(updateName(ctx, &plan, r.client)...)
	}
	if databasePasswordChanged(&plan, &state) {
		dbPass, diags := resolveDatabasePassword(ctx, req.Config, &plan)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(updateDatabasePassword(ctx, &plan, r.client, dbPass)...)
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
		resp.Diagnostics.Append(updateInstanceSize(ctx, &plan, r.client, updateTimeout)...)
	}
	if !plan.LegacyApiKeysEnabled.IsNull() && !plan.LegacyApiKeysEnabled.Equal(state.LegacyApiKeysEnabled) {
		resp.Diagnostics.Append(updateLegacyAPIKeysEnabled(ctx, &plan, r.client)...)
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

func createProject(ctx context.Context, data *ProjectResourceModel, client *api.ClientWithResponses, timeout time.Duration, dbPass string) diag.Diagnostics {
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
		DbPass:           dbPass,
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
	if diags := waitForProjectActive(ctx, data.Id.ValueString(), client, timeout); diags.HasError() {
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

	legacyKeysResp, err := client.V1GetProjectLegacyApiKeysWithResponse(ctx, project.Id)
	if err != nil {
		msg := fmt.Sprintf("Unable to read project legacy api keys state, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if legacyKeysResp.JSON200 == nil {
		// This API endpoint will be removed in the future, so explicitly check for HTTP 404 Not Found.
		if legacyKeysResp.StatusCode() != http.StatusNotFound {
			msg := fmt.Sprintf("Unable to read project legacy api keys, got status %d: %s", legacyKeysResp.StatusCode(), legacyKeysResp.Body)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}

		data.LegacyApiKeysEnabled = types.BoolValue(false)
	} else {
		data.LegacyApiKeysEnabled = types.BoolValue(legacyKeysResp.JSON200.Enabled)
	}

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
		if addon.Type != api.ListProjectAddonsResponseSelectedAddonsTypeComputeInstance {
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

func updateInstanceSize(ctx context.Context, plan *ProjectResourceModel, client *api.ClientWithResponses, timeout time.Duration) diag.Diagnostics {
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
	if diags := waitForProjectActive(ctx, plan.Id.ValueString(), client, timeout); diags.HasError() {
		return diags
	}

	return nil
}

func updateLegacyAPIKeysEnabled(ctx context.Context, plan *ProjectResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1UpdateProjectLegacyApiKeysWithResponse(ctx, plan.Id.ValueString(), &api.V1UpdateProjectLegacyApiKeysParams{
		Enabled: plan.LegacyApiKeysEnabled.ValueBool(),
	})
	if err != nil {
		msg := fmt.Sprintf("Unable to update legacy api keys, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to update legacy api keys, got error: %s", string(httpResp.Body))
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
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

func updateDatabasePassword(ctx context.Context, plan *ProjectResourceModel, client *api.ClientWithResponses, dbPass string) diag.Diagnostics {
	httpResp, err := client.V1UpdateDatabasePasswordWithResponse(ctx, plan.Id.ValueString(), api.V1UpdatePasswordBody{
		Password: dbPass,
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

// databasePasswordChanged reports whether the project password must be pushed to
// the API. A write-only value is absent from state, so the _wo path is driven by
// its version counter instead of by comparing the secret itself.
func databasePasswordChanged(plan, state *ProjectResourceModel) bool {
	if !plan.DatabasePasswordWoVersion.IsNull() {
		return !plan.DatabasePasswordWoVersion.Equal(state.DatabasePasswordWoVersion)
	}
	return !plan.DatabasePassword.Equal(state.DatabasePassword)
}

// resolveDatabasePassword returns the password to send to the API. Write-only
// attributes are nulled out in plan and state by the framework, so the _wo value
// can only be read from config.
func resolveDatabasePassword(ctx context.Context, cfg tfsdk.Config, data *ProjectResourceModel) (string, diag.Diagnostics) {
	if !data.DatabasePassword.IsNull() {
		return data.DatabasePassword.ValueString(), nil
	}
	var wo types.String
	diags := cfg.GetAttribute(ctx, path.Root("database_password_wo"), &wo)
	return wo.ValueString(), diags
}
