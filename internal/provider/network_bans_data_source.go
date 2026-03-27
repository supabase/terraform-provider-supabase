// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &NetworkBansDataSource{}

func NewNetworkBansDataSource() datasource.DataSource {
	return &NetworkBansDataSource{}
}

// Defines the data source implementation.
type NetworkBansDataSource struct {
	client *api.ClientWithResponses
}

// Describes the data source data model.
type NetworkBansDataSourceModel struct {
	ProjectRef          types.String `tfsdk:"project_ref"`
	BannedIpv4Addresses types.Set    `tfsdk:"banned_ipv4_addresses"`
}

func (d *NetworkBansDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_bans"
}

func (d *NetworkBansDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Network bans data source",

		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID",
				Required:            true,
			},
			"banned_ipv4_addresses": schema.SetAttribute{
				MarkdownDescription: "List of banned IPv4 addresses",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *NetworkBansDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		d.client = client
	}
}

func (d *NetworkBansDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NetworkBansDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := d.client.V1ListAllNetworkBansWithResponse(ctx, data.ProjectRef.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read network bans, got error: %s", err))
		return
	}

	// API uses POST and returns 201 despite being a read operation
	if httpResp.JSON201 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read network bans, got status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}

	addresses := httpResp.JSON201.BannedIpv4Addresses
	if addresses == nil {
		addresses = []string{}
	}

	bannedAddresses, diags := types.SetValueFrom(ctx, types.StringType, addresses)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	data.BannedIpv4Addresses = bannedAddresses

	tflog.Trace(ctx, "read network bans data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
