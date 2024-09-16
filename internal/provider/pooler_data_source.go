// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &PoolerDataSource{}

func NewPoolerDataSource() datasource.DataSource {
	return &PoolerDataSource{}
}

// PoolerDataSource defines the data source implementation.
type PoolerDataSource struct {
	client *api.ClientWithResponses
}

// PoolerDataSourceModel describes the data source data model.
type PoolerDataSourceModel struct {
	ProjectRef types.String  `tfsdk:"project_ref"`
	Url        types.MapType `tfsdk:"url"`
}

func (d *PoolerDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pooler"
}

func (d *PoolerDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Pooler data source",

		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project ref",
				Required:            true,
			},
			"url": schema.MapAttribute{
				MarkdownDescription: "Map of pooler mode to connection string",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *PoolerDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PoolerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var projectRef types.String

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("project_ref"), &projectRef)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	httpResp, err := d.client.V1GetSupavisorConfigWithResponse(ctx, projectRef.ValueString())
	if err != nil {
		msg := fmt.Sprintf("Unable to read pooler, got error: %s", err)
		resp.Diagnostics.AddError("Client Error", msg)
		return
	}
	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read pooler, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		resp.Diagnostics.AddError("Client Error", msg)
		return
	}

	url := map[string]string{}
	for _, pooler := range *httpResp.JSON200 {
		if pooler.DatabaseType == api.PRIMARY {
			url[string(pooler.PoolMode)] = pooler.ConnectionString
		}
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("url"), url)...)
}
