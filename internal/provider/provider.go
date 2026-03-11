// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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

	tflog.Debug(ctx, "supabase_provider configure")

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	// Configuration values are now available.

	// Validate endpoint
	if data.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Unknown Supabase API Endpoint",
			"The provider cannot create the Supabase API client as there is an unknown configuration value for the Supbase API endpoint. "+
				"Either target apply the source of the value first, set the value statically in the configuration, use the SUPABASE_API_ENDPOINT environment variable."+
				", or use none of these options to let the endpoint default to https://api.supabase.com",
		)
	}
	apiEndpoint := os.Getenv("SUPABASE_API_ENDPOINT")
	if !data.Endpoint.IsNull() {
		apiEndpoint = data.Endpoint.ValueString()
	}
	if apiEndpoint == "" {
		apiEndpoint = "https://api.supabase.com"
	}

	// Validate access_token
	if data.AccessToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("access_token"),
			"Unknown Supabase API Access Token",
			"The provider cannot create the Supabase API client as there is an unknown configuration value for the Supbase API access token. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the SUPABASE_ACCESS_TOKEN environment variable.",
		)
	}
	accessToken := os.Getenv("SUPABASE_ACCESS_TOKEN")
	if !data.AccessToken.IsNull() {
		accessToken = data.AccessToken.ValueString()
	}
	if accessToken == "" {
		resp.Diagnostics.AddAttributeError(path.Root("access_token"),
			"Missing Supabase API Access Token",
			"Set the access token using either the access_token parameter or the SUPABASE_ACCESS_TOKEN environment variable")
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Example client configuration for data sources and resources
	client, _ := api.NewClientWithResponses(
		apiEndpoint,
		api.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			req.Header.Set("User-Agent", "SupabsaeTerraformProvider/"+p.version)
			return nil
		}),
	)

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *SupabaseProvider) Resources(ctx context.Context) []func() resource.Resource {
	tflog.Debug(ctx, "supabase_provider returning resources")
	return []func() resource.Resource{
		NewProjectResource,
		NewSettingsResource,
		NewBranchResource,
		NewEdgeFunctionResource,
		NewApiKeyResource,
	}
}

func (p *SupabaseProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	tflog.Debug(ctx, "supabase_provider returning data sources")
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
