// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/oapi-codegen/nullable"
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
				Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeLegacy),
				ApiKey: nullable.NewNullableWithValue("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.anon"),
			},
			{
				Name:   "service_role",
				Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeLegacy),
				ApiKey: nullable.NewNullableWithValue("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.service_role"),
			},
			{
				Name:   "publishable",
				Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
				ApiKey: nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			},
			{
				Name:   "secret",
				Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
				ApiKey: nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			},
			{
				Name:   "other_secret",
				Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
				ApiKey: nullable.NewNullableWithValue("sb_secret_eybcCI6kNiIsiR5UCJ9VGpzI1IciOJhJIXIn"),
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
					resource.TestCheckResourceAttr("data.supabase_apikeys.production", "publishable_key", "sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
					resource.TestCheckResourceAttr("data.supabase_apikeys.production", "secret_keys.#", "2"),
					resource.TestCheckResourceAttr("data.supabase_apikeys.production", "secret_keys.0.name", "secret"),
					resource.TestCheckResourceAttr("data.supabase_apikeys.production", "secret_keys.0.api_key", "sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
					resource.TestCheckResourceAttr("data.supabase_apikeys.production", "secret_keys.1.name", "other_secret"),
					resource.TestCheckResourceAttr("data.supabase_apikeys.production", "secret_keys.1.api_key", "sb_secret_eybcCI6kNiIsiR5UCJ9VGpzI1IciOJhJIXIn"),
				),
			},
		},
	})
}
