// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
	"gopkg.in/h2non/gock.v1"
)

func TestAccProjectResource_basic(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_project.test"
	rName := acctest.RandomName()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Project/basic/"),
				ConfigVariables: config.Variables{
					"name":            config.StringVariable(rName),
					"organization_id": config.StringVariable(organizationID),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "instance_size", "micro"),
					resource.TestCheckResourceAttr(resourceName, "database_password", "barbaz"),
					resource.TestCheckResourceAttr(resourceName, "legacy_api_keys_enabled", "false"),
				),
			},
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Project/basic/"),
				ConfigVariables: config.Variables{
					"name":            config.StringVariable(rName),
					"organization_id": config.StringVariable(organizationID),
				},
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,

				// database_password is not refreshed from the API
				ImportStateVerifyIgnore: []string{"database_password"},
			},
		},
	})
}

func TestAccProjectResource_update(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_project.test"
	rName := acctest.RandomName()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Project/basic/"),
				ConfigVariables: config.Variables{
					"name":            config.StringVariable(rName),
					"organization_id": config.StringVariable(organizationID),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "instance_size", "micro"),
					resource.TestCheckResourceAttr(resourceName, "database_password", "barbaz"),
					resource.TestCheckResourceAttr(resourceName, "legacy_api_keys_enabled", "false"),
				),
			},
			// Update password
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Project/basic/"),
				ConfigVariables: config.Variables{
					"name":              config.StringVariable(rName),
					"organization_id":   config.StringVariable(organizationID),
					"database_password": config.StringVariable("barbaznew"),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "instance_size", "micro"),
					resource.TestCheckResourceAttr(resourceName, "database_password", "barbaznew"),
					resource.TestCheckResourceAttr(resourceName, "legacy_api_keys_enabled", "false"),
				),
			},
			// Update instance size testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Project/basic/"),
				ConfigVariables: config.Variables{
					"name":              config.StringVariable(rName),
					"organization_id":   config.StringVariable(organizationID),
					"database_password": config.StringVariable("barbaznew"),
					"instance_size":     config.StringVariable("16xlarge"),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "instance_size", "16xlarge"),
					resource.TestCheckResourceAttr(resourceName, "database_password", "barbaznew"),
					resource.TestCheckResourceAttr(resourceName, "legacy_api_keys_enabled", "false"),
				),
			},
			//// Toggle legacy API keys
			//{
			//	ConfigDirectory: config.StaticDirectory("test-fixtures/Project/basic/"),
			//	ConfigVariables: config.Variables{
			//		"name":            config.StringVariable(rName),
			//		"organization_id": config.StringVariable(organizationID),
			//	},
			//	Check: resource.ComposeAggregateTestCheckFunc(
			//		resource.TestCheckResourceAttr("supabase_project.test", "id", testProjectRef),
			//		resource.TestCheckResourceAttr("supabase_project.test", "name", "bar"),
			//		resource.TestCheckResourceAttr("supabase_project.test", "instance_size", "16xlarge"),
			//		resource.TestCheckResourceAttr("supabase_project.test", "database_password", "barbaznew"),
			//		resource.TestCheckResourceAttr("supabase_project.test", "legacy_api_keys_enabled", "true"),
			//	),
			//},
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Project/basic/"),
				ConfigVariables: config.Variables{
					"name":              config.StringVariable(rName),
					"organization_id":   config.StringVariable(organizationID),
					"database_password": config.StringVariable("barbaznew"),
					"instance_size":     config.StringVariable("16xlarge"),
				},
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,

				// database_password is not refreshed from the API
				ImportStateVerifyIgnore: []string{"database_password"},
			},
		},
	})
}

func projectResourceConfig(p ProjectResourceModel) string {
	rv := fmt.Sprintf(`resource "supabase_project" "test" {
  organization_id         = "%s"
  name                    = "%s"
  database_password       = "%s"
  region                  = "%s"
  instance_size           = "%s"
`,
		p.OrganizationId.ValueString(),
		p.Name.ValueString(),
		p.DatabasePassword.ValueString(),
		p.Region.ValueString(),
		p.InstanceSize.ValueString(),
	)
	if !p.LegacyApiKeysEnabled.IsNull() {
		rv += fmt.Sprintf("\n  legacy_api_keys_enabled = %t", p.LegacyApiKeysEnabled.ValueBool())
	}

	return rv + "\n}"
}

// projectResourceConfigWithTimeouts returns a project resource config string with a timeouts block.
func projectResourceConfigWithTimeouts(p ProjectResourceModel) string {
	s := projectResourceConfig(p)
	idx := strings.LastIndex(s, "}")
	return s[:idx] + `
  timeouts {
    create = "30m"
    update = "30m"
  }
` + s[idx:]
}

func TestAccProjectResource_Timeouts(t *testing.T) {
	defer gock.OffAll()
	// Create
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
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New("https://api.supabase.com").
		Put("/v1/projects/mayuaycdtijbctgqbycg/api-keys/legacy").
		MatchParam("enabled", "false").
		Reply(http.StatusOK)
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/api-keys/legacy").
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
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
	// Post-apply refresh: read project, legacy keys, addons
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/api-keys/legacy").
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
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
	// Delete
	gock.New("https://api.supabase.com").
		Delete("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: projectResourceConfigWithTimeouts(ProjectResourceModel{
					OrganizationId:       types.StringValue("continued-brown-smelt"),
					Name:                 types.StringValue("foo"),
					DatabasePassword:     types.StringValue("barbaz"),
					Region:               types.StringValue("us-east-1"),
					InstanceSize:         types.StringValue("micro"),
					LegacyApiKeysEnabled: types.BoolValue(false),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_project.test", "id", "mayuaycdtijbctgqbycg"),
					resource.TestCheckResourceAttr("supabase_project.test", "name", "foo"),
				),
			},
		},
	})
}
