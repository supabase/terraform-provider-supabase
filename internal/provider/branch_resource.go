// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &BranchResource{}
var _ resource.ResourceWithImportState = &BranchResource{}

func NewBranchResource() resource.Resource {
	return &BranchResource{}
}

// BranchResource defines the resource implementation.
type BranchResource struct {
	client *api.ClientWithResponses
}

type BranchDatabaseModel struct {
	Host      types.String `tfsdk:"host"`
	Password  types.String `tfsdk:"password"`
	Port      types.Int64  `tfsdk:"port"`
	User      types.String `tfsdk:"user"`
	JwtSecret types.String `tfsdk:"jwt_secret"`
	Version   types.String `tfsdk:"version"`
	Status    types.String `tfsdk:"status"`
	Id        types.String `tfsdk:"id"`
}

func (m BranchDatabaseModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"host":       types.StringType,
		"password":   types.StringType,
		"port":       types.Int64Type,
		"user":       types.StringType,
		"jwt_secret": types.StringType,
		"version":    types.StringType,
		"status":     types.StringType,
		"id":         types.StringType,
	}
}

// BranchResourceModel describes the resource data model.
type BranchResourceModel struct {
	GitBranch        types.String `tfsdk:"git_branch"`
	ParentProjectRef types.String `tfsdk:"parent_project_ref"`
	Region           types.String `tfsdk:"region"`
	Database         types.Object `tfsdk:"database"`
	Id               types.String `tfsdk:"id"`
}

func (r *BranchResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_branch"
}

func (r *BranchResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Branch database resource",

		Attributes: map[string]schema.Attribute{
			"git_branch": schema.StringAttribute{
				MarkdownDescription: "Git branch",
				Required:            true,
			},
			"parent_project_ref": schema.StringAttribute{
				MarkdownDescription: "Parent project ref",
				Required:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Database region",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"database": schema.SingleNestedAttribute{
				MarkdownDescription: "Database connection details",
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"host": schema.StringAttribute{
						MarkdownDescription: "Host",
						Computed:            true,
					},
					"port": schema.Int64Attribute{
						MarkdownDescription: "Port",
						Computed:            true,
					},
					"user": schema.StringAttribute{
						MarkdownDescription: "User",
						Computed:            true,
					},
					"password": schema.StringAttribute{
						MarkdownDescription: "Password",
						Sensitive:           true,
						Computed:            true,
					},
					"jwt_secret": schema.StringAttribute{
						MarkdownDescription: "JWT secret",
						Sensitive:           true,
						Computed:            true,
					},
					"version": schema.StringAttribute{
						MarkdownDescription: "Postgres version",
						Computed:            true,
					},
					"status": schema.StringAttribute{
						MarkdownDescription: "Status",
						Computed:            true,
					},
					"id": schema.StringAttribute{
						MarkdownDescription: "Branch project ref",
						Computed:            true,
					},
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Branch identifier",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *BranchResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BranchResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BranchResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(createBranch(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BranchResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BranchResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(readBranch(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BranchResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data BranchResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(updateBranch(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BranchResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data BranchResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deleteBranch(ctx, &data, r.client)...)
}

func (r *BranchResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func updateBranch(ctx context.Context, plan *BranchResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1UpdateABranchConfigWithResponse(ctx, plan.Id.ValueString(), api.UpdateBranchBody{
		BranchName: plan.GitBranch.ValueStringPointer(),
		GitBranch:  plan.GitBranch.ValueStringPointer(),
	})
	if err != nil {
		msg := fmt.Sprintf("Unable to update branch, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to update branch, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	plan.ParentProjectRef = types.StringValue(httpResp.JSON200.ParentProjectRef)
	plan.GitBranch = types.StringPointerValue(httpResp.JSON200.GitBranch)
	if diag := readBranchDatabase(ctx, plan, client); diag.HasError() {
		for _, err := range diag.Errors() {
			tflog.Warn(ctx, fmt.Sprintf("%s: %s", err.Summary(), err.Detail()))
		}
	}
	return nil
}

func readBranch(ctx context.Context, state *BranchResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	return readBranchDatabase(ctx, state, client)
}

func readBranchDatabase(ctx context.Context, state *BranchResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1GetABranchConfigWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read branch database, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read branch database, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	database := BranchDatabaseModel{
		Id:        types.StringValue(httpResp.JSON200.Ref),
		Host:      types.StringValue(httpResp.JSON200.DbHost),
		Port:      types.Int64Value(int64(httpResp.JSON200.DbPort)),
		User:      types.StringPointerValue(httpResp.JSON200.DbUser),
		Password:  types.StringPointerValue(httpResp.JSON200.DbPass),
		JwtSecret: types.StringPointerValue(httpResp.JSON200.JwtSecret),
		Version:   types.StringValue(httpResp.JSON200.PostgresVersion),
		Status:    types.StringValue(string(httpResp.JSON200.Status)),
	}
	db, diag := types.ObjectValueFrom(ctx, database.AttributeTypes(), database)
	state.Database = db
	return diag
}

func createBranch(ctx context.Context, plan *BranchResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	resp, err := client.V1ListAllBranches(ctx, plan.ParentProjectRef.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to enable branching, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// 1. Enable branching
	if resp.StatusCode == http.StatusUnprocessableEntity {
		httpResp, err := client.V1CreateABranchWithResponse(ctx, plan.ParentProjectRef.ValueString(), api.CreateBranchBody{
			BranchName: "Production",
		})
		if err != nil {
			msg := fmt.Sprintf("Unable to enable branching, got error: %s", err)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}
		if httpResp.JSON201 == nil {
			msg := fmt.Sprintf("Unable to enable branching, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}
	}
	// 2. Create branch database
	httpResp, err := client.V1CreateABranchWithResponse(ctx, plan.ParentProjectRef.ValueString(), api.CreateBranchBody{
		BranchName: plan.GitBranch.ValueString(),
		GitBranch:  plan.GitBranch.ValueStringPointer(),
		Region:     plan.Region.ValueStringPointer(),
	})
	if err != nil {
		msg := fmt.Sprintf("Unable to create branch, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.JSON201 == nil {
		msg := fmt.Sprintf("Unable to create branch, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	// Update computed fields
	plan.Id = types.StringValue(httpResp.JSON201.Id)
	if diag := readBranchDatabase(ctx, plan, client); diag.HasError() {
		for _, err := range diag.Errors() {
			tflog.Warn(ctx, fmt.Sprintf("%s: %s", err.Summary(), err.Detail()))
		}
	}
	return nil
}

func deleteBranch(ctx context.Context, state *BranchResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1DeleteABranchWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to delete branch, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to delete branch, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	return nil
}
