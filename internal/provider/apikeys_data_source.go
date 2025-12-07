// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &APIKeysDataSource{}

func NewAPIKeysDataSource() datasource.DataSource {
	return &APIKeysDataSource{}
}

// APIKeysDataSource defines the data source implementation.
type APIKeysDataSource struct {
	client *api.ClientWithResponses
}

// APIKeysDataSourceModel describes the data source data model.
type APIKeysDataSourceModel struct {
	ProjectRef     types.String `tfsdk:"project_ref"`
	AnonKey        types.String `tfsdk:"anon_key"`
	ServiceRoleKey types.String `tfsdk:"service_role_key"`
	PublishableKey types.String `tfsdk:"publishable_key"`
	SecretKeys     types.List   `tfsdk:"secret_keys"`
}

func (d *APIKeysDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apikeys"
}

func (d *APIKeysDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "API Keys data source",

		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID",
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
			"publishable_key": schema.StringAttribute{
				MarkdownDescription: "Publishable API key for the project",
				Computed:            true,
				Sensitive:           true,
			},
			"secret_keys": schema.ListNestedAttribute{
				MarkdownDescription: "List of secret API keys for the project",
				Computed:            true,
				Sensitive:           true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the secret key",
							Computed:            true,
						},
						"api_key": schema.StringAttribute{
							MarkdownDescription: "The secret API key value",
							Computed:            true,
							Sensitive:           true,
						},
					},
				},
			},
		},
	}
}

func (d *APIKeysDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *APIKeysDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data APIKeysDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := d.client.V1GetProjectApiKeysWithResponse(ctx, data.ProjectRef.ValueString(), &api.V1GetProjectApiKeysParams{Reveal: Ptr(true)})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read API keys, got error: %s", err))
		return
	}

	if httpResp.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read API keys, got status %d: %s", httpResp.StatusCode(), httpResp.Body))
		return
	}

	objectType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name":    types.StringType,
			"api_key": types.StringType,
		},
	}
	var secretKeyObjects []attr.Value

	for _, key := range *httpResp.JSON200 {
		if key.Type.IsSpecified() && !key.Type.IsNull() {
			keyType := key.Type.MustGet()

			switch keyType {
			case api.ApiKeyResponseTypeLegacy:
				if key.Name == "anon" {
					data.AnonKey = NullableToString(key.ApiKey)
				}
				if key.Name == "service_role" {
					data.ServiceRoleKey = NullableToString(key.ApiKey)
				}
			case api.ApiKeyResponseTypePublishable:
				data.PublishableKey = NullableToString(key.ApiKey)
			case api.ApiKeyResponseTypeSecret:
				obj, diags := types.ObjectValue(objectType.AttrTypes, map[string]attr.Value{
					"name":    types.StringValue(key.Name),
					"api_key": NullableToString(key.ApiKey),
				})
				if diags.HasError() {
					resp.Diagnostics.Append(diags...)
					return
				}
				secretKeyObjects = append(secretKeyObjects, obj)
			}
		}
	}

	// Build list directly from object values
	secretKeysList, diags := types.ListValue(objectType, secretKeyObjects)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	data.SecretKeys = secretKeysList

	tflog.Trace(ctx, "read API keys")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
