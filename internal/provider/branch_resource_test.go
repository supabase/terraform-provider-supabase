// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
)

func testAccBranchLiveVariables(organizationID, projectName, gitBranch string, persistent bool) config.Variables {
	return config.Variables{
		"organization_id": config.StringVariable(organizationID),
		"project_name":    config.StringVariable(projectName),
		"git_branch":      config.StringVariable(gitBranch),
		"persistent":      config.BoolVariable(persistent),
	}
}

func TestAccBranchResource_basic(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_branch.test"
	projectName := acctest.RandomName()
	gitBranch := "develop"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Branch/basic/"),
				ConfigVariables: testAccBranchLiveVariables(organizationID, projectName, gitBranch, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.CompareValuePairs(
						resourceName, tfjsonpath.New("parent_project_ref"),
						projectResourceName, tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("git_branch"), knownvalue.StringExact(gitBranch)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("region"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("persistent"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("database"), knownvalue.ObjectExact(map[string]knownvalue.Check{
						"id":         knownvalue.NotNull(),
						"host":       knownvalue.NotNull(),
						"port":       knownvalue.NotNull(),
						"user":       knownvalue.NotNull(),
						"password":   knownvalue.NotNull(),
						"jwt_secret": knownvalue.NotNull(),
						"version":    knownvalue.NotNull(),
						"status":     knownvalue.NotNull(),
					})),
				},
			},
			// ImportState testing
			{
				ConfigDirectory:         config.StaticDirectory("test-fixtures/Branch/basic/"),
				ConfigVariables:         testAccBranchLiveVariables(organizationID, projectName, gitBranch, false),
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"git_branch", "parent_project_ref", "persistent"},
			},
		},
	})
}

func TestAccBranchResource_update(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_branch.test"
	projectName := acctest.RandomName()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Branch/basic/"),
				ConfigVariables: testAccBranchLiveVariables(organizationID, projectName, "develop", false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("git_branch"), knownvalue.StringExact("develop")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("persistent"), knownvalue.Bool(false)),
				},
			},
			// Update git_branch
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Branch/basic/"),
				ConfigVariables: testAccBranchLiveVariables(organizationID, projectName, "feature", false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("git_branch"), knownvalue.StringExact("feature")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("persistent"), knownvalue.Bool(false)),
				},
			},
			// Update persistent
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Branch/basic/"),
				ConfigVariables: testAccBranchLiveVariables(organizationID, projectName, "feature", true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("git_branch"), knownvalue.StringExact("feature")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("persistent"), knownvalue.Bool(true)),
				},
			},
			// ImportState testing
			{
				ConfigDirectory:         config.StaticDirectory("test-fixtures/Branch/basic/"),
				ConfigVariables:         testAccBranchLiveVariables(organizationID, projectName, "feature", true),
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"git_branch", "parent_project_ref", "persistent"},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
