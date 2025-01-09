// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/supabase/cli/pkg/api"
)

// Ensure SupabaseProvider satisfies various provider interfaces.
var _ provider.Provider = &SupabaseProvider{}

// SupabaseProvider defines the provider implementation.
type SupabaseProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// SupabaseProviderModel describes the provider data model.
type SupabaseProviderModel struct {
	Endpoint    types.String `tfsdk:"endpoint"`
	AccessToken types.String `tfsdk:"access_token"`
}

func (p *SupabaseProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "supabase"
	resp.Version = p.version
}

func (p *SupabaseProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Supabase API endpoint",
				Optional:            true,
			},
			"access_token": schema.StringAttribute{
				MarkdownDescription: "Supabase access token",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *SupabaseProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data SupabaseProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Configuration values are now available.
	if data.Endpoint.IsNull() {
		data.Endpoint = types.StringValue("https://api.supabase.com")
	}
	if data.AccessToken.IsNull() {
		data.AccessToken = types.StringValue(os.Getenv("SUPABASE_ACCESS_TOKEN"))
	}

	// Example client configuration for data sources and resources
	client, _ := api.NewClientWithResponses(
		data.Endpoint.ValueString(),
		api.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+data.AccessToken.ValueString())
			req.Header.Set("User-Agent", "TFProvider/"+p.version)
			return nil
		}),
	)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *SupabaseProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewSettingsResource,
		NewBranchResource,
	}
}

func (p *SupabaseProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewBranchDataSource,
		NewPoolerDataSource,
		NewAPIKeysDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SupabaseProvider{
			version: version,
		}
	}
}
