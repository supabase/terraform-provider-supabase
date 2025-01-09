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

func TestAccProjectAPIKeysDataSource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/api-keys").
		Times(3).
		Reply(http.StatusOK).
		JSON([]api.ApiKeyResponse{
			{
				Name:   "anon",
				ApiKey: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.anon",
			},
			{
				Name:   "service_role",
				ApiKey: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.service_role",
			},
		})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: examples.APIKeysDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.supabase_apikeys.production", "anon_key", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.anon"),
					resource.TestCheckResourceAttr("data.supabase_apikeys.production", "service_role_key", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.service_role"),
				),
			},
		},
	})
}
