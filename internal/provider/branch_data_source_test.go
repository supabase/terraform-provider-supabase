// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/terraform-provider-supabase/examples"
	"github.com/supabase/terraform-provider-supabase/internal/gen/api"
	"gopkg.in/h2non/gock.v1"
)

func TestAccBranchDataSource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	gock.New(defaultApiEndpoint).
		Get(branchesApiPath).
		Times(3).
		Reply(http.StatusOK).
		JSON([]api.BranchResponseOutput{{Id: uuid.New()}})
	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: examples.BranchDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.supabase_branch.all", "branches.#"),
				),
			},
		},
	})
}
