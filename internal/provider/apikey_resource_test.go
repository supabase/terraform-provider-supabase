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

const testAccApikeyResourceConfig = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "test"
}
`

func TestAccApiKeyResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	// Step 1: create
	gock.New(defaultApiEndpoint).
		Get(apiKeysApiPath).
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
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(uuid.New().String()),
			Name:   "default",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
			ApiKey: nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(testApiKeyUUID),
			Name:   "test",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey: nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(testApiKeyUUID),
			Name:   "test",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey: nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			SecretJwtTemplate: nullable.NewNullableWithValue(map[string]interface{}{
				"role": "service_role",
			}),
		})
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
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
					resource.TestCheckResourceAttr("supabase_apikey.new", "id", testApiKeyUUID),
				),
			},
			// ImportState testing
			{
				ResourceName:            "supabase_apikey.new",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"name", "project_ref"},
				ImportStateId:           fmt.Sprintf("%s/%s", testProjectRef, testApiKeyUUID),
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

func TestResolveAPIKeyImportID(t *testing.T) {
	knownID := uuid.New()
	otherID := uuid.New()

	tests := []struct {
		name string
		id   string
		mock func()

		expectProjectRef   string
		expectKeyID        string
		expectErrorSummary string
	}{
		{
			name:             "import by ID",
			id:               testProjectRef + "/" + knownID.String(),
			expectProjectRef: testProjectRef,
			expectKeyID:      knownID.String(),
		},
		{
			name: "import by name",
			id:   testProjectRef + "/mykey",
			mock: func() {
				gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).
					JSON([]api.ApiKeyResponse{{Id: nullable.NewNullableWithValue(knownID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret)}})
			},
			expectProjectRef: testProjectRef,
			expectKeyID:      knownID.String(),
		},
		{
			name: "import by name and type",
			id:   testProjectRef + "/mykey/secret",
			mock: func() {
				gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).
					JSON([]api.ApiKeyResponse{
						{Id: nullable.NewNullableWithValue(otherID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable)},
						{Id: nullable.NewNullableWithValue(knownID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret)},
					})
			},
			expectProjectRef: testProjectRef,
			expectKeyID:      knownID.String(),
		},
		{
			name: "import by name (ambiguous)",
			id:   testProjectRef + "/mykey",
			mock: func() {
				gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).
					JSON([]api.ApiKeyResponse{
						{Id: nullable.NewNullableWithValue(knownID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable)},
						{Id: nullable.NewNullableWithValue(otherID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret)},
					})
			},
			expectErrorSummary: "Ambiguous Import Identifier",
		},
		{
			name: "key name not found",
			id:   testProjectRef + "/mykey",
			mock: func() {
				gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).
					JSON([]api.ApiKeyResponse{
						{Id: nullable.NewNullableWithValue(knownID.String()), Name: "knownkey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable)},
						{Id: nullable.NewNullableWithValue(otherID.String()), Name: "otherkey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret)},
					})
			},
			expectErrorSummary: "Import Error",
		},
		{
			name:               "import by name and bad type",
			id:                 testProjectRef + "/mykey/badtype",
			expectErrorSummary: "Unexpected Import Identifier",
		},
		{
			name:               "invalid import format",
			id:                 testProjectRef,
			expectErrorSummary: "Unexpected Import Identifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gock.InterceptClient(http.DefaultClient)
			defer gock.RestoreClient(http.DefaultClient)
			defer gock.OffAll()
			if tt.mock != nil {
				tt.mock()
			}

			client, err := api.NewClientWithResponses(defaultApiEndpoint)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			actualProjectRef, actualKeyID, diag := resolveAPIKeyImportID(t.Context(), client, tt.id)
			if tt.expectErrorSummary != "" {
				if diag == nil || diag.Summary() != tt.expectErrorSummary {
					t.Errorf("Expected error %q, got: %v", tt.expectErrorSummary, diag)
				}
				return
			}

			if diag != nil {
				t.Fatalf("Expected no error, got: %v", diag)
			}

			if tt.expectProjectRef != actualProjectRef {
				t.Errorf("Expected ref %q, got %q", tt.expectProjectRef, actualProjectRef)
			}
			if tt.expectKeyID != actualKeyID {
				t.Errorf("Expected id %q, got %q", tt.expectKeyID, actualKeyID)
			}
		})
	}
}
