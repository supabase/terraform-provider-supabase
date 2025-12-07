// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

const testProjectRef = "mayuaycdtijbctgqbycg" //nolint:gosec

func TestAccApiKeyResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	// Step 1: create
	testApiKeyUUID := uuid.New()
	apiKeysEndpoint := fmt.Sprintf("/v1/projects/%s/api-keys", testProjectRef)
	apiKeyEndpoint := fmt.Sprintf("%s/%s", apiKeysEndpoint, testApiKeyUUID.String())
	gock.New("https://api.supabase.com").
		Get(apiKeysEndpoint).
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
		})
	gock.New("https://api.supabase.com").
		Post(apiKeysEndpoint).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(uuid.New().String()),
			Name:   "default",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
			ApiKey: nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New("https://api.supabase.com").
		Post(apiKeysEndpoint).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(testApiKeyUUID.String()),
			Name:   "test",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey: nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New("https://api.supabase.com").
		Get(apiKeyEndpoint).
		Persist().
		Reply(http.StatusOK).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(testApiKeyUUID.String()),
			Name:   "test",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey: nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			SecretJwtTemplate: nullable.NewNullableWithValue(map[string]interface{}{
				"role": "service_role",
			}),
		})
	gock.New("https://api.supabase.com").
		Delete(apiKeyEndpoint).
		Reply(http.StatusOK)

	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: examples.ApiKeyResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "id", testApiKeyUUID.String()),
				),
			},
			// ImportState testing
			{
				ResourceName:            "supabase_apikey.new",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"name", "project_ref"},
				ImportStateId:           fmt.Sprintf("%s/%s", testProjectRef, testApiKeyUUID.String()),
			},
			// Update and Read testing
			{
				Config: testAccApikeyResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "name", "test"),
					resource.TestCheckResourceAttr("supabase_apikey.new", "project_ref", testProjectRef),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

const testAccApikeyResourceConfig = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "test"
}
`
