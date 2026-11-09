// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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

const testAccApikeyResourceConfigWithDescription = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "test"
  description = "Service key for test"
}
`

const testAccApikeyResourceConfigInvalidName = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "Invalid-Name-123" # Contains capital letters and hyphens
}
`

const testAccApikeyResourceConfigPublishable = `
resource "supabase_apikey" "web" {
  project_ref = "` + testProjectRef + `"
  name        = "web_client"
  type        = "publishable"
  description = "Web client"
}
`

const testAccApikeyResourceConfigPublishableWithoutType = `
resource "supabase_apikey" "web" {
  project_ref = "` + testProjectRef + `"
  name        = "web_client"
  description = "Web client"
}
`

const testAccApikeyResourceConfigPublishableToSecret = `
resource "supabase_apikey" "web" {
  project_ref = "` + testProjectRef + `"
  name        = "web_client"
  type        = "secret"
  description = "Web client"
}
`

const testAccApikeyResourceConfigInvalidType = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "test"
  type        = "legacy"
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
		JSON(map[string]any{"name": "default", "type": "publishable"}).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(uuid.New().String()),
			Name:   "default",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
			ApiKey: nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		JSON(map[string]any{"name": "test", "type": "secret", "secret_jwt_template": map[string]any{"role": "service_role"}}).
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

func TestAccApiKeyResource_Publishable(t *testing.T) {
	defer gock.OffAll()
	// No mock lists the project's keys: a publishable key must not create a default key first.
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		JSON(map[string]any{"name": "web_client", "type": "publishable", "description": "Web client"}).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(testApiKeyUUID),
			Name:   "web_client",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
			ApiKey: nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(api.ApiKeyResponse{
			Id:          nullable.NewNullableWithValue(testApiKeyUUID),
			Name:        "web_client",
			Type:        nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
			Description: nullable.NewNullableWithValue("Web client"),
			ApiKey:      nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApikeyResourceConfigPublishable,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.web", "id", testApiKeyUUID),
					resource.TestCheckResourceAttr("supabase_apikey.web", "type", "publishable"),
					resource.TestCheckResourceAttr("supabase_apikey.web", "description", "Web client"),
					resource.TestCheckNoResourceAttr("supabase_apikey.web", "secret_jwt_template.role"),
				),
			},
			// Removing `type` from config keeps the key, as for a key that was imported.
			{
				Config:   testAccApikeyResourceConfigPublishableWithoutType,
				PlanOnly: true,
			},
			{
				Config:             testAccApikeyResourceConfigPublishableToSecret,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("supabase_apikey.web", plancheck.ResourceActionReplace),
					},
				},
			},
		},
	})
}

func TestAccApiKeyResource_InvalidType(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccApikeyResourceConfigInvalidType,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}

func TestAccApiKeyResource_InvalidName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccApikeyResourceConfigInvalidName,
				ExpectError: regexp.MustCompile(`Name must start with a lowercase letter or an underscore`),
			},
		},
	})
}

func TestAccApiKeyResource_WithDescription(t *testing.T) {
	defer gock.OffAll()

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
		AddMatcher(matchJSONBody(t, map[string]any{
			"name":                "test",
			"type":                "secret",
			"description":         "Service key for test",
			"secret_jwt_template": map[string]any{"role": "service_role"},
		})).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:          nullable.NewNullableWithValue(testApiKeyUUID),
			Name:        "test",
			Type:        nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey:      nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			Description: nullable.NewNullableWithValue("Service key for test"),
		})
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(api.ApiKeyResponse{
			Id:          nullable.NewNullableWithValue(testApiKeyUUID),
			Name:        "test",
			Type:        nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey:      nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			Description: nullable.NewNullableWithValue("Service key for test"),
			SecretJwtTemplate: nullable.NewNullableWithValue(map[string]interface{}{
				"role": "service_role",
			}),
		})
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApikeyResourceConfigWithDescription,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "id", testApiKeyUUID),
					resource.TestCheckResourceAttr("supabase_apikey.new", "description", "Service key for test"),
				),
			},
		},
	})
}

func TestNullableDescription(t *testing.T) {
	t.Parallel()

	got := nullableDescription(types.StringNull())
	if got.IsSpecified() {
		t.Fatalf("expected unspecified nullable for null description, got %#v", got)
	}

	got = nullableDescription(types.StringValue("Service key for test"))
	if !got.IsSpecified() || got.IsNull() || got.MustGet() != "Service key for test" {
		t.Fatalf("expected nullable value for description, got %#v", got)
	}
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
