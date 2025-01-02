// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccProjectAPIKeysDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccProjectAPIKeysDataSourceConfig("example-project-id"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.supabase_project_apikeys.test", "project_id", "example-project-id"),
					resource.TestCheckResourceAttrSet("data.supabase_project_apikeys.test", "anon_key"),
					resource.TestCheckResourceAttrSet("data.supabase_project_apikeys.test", "service_role_key"),
				),
			},
		},
	})
}

func testAccProjectAPIKeysDataSourceConfig(projectID string) string {
	return fmt.Sprintf(`
data "supabase_project_apikeys" "test" {
  project_id = %[1]q
}
`, projectID)
}

func TestAccProjectAPIKeysDataSource_NotFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccProjectAPIKeysDataSourceConfig("non-existent-project"),
				ExpectError: regexp.MustCompile("Unable to read project API keys"),
			},
		},
	})
}
