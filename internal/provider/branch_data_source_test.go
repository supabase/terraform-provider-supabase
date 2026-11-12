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
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
)

func TestAccBranchDataSource(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_branch.test"
	dataSourceName := "data.supabase_branch.test"
	gitBranch := "develop"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Branch/datasource/"),
				ConfigVariables: config.Variables{
					"organization_id": config.StringVariable(organizationID),
					"project_name":    config.StringVariable(acctest.RandomName()),
					"git_branch":      config.StringVariable(gitBranch),
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(
						dataSourceName, tfjsonpath.New("parent_project_ref"),
						projectResourceName, tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					statecheck.ExpectKnownValue(dataSourceName, tfjsonpath.New("branches"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(dataSourceName, tfjsonpath.New("branches"), knownvalue.SetPartial([]knownvalue.Check{
						knownvalue.ObjectPartial(map[string]knownvalue.Check{
							"git_branch": knownvalue.StringExact(gitBranch),
						}),
					})),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("git_branch"), knownvalue.StringExact(gitBranch)),
				},
			},
		},
	})
}
