// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
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
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(rName)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("instance_size"), knownvalue.StringExact("micro")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("database_password"), knownvalue.StringExact("barbaz")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("legacy_api_keys_enabled"), knownvalue.Bool(false)),
				},
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
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(rName)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("instance_size"), knownvalue.StringExact("micro")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("database_password"), knownvalue.StringExact("barbaz")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("legacy_api_keys_enabled"), knownvalue.Bool(false)),
				},
			},
			// Update password
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Project/basic/"),
				ConfigVariables: config.Variables{
					"name":              config.StringVariable(rName),
					"organization_id":   config.StringVariable(organizationID),
					"database_password": config.StringVariable("barbaznew"),
				},
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(rName)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("instance_size"), knownvalue.StringExact("micro")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("database_password"), knownvalue.StringExact("barbaznew")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("legacy_api_keys_enabled"), knownvalue.Bool(false)),
				},
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
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(rName)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("instance_size"), knownvalue.StringExact("16xlarge")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("database_password"), knownvalue.StringExact("barbaznew")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("legacy_api_keys_enabled"), knownvalue.Bool(false)),
				},
			},
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

func TestAccProjectResource_legacyAPIKeysDeprecated(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	rName := acctest.RandomName()

	// legacyAPIKeysAlreadyDisabledRegex matches the error returned when legacy API keys
	// were already disabled, e.g.:
	//
	//	Unable to update legacy api keys, got error: {"message":"JWT based legacy (anon, service_role) API keys were already disabled on 2026-11-06T15:26:22.165832+00:00"}
	//
	// Terraform wraps long diagnostics across lines, so whitespace is matched with \s+.
	var legacyAPIKeysAlreadyDisabledRegex = regexp.MustCompile(
		`Unable\s+to\s+update\s+legacy\s+api\s+keys,\s+got\s+error:\s+\{"message":"JWT\s+based\s+legacy\s+\(anon,\s+service_role\)\s+API\s+keys\s+were\s+already\s+disabled\s+on\s+[^"]+"\}`,
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Toggle legacy API keys
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Project/legacykeys/"),
				ConfigVariables: config.Variables{
					"name":            config.StringVariable(rName),
					"organization_id": config.StringVariable(organizationID),
				},
				ExpectError: legacyAPIKeysAlreadyDisabledRegex,
			},
		},
	})
}
