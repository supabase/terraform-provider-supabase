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

func TestAccProjectResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	// Step 1: create
	gock.New("https://api.supabase.com").
		Post("/v1/projects").
		Reply(http.StatusCreated).
		JSON(api.ProjectResponse{
			Id:   "mayuaycdtijbctgqbycg",
			Name: "foo",
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects").
		Reply(http.StatusOK).
		JSON(
			[]api.ProjectResponse{
				{
					Id:             "mayuaycdtijbctgqbycg",
					Name:           "foo",
					OrganizationId: "continued-brown-smelt",
					Region:         "us-east-1",
				},
			},
		)
	// Step 2: read
	gock.New("https://api.supabase.com").
		Get("/v1/projects").
		Reply(http.StatusOK).
		JSON(
			[]api.ProjectResponse{
				{
					Id:             "mayuaycdtijbctgqbycg",
					Name:           "foo",
					OrganizationId: "continued-brown-smelt",
					Region:         "us-east-1",
				},
			},
		)
		// Step 3: delete
	gock.New("https://api.supabase.com").
		Delete("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: examples.ProjectResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_project.test", "id", "mayuaycdtijbctgqbycg"),
				),
			},
			// ImportState testing
			{
				ResourceName:            "supabase_project.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"database_password"},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
