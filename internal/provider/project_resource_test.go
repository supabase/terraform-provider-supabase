// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"strings"
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
		JSON(api.V1ProjectResponse{
			Id:   "mayuaycdtijbctgqbycg",
			Name: "foo",
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/billing/addons").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0CiMicro,
						"name":  "Micro",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/billing/addons").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0CiMicro,
						"name":  "Micro",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Step 2: update instance size
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/billing/addons").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0Ci16xlarge,
						"name":  "16XL",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK)
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/database/password").
		Reply(http.StatusOK)
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/billing/addons").
		Reply(http.StatusOK)
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "bar",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/billing/addons").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0Ci16xlarge,
						"name":  "16XL",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Step 4: import state
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "bar",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/billing/addons").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0Ci16xlarge,
						"name":  "16XL",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Step 5: delete
	gock.New("https://api.supabase.com").
		Delete("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
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
					resource.TestCheckResourceAttr("supabase_project.test", "name", "foo"),
					resource.TestCheckResourceAttr("supabase_project.test", "instance_size", "micro"),
					resource.TestCheckResourceAttr("supabase_project.test", "database_password", "barbaz"),
				),
			},
			// Update instance size testing
			{
				Config: Config: strings.ReplaceAll(
					strings.ReplaceAll(examples.ProjectResourceConfig, `"micro"`, `"16xlarge"`),
					strings.ReplaceAll(examples.ProjectResourceConfig, `"foo"`, `"bar"`),
					`"barbaz"`,
					`"barbaznew"`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_project.test", "id", "mayuaycdtijbctgqbycg"),
					resource.TestCheckResourceAttr("supabase_project.test", "name", "bar"),
					resource.TestCheckResourceAttr("supabase_project.test", "instance_size", "16xlarge"),
					resource.TestCheckResourceAttr("supabase_project.test", "database_password", "barbaznew"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "supabase_project.test",
				ImportState:       true,
				ImportStateVerify: true,

				// database_password is not refreshed from the API
				ImportStateVerifyIgnore: []string{"database_password"},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
