// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

const (
	testAccBranchResourceConfig = `
resource "supabase_branch" "new" {
  parent_project_ref = "` + testProjectRef + `"
  git_branch         = "develop"
}
`
)

func TestAccBranchResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	// Step 1: create
	gock.New(defaultApiEndpoint).
		Get(branchesApiPath).
		Reply(http.StatusUnprocessableEntity)
	gock.New(defaultApiEndpoint).
		Post(branchesApiPath).
		Reply(http.StatusCreated).
		JSON(api.BranchResponse{
			Id:               uuid.New(),
			ParentProjectRef: testProjectRef,
			IsDefault:        true,
		})
	gock.New(defaultApiEndpoint).
		Post(branchesApiPath).
		Reply(http.StatusCreated).
		JSON(api.BranchResponse{
			Id:               uuid.MustParse(testBranchUUID),
			ParentProjectRef: testProjectRef,
			GitBranch:        Ptr("main"),
		})

	gock.New(defaultApiEndpoint).
		Get(branchApiPath).
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	gock.New(defaultApiEndpoint).
		Get(branchApiPath).
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	// Step 2: read
	gock.New(defaultApiEndpoint).
		Get(branchApiPath).
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	gock.New(defaultApiEndpoint).
		Get(branchApiPath).
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	// Step 3: update and read
	gock.New(defaultApiEndpoint).
		Get(branchApiPath).
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	gock.New(defaultApiEndpoint).
		Patch(branchApiPath).
		Reply(http.StatusOK).
		JSON(api.BranchResponse{
			Id:               uuid.MustParse(testBranchUUID),
			ParentProjectRef: testProjectRef,
			GitBranch:        Ptr("develop"),
		})
	gock.New(defaultApiEndpoint).
		Get(branchApiPath).
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{})
	// Step 4: delete
	gock.New(defaultApiEndpoint).
		Delete(branchApiPath).
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
					resource.TestCheckResourceAttr("supabase_branch.new", "id", testBranchUUID),
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
