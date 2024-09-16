// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &BranchDataSource{}

func NewBranchDataSource() datasource.DataSource {
	return &BranchDataSource{}
}

// BranchDataSource defines the data source implementation.
type BranchDataSource struct {
	client *api.ClientWithResponses
}

// BranchDataSourceModel describes the data source data model.
type BranchDataSourceModel struct {
	ProjectRef types.String `tfsdk:"project_ref"`
	GitBranch  types.String `tfsdk:"git_branch"`
	Id         types.String `tfsdk:"id"`
}

func (d *BranchDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_branch"
}

func (d *BranchDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Branch data source",

		Attributes: map[string]schema.Attribute{
			"parent_project_ref": schema.StringAttribute{
				MarkdownDescription: "Parent project ref",
				Required:            true,
			},
			"branches": schema.SetNestedAttribute{
				MarkdownDescription: "Branch databases",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"project_ref": schema.StringAttribute{
							MarkdownDescription: "Branch project ref",
							Computed:            true,
						},
						"git_branch": schema.StringAttribute{
							MarkdownDescription: "Git branch",
							Computed:            true,
						},
						"id": schema.StringAttribute{
							MarkdownDescription: "Branch identifier",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *BranchDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.ClientWithResponses)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *api.ClientWithResponses, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *BranchDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var projectRef types.String

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("parent_project_ref"), &projectRef)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	httpResp, err := d.client.V1ListAllBranchesWithResponse(ctx, projectRef.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read branch, got error: %s", err)
		resp.Diagnostics.AddError("Client Error", msg)
		return
	}
	// Create an empty array if branching is disabled
	if httpResp.StatusCode() == http.StatusUnprocessableEntity {
		httpResp.JSON200 = &[]api.BranchResponse{}
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read branch, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		resp.Diagnostics.AddError("Client Error", msg)
		return
	}

	branches := make([]BranchDataSourceModel, 0)
	for _, branch := range *httpResp.JSON200 {
		if branch.IsDefault {
			continue
		}
		branches = append(branches, BranchDataSourceModel{
			Id:         types.StringValue(branch.Id),
			GitBranch:  types.StringPointerValue(branch.GitBranch),
			ProjectRef: types.StringValue(branch.ProjectRef),
		})
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("branches"), &branches)...)
}
