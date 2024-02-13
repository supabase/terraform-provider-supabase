// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

func TestAccBranchResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	// Step 1: create
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/branches").
		Reply(http.StatusUnprocessableEntity)
	gock.New("https://api.supabase.com").
		Post("/v1/projects/mayuaycdtijbctgqbycg/branches").
		Reply(http.StatusCreated).
		JSON(api.BranchResponse{
			Id:               "prod-branch",
			ParentProjectRef: "mayuaycdtijbctgqbycg",
			IsDefault:        true,
		})
	gock.New("https://api.supabase.com").
		Post("/v1/projects/mayuaycdtijbctgqbycg/branches").
		Reply(http.StatusCreated).
		JSON(api.BranchResponse{
			Id:               "test-branch",
			ParentProjectRef: "mayuaycdtijbctgqbycg",
			GitBranch:        Ptr("main"),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/branches/test-branch").
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	gock.New("https://api.supabase.com").
		Get("/v1/branches/test-branch").
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	// Step 2: read
	gock.New("https://api.supabase.com").
		Get("/v1/branches/test-branch").
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	gock.New("https://api.supabase.com").
		Get("/v1/branches/test-branch").
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	// Step 3: update and read
	gock.New("https://api.supabase.com").
		Get("/v1/branches/test-branch").
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	gock.New("https://api.supabase.com").
		Patch("/v1/branches/test-branch").
		Reply(http.StatusOK).
		JSON(api.BranchResponse{
			Id:               "test-branch",
			ParentProjectRef: "mayuaycdtijbctgqbycg",
			GitBranch:        Ptr("develop"),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/branches/test-branch").
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	// Step 4: delete
	gock.New("https://api.supabase.com").
		Delete("/v1/branches/test-branch").
		Reply(http.StatusOK)
	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: examples.BranchResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_branch.new", "id", "test-branch"),
				),
			},
			// ImportState testing
			{
				ResourceName:            "supabase_branch.new",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"git_branch", "parent_project_ref"},
			},
			// Update and Read testing
			{
				Config: testAccBranchResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_branch.new", "git_branch", "develop"),
					// resource.TestCheckResourceAttrSet("supabase_branch.new", "database"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

const testAccBranchResourceConfig = `
resource "supabase_branch" "new" {
  parent_project_ref = "mayuaycdtijbctgqbycg"
  git_branch         = "develop"
}
`
