// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
	"gopkg.in/h2non/gock.v1"
)

func TestAccNetworkBansDataSource(t *testing.T) {
	defer gock.OffAll()
	gock.New(defaultApiEndpoint).
		Post(networkBansApiPath).
		Times(3).
		Reply(http.StatusCreated).
		JSON(api.NetworkBanResponse{
			BannedIpv4Addresses: []string{"1.2.3.4", "5.6.7.8"},
		})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: examples.NetworkBansDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.supabase_network_bans.test", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("data.supabase_network_bans.test", "banned_ipv4_addresses.#", "2"),
					resource.TestCheckTypeSetElemAttr("data.supabase_network_bans.test", "banned_ipv4_addresses.*", "1.2.3.4"),
					resource.TestCheckTypeSetElemAttr("data.supabase_network_bans.test", "banned_ipv4_addresses.*", "5.6.7.8"),
				),
			},
		},
	})
}

func TestAccNetworkBansDataSource_empty(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	dataSourceName := "data.supabase_network_bans.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// A newly created project has no banned addresses
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/NetworkBans/basic/"),
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
					statecheck.ExpectKnownValue(dataSourceName, tfjsonpath.New("banned_ipv4_addresses"), knownvalue.SetSizeExact(0)),
				},
			},
		},
	})
}
