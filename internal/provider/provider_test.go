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
	"github.com/supabase/cli/pkg/api"
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

	// Use reflection to access the embedded Client field
	v := reflect.ValueOf(client).Elem()
	clientInterfaceField := v.FieldByName("ClientInterface")
	if !clientInterfaceField.IsValid() {
		t.Fatal("Cannot access ClientInterface field")
	}

	// Get the underlying *Client
	underlyingClient := clientInterfaceField.Interface().(*api.Client)

	// Create a mock request to apply the request editors to
	mockReq, _ := http.NewRequest("GET", "https://api.supabase.com/test", nil)

	// Apply the request editors to see what Authorization header they set
	requestEditorsField := reflect.ValueOf(underlyingClient).Elem().FieldByName("RequestEditors")
	if !requestEditorsField.IsValid() {
		t.Fatal("Cannot access RequestEditors field")
	}

	requestEditors := requestEditorsField.Interface().([]api.RequestEditorFn)
	for _, editor := range requestEditors {
		if err := editor(context.Background(), mockReq); err != nil {
			t.Fatalf("Request editor failed: %v", err)
		}
	}

	// Verify the Authorization header contains the config token, not the env token
	authHeader := mockReq.Header.Get("Authorization")
	expectedAuth := "Bearer config-token"
	if authHeader != expectedAuth {
		t.Errorf("Expected Authorization header %q, got %q (env token would be 'Bearer env-token')",
			expectedAuth, authHeader)
	}
}
