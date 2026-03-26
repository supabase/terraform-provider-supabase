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
					resource.TestCheckResourceAttr("data.supabase_network_bans.test", "banned_ipv4_addresses.0", "1.2.3.4"),
					resource.TestCheckResourceAttr("data.supabase_network_bans.test", "banned_ipv4_addresses.1", "5.6.7.8"),
				),
			},
		},
	})
}

func TestAccNetworkBansDataSource_Empty(t *testing.T) {
	defer gock.OffAll()
	gock.New(defaultApiEndpoint).
		Post(networkBansApiPath).
		Times(3).
		Reply(http.StatusCreated).
		JSON(api.NetworkBanResponse{})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: examples.NetworkBansDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.supabase_network_bans.test", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("data.supabase_network_bans.test", "banned_ipv4_addresses.#", "0"),
				),
			},
		},
	})
}
