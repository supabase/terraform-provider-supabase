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

func TestAccPoolerDataSource(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	dataSourceName := "data.supabase_pooler.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Pooler/basic/"),
				ConfigVariables: config.Variables{
					"organization_id": config.StringVariable(organizationID),
					"project_name":    config.StringVariable(acctest.RandomName()),
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(
						dataSourceName, tfjsonpath.New("project_ref"),
						projectResourceName, tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					statecheck.ExpectKnownValue(dataSourceName, tfjsonpath.New("url"), knownvalue.MapPartial(map[string]knownvalue.Check{
						"transaction": knownvalue.NotNull(),
					})),
				},
			},
		},
	})
}
