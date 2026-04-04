// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
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

const defaultApiEndpoint = "https://api.supabase.com"

// Stores the HTTP method in request context so transport-error
// retries can still distinguish GET requests.
type retryMethodKey struct{}

const maxRetryAfterWait = 10 * time.Second

// Caps Retry-After delays so refresh does not stall for too long.
func cappedJitterBackoff(minWait, maxWait time.Duration, attemptNum int, resp *http.Response) time.Duration {
	wait := retryablehttp.RateLimitLinearJitterBackoff(minWait, maxWait, attemptNum, resp)
	if wait > maxRetryAfterWait {
		return maxRetryAfterWait
	}
	return wait
}

// Retries transient GET failures only. Writes are never retried.
func getOnlyRetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		// A response arrived, return nil so the body reaches the caller.
		// Returning ctx.Err() causes the generated helpers to discard it.
		if resp != nil {
			return false, nil
		}
		return false, ctx.Err()
	}

	method := ""
	if resp != nil && resp.Request != nil {
		method = resp.Request.Method
	} else if m, ok := ctx.Value(retryMethodKey{}).(string); ok {
		method = m
	}

	if method != http.MethodGet {
		return false, nil
	}

	if err != nil {
		return retryablehttp.ErrorPropagatedRetryPolicy(ctx, nil, err)
	}

	// 408, 429, 500, 502, 503, 504
	switch resp.StatusCode {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true, nil
	}

	return false, nil
}

// Records the HTTP method so transport-error retries can
// still gate on GET versus write requests.
func stashRequestMethod(_ context.Context, req *http.Request) error {
	*req = *req.WithContext(context.WithValue(req.Context(), retryMethodKey{}, req.Method))
	return nil
}

// Sends GETs through retryablehttp and bypasses it for
// writes, which avoids extra body buffering on non-retried requests.
type retryGETTransport struct {
	// used for GET only
	retrying http.RoundTripper
	// used for all writes, no buffering
	plain http.RoundTripper
}

func (t *retryGETTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodGet {
		return t.retrying.RoundTrip(req)
	}
	return t.plain.RoundTrip(req)
}

func newRetryableClient() *http.Client {
	rc := retryablehttp.NewClient()
	// keep gock-based tests intercepting requests
	rc.HTTPClient = http.DefaultClient
	rc.RetryMax = 3
	rc.RetryWaitMin = 500 * time.Millisecond
	rc.RetryWaitMax = 1500 * time.Millisecond
	rc.Logger = nil
	rc.CheckRetry = getOnlyRetryPolicy
	rc.Backoff = cappedJitterBackoff
	rc.ErrorHandler = retryablehttp.PassthroughErrorHandler
	return &http.Client{
		Transport: &retryGETTransport{
			retrying: rc.StandardClient().Transport,
			plain:    http.DefaultTransport,
		},
	}
}

func (p *SupabaseProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "supabase"
	resp.Version = p.version
}

func (p *SupabaseProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Supabase API endpoint. Can also be set via the `SUPABASE_API_ENDPOINT` " +
					"environment variable. If neither is set, defaults to `https://api.supabase.com`. " +
					"When both are specified, the provider configuration takes precedence over the environment variable.",
				Optional: true,
			},
			"access_token": schema.StringAttribute{
				MarkdownDescription: "Supabase access token. Can also be set via the `SUPABASE_ACCESS_TOKEN` " +
					"environment variable. When both are specified, the provider configuration takes precedence " +
					"over the environment variable. Generate a token from the " +
					"[Supabase Dashboard](https://supabase.com/dashboard/account/tokens).",
				Optional:  true,
				Sensitive: true,
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
			"The provider cannot create the Supabase API client as there is an unknown configuration value for the Supabase API endpoint. "+
				"Either target apply the source of the value first, set the value statically in the configuration, use the SUPABASE_API_ENDPOINT environment variable"+
				", or use none of these options to let the endpoint default to https://api.supabase.com",
		)
	}
	apiEndpoint := os.Getenv("SUPABASE_API_ENDPOINT")
	if !data.Endpoint.IsNull() {
		apiEndpoint = data.Endpoint.ValueString()
	}
	if apiEndpoint == "" {
		apiEndpoint = defaultApiEndpoint
	}

	// Validate access_token
	if data.AccessToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("access_token"),
			"Unknown Supabase API Access Token",
			"The provider cannot create the Supabase API client as there is an unknown configuration value for the Supabase API access token. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the SUPABASE_ACCESS_TOKEN environment variable.",
		)
	}
	accessToken := os.Getenv("SUPABASE_ACCESS_TOKEN")
	if !data.AccessToken.IsNull() {
		accessToken = data.AccessToken.ValueString()
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		resp.Diagnostics.AddAttributeError(path.Root("access_token"),
			"Missing Supabase API Access Token",
			"Set the access token using either the access_token parameter or the SUPABASE_ACCESS_TOKEN environment variable")
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Example client configuration for data sources and resources
	client, err := api.NewClientWithResponses(
		apiEndpoint,
		api.WithHTTPClient(newRetryableClient()),
		api.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			req.Header.Set("User-Agent", "TFProvider/"+p.version)
			return nil
		}),
		api.WithRequestEditorFn(stashRequestMethod),
	)
	if err != nil {
		tflog.Error(ctx, "NewClientWithResponses Error: "+err.Error())
		resp.Diagnostics.AddError(
			"NewClientWithResponses Failed, API is not usable.",
			err.Error(),
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

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
		NewEdgeFunctionSecretsResource,
		NewApiKeyResource,
	}
}

func (p *SupabaseProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	tflog.Debug(ctx, "supabase_provider returning data sources")
	return []func() datasource.DataSource{
		NewBranchDataSource,
		NewPoolerDataSource,
		NewAPIKeysDataSource,
		NewNetworkBansDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SupabaseProvider{
			version: version,
		}
	}
}
