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

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &ProjectAPIKeysDataSource{}

func NewProjectAPIKeysDataSource() datasource.DataSource {
	return &ProjectAPIKeysDataSource{}
}

// ProjectAPIKeysDataSource defines the data source implementation.
type ProjectAPIKeysDataSource struct {
	client *api.ClientWithResponses
}

// ProjectAPIKeysDataSourceModel describes the data source data model.
type ProjectAPIKeysDataSourceModel struct {
	ProjectId      types.String `tfsdk:"project_id"`
	AnonKey        types.String `tfsdk:"anon_key"`
	ServiceRoleKey types.String `tfsdk:"service_role_key"`
}

func (d *ProjectAPIKeysDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_apikeys"
}

func (d *ProjectAPIKeysDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Project API Keys data source",

		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				MarkdownDescription: "Project identifier",
				Required:            true,
			},
			"anon_key": schema.StringAttribute{
				MarkdownDescription: "Anonymous API key for the project",
				Computed:            true,
				Sensitive:           true,
			},
			"service_role_key": schema.StringAttribute{
				MarkdownDescription: "Service role API key for the project",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}

func (d *ProjectAPIKeysDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ProjectAPIKeysDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ProjectAPIKeysDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := d.client.V1GetProjectApiKeysWithResponse(ctx, data.ProjectId.ValueString(), &api.V1GetProjectApiKeysParams{})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read project API keys, got error: %s", err))
		return
	}

	if httpResp.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read project API keys, got status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}

	for _, key := range *httpResp.JSON200 {
		switch key.Name {
		case "anon":
			data.AnonKey = types.StringValue(key.ApiKey)
		case "service_role":
			data.ServiceRoleKey = types.StringValue(key.ApiKey)
		}
	}

	tflog.Trace(ctx, "read project API keys")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
