// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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

	// We override the env vars to make sure tests pass

	// Setting an access token is required now because it is validated in the
	// Configure function in provider.go
	t.Setenv("SUPABASE_ACCESS_TOKEN", "test")

	// Setting the API endpoint to a value used in the tests so that mocks
	// can verify the correct endpoint calls
	t.Setenv("SUPABASE_API_ENDPOINT", defaultApiEndpoint)
}

func TestAccProviderConfigure_AccessTokenRequired(t *testing.T) {
	// Verify that an error is returned when neither the
	// access_token configuration nor the
	// SUPABASE_ACCESS_TOKEN environment variable is set.
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Setenv("SUPABASE_ACCESS_TOKEN", "")
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "supabase_branch" "test" {
  parent_project_ref = "%s"
}
`, testProjectRef),
				ExpectError: regexp.MustCompile("Missing Supabase API Access Token"),
			},
		},
	})
}

func TestAccProviderConfigure_EnvVarOnly(t *testing.T) {
	// Verify that setting only the SUPABASE_ACCESS_TOKEN
	// environment variable successfully configures the provider.
	defer gock.OffAll()
	gock.New(defaultApiEndpoint).
		Get(branchesApiPath).
		MatchHeader("Authorization", "^Bearer env-token$").
		Persist().
		Reply(http.StatusOK).
		JSON([]map[string]any{})

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Setenv("SUPABASE_ACCESS_TOKEN", "env-token")
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "supabase_branch" "test" {
  parent_project_ref = "%s"
}
`, testProjectRef),
			},
		},
	})
}

func TestAccProviderConfigure_ConfigOnly(t *testing.T) {
	// Verify that setting only the access_token
	// in the provider configuration successfully configures the provider.
	defer gock.OffAll()
	gock.New(defaultApiEndpoint).
		Get(branchesApiPath).
		MatchHeader("Authorization", "^Bearer config-token$").
		Persist().
		Reply(http.StatusOK).
		JSON([]map[string]any{})

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Setenv("SUPABASE_ACCESS_TOKEN", "")
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "supabase" {
  access_token = "config-token"
}

data "supabase_branch" "test" {
  parent_project_ref = "%s"
}
`, testProjectRef),
			},
		},
	})
}

func TestAccProviderConfigure_ConfigTakesPrecedence(t *testing.T) {
	// Verify that when both the access_token configuration and
	// SUPABASE_ACCESS_TOKEN environment variable
	// are set, the configuration value takes precedence.
	defer gock.OffAll()
	gock.New(defaultApiEndpoint).
		Get(branchesApiPath).
		MatchHeader("Authorization", "^Bearer config-token$").
		Persist().
		Reply(http.StatusOK).
		JSON([]map[string]any{})

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Setenv("SUPABASE_ACCESS_TOKEN", "env-token")
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "supabase" {
  access_token = "config-token"
}

data "supabase_branch" "test" {
  parent_project_ref = "%s"
}
`, testProjectRef),
			},
		},
	})
}

func TestAccProviderConfigure_EndpointDefault(t *testing.T) {
	// Verify that when neither the endpoint configuration nor the
	// SUPABASE_API_ENDPOINT environment variable is set,
	// the endpoint defaults to defaultApiEndpoint.

	defer gock.OffAll()
	gock.New(defaultApiEndpoint).
		Get(branchesApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON([]map[string]any{})

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Setenv("SUPABASE_API_ENDPOINT", "")
			t.Setenv("SUPABASE_ACCESS_TOKEN", "test-token")
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "supabase_branch" "test" {
  parent_project_ref = "%s"
}
`, testProjectRef),
			},
		},
	})
}

func TestAccProviderConfigure_EndpointEnvVarOnly(t *testing.T) {
	// Verify that setting only the SUPABASE_API_ENDPOINT environment variable
	// successfully configures the provider.
	defer gock.OffAll()
	gock.New("https://api.env-endpoint.com").
		Get(branchesApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON([]map[string]any{})

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Setenv("SUPABASE_API_ENDPOINT", "https://api.env-endpoint.com")
			t.Setenv("SUPABASE_ACCESS_TOKEN", "test-token")
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "supabase_branch" "test" {
  parent_project_ref = "%s"
}
`, testProjectRef),
			},
		},
	})
}

func TestAccProviderConfigure_EndpointConfigOnly(t *testing.T) {
	// Verify that setting only the endpoint in the provider
	// configuration successfully configures the provider.
	defer gock.OffAll()
	gock.New("https://api.config-endpoint.com").
		Get(branchesApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON([]map[string]any{})

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Setenv("SUPABASE_API_ENDPOINT", "")
			t.Setenv("SUPABASE_ACCESS_TOKEN", "test-token")
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "supabase" {
  endpoint = "https://api.config-endpoint.com"
}

data "supabase_branch" "test" {
  parent_project_ref = "%s"
}
`, testProjectRef),
			},
		},
	})
}

func TestAccProviderConfigure_EndpointConfigTakesPrecedence(t *testing.T) {
	// Verify that when both the endpoint configuration and SUPABASE_API_ENDPOINT
	// environment variable are set, the configuration value takes precedence.
	defer gock.OffAll()
	gock.New("https://api.config-endpoint.com").
		Get(branchesApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON([]map[string]any{})

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Setenv("SUPABASE_API_ENDPOINT", "https://api.env-endpoint.com")
			t.Setenv("SUPABASE_ACCESS_TOKEN", "test-token")
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "supabase" {
  endpoint = "https://api.config-endpoint.com"
}

data "supabase_branch" "test" {
  parent_project_ref = "%s"
}
`, testProjectRef),
			},
		},
	})
}

func TestAccProviderTrimsAccessTokenWhitespace(t *testing.T) {
	// Verify that access tokens with trailing whitespace (common from file() function)
	// are trimmed before being set in the Authorization header.
	defer gock.OffAll()
	gock.New(defaultApiEndpoint).
		Get(branchesApiPath).
		MatchHeader("Authorization", "^Bearer sbp_test123$").
		Persist().
		Reply(http.StatusOK).
		JSON([]map[string]any{})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "supabase" {
  access_token = "sbp_test123\n"
}

data "supabase_branch" "test" {
  parent_project_ref = "%s"
}
`, testProjectRef),
			},
		},
	})
}
