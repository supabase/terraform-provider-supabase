// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-provider-scaffolding-framework/examples"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestAccSettingsResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Persist().
		Reply(http.StatusOK).
		JSON(api.PostgrestConfigResponse{
			MaxRows: 1000,
		})
	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: examples.SettingsResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.production", "id", "mayuaycdtijbctgqbycg"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "supabase_settings.production",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccSettingsResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.branch", "api.max_rows", "1000"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

const testAccSettingsResourceConfig = `
resource "supabase_settings" "branch" {
  project_ref = "mayuaycdtijbctgqbycg"

  api = {
	max_rows = 1000
  }

  auth = {
	site_url = "http://localhost:3000"
  }
}
`
