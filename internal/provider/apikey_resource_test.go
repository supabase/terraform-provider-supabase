// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"maps"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
	"gopkg.in/h2non/gock.v1"
)

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

func testAccApiKeyImportStateID(projectResource, apiKeyResource string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		project, ok := s.RootModule().Resources[projectResource]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", projectResource)
		}
		key, ok := s.RootModule().Resources[apiKeyResource]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", apiKeyResource)
		}
		return fmt.Sprintf("%s/%s", project.Primary.ID, key.Primary.ID), nil
	}
}

func testAccApiKeyLiveVariables(organizationID, projectName, name string, extra config.Variables) config.Variables {
	vars := config.Variables{
		"organization_id": config.StringVariable(organizationID),
		"project_name":    config.StringVariable(projectName),
		"name":            config.StringVariable(name),
	}
	maps.Copy(vars, extra)
	return vars
}

func TestAccApiKeyResource_basic(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_apikey.test"
	projectName := acctest.RandomName()
	keyName := "test_key"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ApiKey/basic/"),
				ConfigVariables: testAccApiKeyLiveVariables(organizationID, projectName, keyName, nil),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.CompareValuePairs(
						resourceName, tfjsonpath.New("project_ref"),
						projectResourceName, tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(keyName)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("type"), knownvalue.StringExact("secret")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("api_key"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("secret_jwt_template"), knownvalue.ObjectExact(map[string]knownvalue.Check{
						"role": knownvalue.Null(),
					})),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.Null()),
				},
			},
			// ImportState testing
			{
				ConfigDirectory:   config.StaticDirectory("test-fixtures/ApiKey/basic/"),
				ConfigVariables:   testAccApiKeyLiveVariables(organizationID, projectName, keyName, nil),
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccApiKeyImportStateID(projectResourceName, resourceName),
			},
		},
	})
}

func TestAccApiKeyResource_update(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_apikey.test"
	projectName := acctest.RandomName()
	keyName := "test_key"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create without description
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ApiKey/basic/"),
				ConfigVariables: testAccApiKeyLiveVariables(organizationID, projectName, keyName, nil),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(keyName)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.Null()),
				},
			},
			// Add description
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ApiKey/basic/"),
				ConfigVariables: testAccApiKeyLiveVariables(organizationID, projectName, keyName, config.Variables{
					"description": config.StringVariable("Service key for test"),
				}),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(keyName)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.StringExact("Service key for test")),
				},
			},
			// Update description
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ApiKey/basic/"),
				ConfigVariables: testAccApiKeyLiveVariables(organizationID, projectName, keyName, config.Variables{
					"description": config.StringVariable("Updated description"),
				}),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.StringExact("Updated description")),
				},
			},
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ApiKey/basic/"),
				ConfigVariables: testAccApiKeyLiveVariables(organizationID, projectName, keyName, config.Variables{
					"description": config.StringVariable("Updated description"),
				}),
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccApiKeyImportStateID(projectResourceName, resourceName),
			},
		},
	})
}

func TestAccApiKeyResource_invalidName(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ApiKey/basic/"),
				ConfigVariables: testAccApiKeyLiveVariables(organizationID, acctest.RandomName(), "Invalid-Name-123", nil),
				// Terraform wraps long diagnostics across lines, so whitespace is matched with \s+.
				ExpectError: regexp.MustCompile(`Name\s+must\s+start\s+with\s+a\s+lowercase\s+letter`),
			},
		},
	})
}
