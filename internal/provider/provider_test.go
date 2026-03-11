// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
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
