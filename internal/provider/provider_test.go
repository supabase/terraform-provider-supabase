// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"testing"

  "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"supabase": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.

	// Setting an access token is required now because it is validated in the
	// Configure function in provider.go
	if os.Getenv("SUPABASE_ACCESS_TOKEN") == "" {
		t.Setenv("SUPABASE_ACCESS_TOKEN", "test")
	}
}

// getUnderlyingClient extracts the underlying *api.Client from a ClientWithResponses using reflection.
func getUnderlyingClient(t *testing.T, client *api.ClientWithResponses) *api.Client {
	t.Helper()
	v := reflect.ValueOf(client).Elem()
	clientInterfaceField := v.FieldByName("ClientInterface")
	if !clientInterfaceField.IsValid() {
		t.Fatal("Cannot access ClientInterface field")
	}
	underlyingClient, ok := clientInterfaceField.Interface().(*api.Client)
	if !ok {
		t.Fatal("ClientInterface is not a *api.Client")
	}
	return underlyingClient
}

// getClientEndpoint extracts the server endpoint URL from the API client using reflection.
func getClientEndpoint(t *testing.T, client *api.Client) string {
	t.Helper()
	serverField := reflect.ValueOf(client).Elem().FieldByName("Server")
	if !serverField.IsValid() {
		t.Fatal("Cannot access Server field")
	}
	return serverField.String()
}

// getAuthorizationHeader applies the client's request editors to a mock request
// and returns the Authorization header value.
func getAuthorizationHeader(t *testing.T, client *api.Client) string {
	t.Helper()

	// Create a mock request to apply the request editors to
	mockReq, err := http.NewRequest("GET", "https://api.supabase.com/test", nil)
	if err != nil {
		t.Fatalf("Failed to create mock request: %v", err)
	}

	// Apply the request editors to see what Authorization header they set
	requestEditorsField := reflect.ValueOf(client).Elem().FieldByName("RequestEditors")
	if !requestEditorsField.IsValid() {
		t.Fatal("Cannot access RequestEditors field")
	}

	requestEditors, ok := requestEditorsField.Interface().([]api.RequestEditorFn)
	if !ok {
		t.Fatal("RequestEditors is not a []api.RequestEditorFn")
	}
	for _, editor := range requestEditors {
		if err := editor(context.Background(), mockReq); err != nil {
			t.Fatalf("Request editor failed: %v", err)
		}
	}

	return mockReq.Header.Get("Authorization")
}

// TestProviderConfigure_AccessTokenRequired tests that an error is returned when
// neither the access_token configuration nor the SUPABASE_ACCESS_TOKEN environment
// variable is set.
func TestProviderConfigure_AccessTokenRequired(t *testing.T) {
	// Ensure no access token is set
	t.Setenv("SUPABASE_ACCESS_TOKEN", "")

	p := &SupabaseProvider{version: "test"}

	// Get the schema to create a proper config
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	// Create an empty configuration (no access_token set)
	configValue := tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"endpoint":     tftypes.String,
			"access_token": tftypes.String,
		},
	}, map[string]tftypes.Value{
		"endpoint":     tftypes.NewValue(tftypes.String, nil),
		"access_token": tftypes.NewValue(tftypes.String, nil),
	})

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}

	req := provider.ConfigureRequest{
		Config: config,
	}
	resp := &provider.ConfigureResponse{}

	p.Configure(context.Background(), req, resp)

	// Should have an error diagnostic
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when access token is missing, but got none")
	}

	// Verify the error is about missing access token
	diags := resp.Diagnostics.Errors()
	if len(diags) == 0 {
		t.Fatal("Expected error diagnostic")
	}
	if diags[0].Summary() != "Missing Supabase API Access Token" {
		t.Errorf("Expected 'Missing Supabase API Access Token' error, got: %s", diags[0].Summary())
	}
}

// TestProviderConfigure_EnvVarOnly tests that setting only the SUPABASE_ACCESS_TOKEN
// environment variable successfully configures the provider.
func TestProviderConfigure_EnvVarOnly(t *testing.T) {
	// Set only the environment variable
	t.Setenv("SUPABASE_ACCESS_TOKEN", "test-token-from-env")

	p := &SupabaseProvider{version: "test"}

	// Get the schema
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	// Create an empty configuration (no access_token set in config)
	configValue := tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"endpoint":     tftypes.String,
			"access_token": tftypes.String,
		},
	}, map[string]tftypes.Value{
		"endpoint":     tftypes.NewValue(tftypes.String, nil),
		"access_token": tftypes.NewValue(tftypes.String, nil),
	})

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}

	req := provider.ConfigureRequest{
		Config: config,
	}
	resp := &provider.ConfigureResponse{}

	p.Configure(context.Background(), req, resp)

	// Should not have errors
	if resp.Diagnostics.HasError() {
		t.Fatalf("Unexpected error: %v", resp.Diagnostics.Errors())
	}

	// Verify client was configured
	if resp.ResourceData == nil {
		t.Error("Expected ResourceData to be set")
	}
	if resp.DataSourceData == nil {
		t.Error("Expected DataSourceData to be set")
	}
}

// TestProviderConfigure_ConfigOnly tests that setting only the access_token
// in the provider configuration successfully configures the provider.
func TestProviderConfigure_ConfigOnly(t *testing.T) {
	// Ensure environment variable is not set
	t.Setenv("SUPABASE_ACCESS_TOKEN", "")

	p := &SupabaseProvider{version: "test"}

	// Get the schema
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	// Create configuration with access_token set
	configValue := tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"endpoint":     tftypes.String,
			"access_token": tftypes.String,
		},
	}, map[string]tftypes.Value{
		"endpoint":     tftypes.NewValue(tftypes.String, nil),
		"access_token": tftypes.NewValue(tftypes.String, "test-token-from-config"),
	})

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}

	req := provider.ConfigureRequest{
		Config: config,
	}
	resp := &provider.ConfigureResponse{}

	p.Configure(context.Background(), req, resp)

	// Should not have errors
	if resp.Diagnostics.HasError() {
		t.Fatalf("Unexpected error: %v", resp.Diagnostics.Errors())
	}

	// Verify client was configured
	if resp.ResourceData == nil {
		t.Error("Expected ResourceData to be set")
	}
	if resp.DataSourceData == nil {
		t.Error("Expected DataSourceData to be set")
	}
}

// TestProviderConfigure_ConfigTakesPrecedence tests that when both the
// access_token configuration and SUPABASE_ACCESS_TOKEN environment variable
// are set, the configuration value takes precedence.
func TestProviderConfigure_ConfigTakesPrecedence(t *testing.T) {
	// Set environment variable to one value
	t.Setenv("SUPABASE_ACCESS_TOKEN", "env-token")

	p := &SupabaseProvider{version: "test"}

	// Get the schema
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	// Create configuration with different access_token (this should take precedence)
	configValue := tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"endpoint":     tftypes.String,
			"access_token": tftypes.String,
		},
	}, map[string]tftypes.Value{
		"endpoint":     tftypes.NewValue(tftypes.String, nil),
		"access_token": tftypes.NewValue(tftypes.String, "config-token"),
	})

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}

	req := provider.ConfigureRequest{
		Config: config,
	}
	resp := &provider.ConfigureResponse{}

	p.Configure(context.Background(), req, resp)

	// Should not have errors
	if resp.Diagnostics.HasError() {
		t.Fatalf("Unexpected error: %v", resp.Diagnostics.Errors())
	}

	// Verify client was configured
	if resp.DataSourceData == nil {
		t.Fatal("Expected DataSourceData to be set")
	}

	// Cast to the API client type
	client, ok := resp.DataSourceData.(*api.ClientWithResponses)
	if !ok {
		t.Fatal("DataSourceData is not a *api.ClientWithResponses")
	}

	// Get the underlying client and verify the Authorization header
	underlyingClient := getUnderlyingClient(t, client)
	authHeader := getAuthorizationHeader(t, underlyingClient)

	expectedAuth := "Bearer config-token"
	if authHeader != expectedAuth {
		t.Errorf("Expected Authorization header %q, got %q (env token would be 'Bearer env-token')",
			expectedAuth, authHeader)
	}
}

// TestProviderConfigure_EndpointDefault tests that when neither the endpoint
// configuration nor the SUPABASE_API_ENDPOINT environment variable is set,
// the endpoint defaults to "https://api.supabase.com".
func TestProviderConfigure_EndpointDefault(t *testing.T) {
	// Ensure environment variable is not set
	t.Setenv("SUPABASE_API_ENDPOINT", "")
	// Set access token to satisfy validation
	t.Setenv("SUPABASE_ACCESS_TOKEN", "test-token")

	p := &SupabaseProvider{version: "test"}

	// Get the schema
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	// Create configuration without endpoint set
	configValue := tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"endpoint":     tftypes.String,
			"access_token": tftypes.String,
		},
	}, map[string]tftypes.Value{
		"endpoint":     tftypes.NewValue(tftypes.String, nil),
		"access_token": tftypes.NewValue(tftypes.String, nil),
	})

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}

	req := provider.ConfigureRequest{
		Config: config,
	}
	resp := &provider.ConfigureResponse{}

	p.Configure(context.Background(), req, resp)

	// Should not have errors
	if resp.Diagnostics.HasError() {
		t.Fatalf("Unexpected error: %v", resp.Diagnostics.Errors())
	}

	// Verify client was configured
	if resp.DataSourceData == nil {
		t.Fatal("Expected DataSourceData to be set")
	}

	// Cast to the API client type
	client, ok := resp.DataSourceData.(*api.ClientWithResponses)
	if !ok {
		t.Fatal("DataSourceData is not a *api.ClientWithResponses")
	}

	// Get the endpoint from the client
	underlyingClient := getUnderlyingClient(t, client)
	endpoint := getClientEndpoint(t, underlyingClient)

	expectedEndpoint := "https://api.supabase.com/"
	if endpoint != expectedEndpoint {
		t.Errorf("Expected endpoint %q, got %q", expectedEndpoint, endpoint)
	}
}

// TestProviderConfigure_EndpointEnvVarOnly tests that setting only the
// SUPABASE_API_ENDPOINT environment variable successfully configures the provider.
func TestProviderConfigure_EndpointEnvVarOnly(t *testing.T) {
	// Set only the environment variable
	t.Setenv("SUPABASE_API_ENDPOINT", "https://api.env-endpoint.com")
	// Set access token to satisfy validation
	t.Setenv("SUPABASE_ACCESS_TOKEN", "test-token")

	p := &SupabaseProvider{version: "test"}

	// Get the schema
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	// Create configuration without endpoint set in config
	configValue := tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"endpoint":     tftypes.String,
			"access_token": tftypes.String,
		},
	}, map[string]tftypes.Value{
		"endpoint":     tftypes.NewValue(tftypes.String, nil),
		"access_token": tftypes.NewValue(tftypes.String, nil),
	})

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}

	req := provider.ConfigureRequest{
		Config: config,
	}
	resp := &provider.ConfigureResponse{}

	p.Configure(context.Background(), req, resp)

	// Should not have errors
	if resp.Diagnostics.HasError() {
		t.Fatalf("Unexpected error: %v", resp.Diagnostics.Errors())
	}

	// Verify client was configured
	if resp.DataSourceData == nil {
		t.Fatal("Expected DataSourceData to be set")
	}

	// Cast to the API client type
	client, ok := resp.DataSourceData.(*api.ClientWithResponses)
	if !ok {
		t.Fatal("DataSourceData is not a *api.ClientWithResponses")
	}

	// Get the endpoint from the client
	underlyingClient := getUnderlyingClient(t, client)
	endpoint := getClientEndpoint(t, underlyingClient)

	expectedEndpoint := "https://api.env-endpoint.com/"
	if endpoint != expectedEndpoint {
		t.Errorf("Expected endpoint %q, got %q", expectedEndpoint, endpoint)
	}
}

// TestProviderConfigure_EndpointConfigOnly tests that setting only the endpoint
// in the provider configuration successfully configures the provider.
func TestProviderConfigure_EndpointConfigOnly(t *testing.T) {
	// Ensure environment variable is not set
	t.Setenv("SUPABASE_API_ENDPOINT", "")
	// Set access token to satisfy validation
	t.Setenv("SUPABASE_ACCESS_TOKEN", "test-token")

	p := &SupabaseProvider{version: "test"}

	// Get the schema
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	// Create configuration with endpoint set
	configValue := tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"endpoint":     tftypes.String,
			"access_token": tftypes.String,
		},
	}, map[string]tftypes.Value{
		"endpoint":     tftypes.NewValue(tftypes.String, "https://api.config-endpoint.com"),
		"access_token": tftypes.NewValue(tftypes.String, nil),
	})

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}

	req := provider.ConfigureRequest{
		Config: config,
	}
	resp := &provider.ConfigureResponse{}

	p.Configure(context.Background(), req, resp)

	// Should not have errors
	if resp.Diagnostics.HasError() {
		t.Fatalf("Unexpected error: %v", resp.Diagnostics.Errors())
	}

	// Verify client was configured
	if resp.DataSourceData == nil {
		t.Fatal("Expected DataSourceData to be set")
	}

	// Cast to the API client type
	client, ok := resp.DataSourceData.(*api.ClientWithResponses)
	if !ok {
		t.Fatal("DataSourceData is not a *api.ClientWithResponses")
	}

	// Get the endpoint from the client
	underlyingClient := getUnderlyingClient(t, client)
	endpoint := getClientEndpoint(t, underlyingClient)

	expectedEndpoint := "https://api.config-endpoint.com/"
	if endpoint != expectedEndpoint {
		t.Errorf("Expected endpoint %q, got %q", expectedEndpoint, endpoint)
	}
}

// TestProviderConfigure_EndpointConfigTakesPrecedence tests that when both the
// endpoint configuration and SUPABASE_API_ENDPOINT environment variable
// are set, the configuration value takes precedence.
func TestProviderConfigure_EndpointConfigTakesPrecedence(t *testing.T) {
	// Set environment variable to one value
	t.Setenv("SUPABASE_API_ENDPOINT", "https://api.env-endpoint.com")
	// Set access token to satisfy validation
	t.Setenv("SUPABASE_ACCESS_TOKEN", "test-token")

	p := &SupabaseProvider{version: "test"}

	// Get the schema
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)

	// Create configuration with different endpoint (this should take precedence)
	configValue := tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"endpoint":     tftypes.String,
			"access_token": tftypes.String,
		},
	}, map[string]tftypes.Value{
		"endpoint":     tftypes.NewValue(tftypes.String, "https://api.config-endpoint.com"),
		"access_token": tftypes.NewValue(tftypes.String, nil),
	})

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}

	req := provider.ConfigureRequest{
		Config: config,
	}
	resp := &provider.ConfigureResponse{}

	p.Configure(context.Background(), req, resp)

	// Should not have errors
	if resp.Diagnostics.HasError() {
		t.Fatalf("Unexpected error: %v", resp.Diagnostics.Errors())
	}

	// Verify client was configured
	if resp.DataSourceData == nil {
		t.Fatal("Expected DataSourceData to be set")
	}

	// Cast to the API client type
	client, ok := resp.DataSourceData.(*api.ClientWithResponses)
	if !ok {
		t.Fatal("DataSourceData is not a *api.ClientWithResponses")
	}

	// Get the endpoint from the client
	underlyingClient := getUnderlyingClient(t, client)
	endpoint := getClientEndpoint(t, underlyingClient)

	expectedEndpoint := "https://api.config-endpoint.com/"
	if endpoint != expectedEndpoint {
		t.Errorf("Expected endpoint %q, got %q (env endpoint would be 'https://api.env-endpoint.com/')",
			expectedEndpoint, endpoint)
	}
}

func TestAccProviderTrimsAccessTokenWhitespace(t *testing.T) {
	// Verify that access tokens with trailing whitespace (common from file() function)
	// are trimmed before being set in the Authorization header.
	defer gock.OffAll()
	gock.New("https://api.supabase.com").
		Get("/v1/projects/test-ref/branches").
		MatchHeader("Authorization", "^Bearer sbp_test123$").
		Persist().
		Reply(http.StatusOK).
		JSON([]map[string]any{})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "supabase" {
  access_token = "sbp_test123\n"
}

data "supabase_branch" "test" {
  parent_project_ref = "test-ref"
}
`,
			},
		},
	})
}
