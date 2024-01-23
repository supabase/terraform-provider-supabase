// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestAccBranchDataSource(t *testing.T) {
	defer gock.OffAll()
	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			// Setup mock api
			gock.New("https://api.supabase.com").
				Get("/v1/projects/mayuaycdtijbctgqbycg/branches").
				Persist().
				Reply(http.StatusOK).
				JSON([]api.BranchResponse{{Id: "test"}})
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccBranchDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.supabase_branch.preview", "branches.#"),
				),
			},
		},
	})
}

const testAccBranchDataSourceConfig = `
data "supabase_branch" "preview" {
  parent_project_ref = "mayuaycdtijbctgqbycg"
}
`
