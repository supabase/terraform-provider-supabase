// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"gopkg.in/h2non/gock.v1"
)

const testSecretUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

func TestAccVaultSecretResource(t *testing.T) {
	// Setup mock API
	defer gock.OffAll()

	// CHECK: mock the pre-create existence check - returns no results (secret doesn't exist)
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT id FROM vault.secrets WHERE name = 'my-secret' LIMIT 1",
		}).
		Reply(http.StatusOK).
		JSON([][]interface{}{}) // Empty result

	// CREATE: mock vault.create_secret() response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT vault.create_secret('my-secret-value', 'my-secret', 'My test secret')",
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"create_secret": testSecretUUID},
		})

	// READ after CREATE: mock decrypted_secrets query response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT decrypted_secret FROM vault.decrypted_secrets WHERE id = '%s'", testSecretUUID),
		}).
		Times(2). // Once for Create verification, once for Import state verification
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"decrypted_secret": "my-secret-value"},
		})

	// UPDATE: mock vault.update_secret() response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT vault.update_secret('%s', 'updated-value', 'updated-name', 'Updated description')", testSecretUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"update_secret": testSecretUUID},
		})

	// READ after UPDATE: mock decrypted_secrets query response with updated value
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT decrypted_secret FROM vault.decrypted_secrets WHERE id = '%s'", testSecretUUID),
		}).
		Times(2). // Once for Update verification, once for final refresh
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"decrypted_secret": "updated-value"},
		})

	// DELETE: mock vault.secrets delete response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("DELETE FROM vault.secrets WHERE id = '%s'", testSecretUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{})

	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccVaultSecretResourceConfig("my-secret-value", "my-secret", "My test secret"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "id", testSecretUUID),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "value", "my-secret-value"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "name", "my-secret"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "description", "My test secret"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "supabase_vault_secret.test",
				ImportState:       true,
				ImportStateVerify: true,
				// value is not returned by import, only by read
				ImportStateVerifyIgnore: []string{"name", "description"},
				ImportStateId:           fmt.Sprintf("%s:%s", testProjectRef, testSecretUUID),
			},
			// Update and Read testing
			{
				Config: testAccVaultSecretResourceConfig("updated-value", "updated-name", "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "id", testSecretUUID),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "value", "updated-value"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "name", "updated-name"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "description", "Updated description"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccVaultSecretResourceConfig(value, name, description string) string {
	return fmt.Sprintf(`
resource "supabase_vault_secret" "test" {
  project_ref = %[1]q
  value       = %[2]q
  name        = %[3]q
  description = %[4]q
}
`, testProjectRef, value, name, description)
}

func TestAccVaultSecretResource_NoDescription(t *testing.T) {
	// Setup mock API
	defer gock.OffAll()

	secretUUID := "b2c3d4e5-f6a7-8901-bcde-f12345678901"

	// CHECK: mock the pre-create existence check - returns no results
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT id FROM vault.secrets WHERE name = 'test-name' LIMIT 1",
		}).
		Reply(http.StatusOK).
		JSON([][]interface{}{}) // Empty result

	// CREATE: mock vault.create_secret() with NULL description
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT vault.create_secret('test-value', 'test-name', NULL)",
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"create_secret": secretUUID},
		})

	// READ after CREATE: mock decrypted_secrets query response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT decrypted_secret FROM vault.decrypted_secrets WHERE id = '%s'", secretUUID),
		}).
		Times(2). // Once for Create verification, once for final refresh
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"decrypted_secret": "test-value"},
		})

	// DELETE: mock vault.secrets delete response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("DELETE FROM vault.secrets WHERE id = '%s'", secretUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{})

	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing without description
			{
				Config: testAccVaultSecretResourceConfigNoDescription("test-value", "test-name"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "id", secretUUID),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "value", "test-value"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "name", "test-name"),
					resource.TestCheckNoResourceAttr("supabase_vault_secret.test", "description"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccVaultSecretResourceConfigNoDescription(value, name string) string {
	return fmt.Sprintf(`
resource "supabase_vault_secret" "test" {
  project_ref = %[1]q
  value       = %[2]q
  name        = %[3]q
}
`, testProjectRef, value, name)
}

func TestAccVaultSecretResource_UpdateNoDescription(t *testing.T) {
	// Setup mock API
	defer gock.OffAll()

	secretUUID := "c3d4e5f6-a7b8-9012-cdef-123456789012"

	// CHECK: mock the pre-create existence check - returns no results
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT id FROM vault.secrets WHERE name = 'initial-name' LIMIT 1",
		}).
		Reply(http.StatusOK).
		JSON([][]interface{}{}) // Empty result

	// CREATE: mock vault.create_secret() with description
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT vault.create_secret('initial-value', 'initial-name', 'Initial description')",
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"create_secret": secretUUID},
		})

	// READ after CREATE
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT decrypted_secret FROM vault.decrypted_secrets WHERE id = '%s'", secretUUID),
		}).
		Times(2).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"decrypted_secret": "initial-value"},
		})

	// UPDATE: mock vault.update_secret() with NULL description
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT vault.update_secret('%s', 'updated-value', 'updated-name', NULL)", secretUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"update_secret": secretUUID},
		})

	// READ after UPDATE
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT decrypted_secret FROM vault.decrypted_secrets WHERE id = '%s'", secretUUID),
		}).
		Times(2).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"decrypted_secret": "updated-value"},
		})

	// DELETE
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("DELETE FROM vault.secrets WHERE id = '%s'", secretUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{})

	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with description
			{
				Config: testAccVaultSecretResourceConfig("initial-value", "initial-name", "Initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "name", "initial-name"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "description", "Initial description"),
				),
			},
			// Update to remove description
			{
				Config: testAccVaultSecretResourceConfigNoDescription("updated-value", "updated-name"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "id", secretUUID),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "value", "updated-value"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "name", "updated-name"),
					resource.TestCheckNoResourceAttr("supabase_vault_secret.test", "description"),
				),
			},
		},
	})
}

// TestEscapeSQLLiteral tests the SQL escaping function to prevent SQL injection
func TestEscapeSQLLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected string
	}{
		{
			name:     "nil pointer",
			input:    nil,
			expected: "NULL",
		},
		{
			name:     "simple string",
			input:    stringPtr("hello"),
			expected: "'hello'",
		},
		{
			name:     "string with single quote",
			input:    stringPtr("it's"),
			expected: "'it''s'",
		},
		{
			name:     "SQL injection attempt",
			input:    stringPtr("'; DROP TABLE users; --"),
			expected: "'''; DROP TABLE users; --'",
		},
		{
			name:     "multiple single quotes",
			input:    stringPtr("''test''"),
			expected: "'''''test'''''",
		},
		{
			name:     "empty string",
			input:    stringPtr(""),
			expected: "''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeSQLLiteral(tt.input)
			if result != tt.expected {
				t.Errorf("escapeSQLLiteral(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestEscapeSQLLiteralValue tests the SQL escaping function for non-pointer values
func TestEscapeSQLLiteralValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello",
			expected: "'hello'",
		},
		{
			name:     "string with single quote",
			input:    "it's",
			expected: "'it''s'",
		},
		{
			name:     "SQL injection attempt with UNION",
			input:    "' UNION SELECT * FROM secrets --",
			expected: "''' UNION SELECT * FROM secrets --'",
		},
		{
			name:     "SQL injection with newlines",
			input:    "value'\nDROP TABLE vault.secrets;\n--",
			expected: "'value''\nDROP TABLE vault.secrets;\n--'",
		},
		{
			name:     "unicode characters",
			input:    "Hello 世界 🌍",
			expected: "'Hello 世界 🌍'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeSQLLiteralValue(tt.input)
			if result != tt.expected {
				t.Errorf("escapeSQLLiteralValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// TestSQLQueryGeneration validates that SQL queries are generated correctly
func TestSQLQueryGeneration(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		secretName  string
		description *string
		expected    string
	}{
		{
			name:        "with description",
			value:       "secret-value",
			secretName:  "secret-name",
			description: stringPtr("test description"),
			expected:    "SELECT vault.create_secret('secret-value', 'secret-name', 'test description')",
		},
		{
			name:        "without description",
			value:       "secret-value",
			secretName:  "secret-name",
			description: nil,
			expected:    "SELECT vault.create_secret('secret-value', 'secret-name', NULL)",
		},
		{
			name:        "with single quote in name",
			value:       "value",
			secretName:  "name's",
			description: nil,
			expected:    "SELECT vault.create_secret('value', 'name''s', NULL)",
		},
		{
			name:        "with single quote in description",
			value:       "value",
			secretName:  "name",
			description: stringPtr("it's a test"),
			expected:    "SELECT vault.create_secret('value', 'name', 'it''s a test')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := escapeSQLLiteralValue(tt.value)
			name := escapeSQLLiteralValue(tt.secretName)
			description := escapeSQLLiteral(tt.description)

			query := fmt.Sprintf("SELECT vault.create_secret(%s, %s, %s)", value, name, description)
			if query != tt.expected {
				t.Errorf("SQL query mismatch:\ngot:  %s\nwant: %s", query, tt.expected)
			}
		})
	}
}

func TestAccVaultSecretResource_PreExisting(t *testing.T) {
	// Test that Create updates an existing secret instead of failing
	defer gock.OffAll()

	existingUUID := "pre-existing-uuid-1234"

	// CHECK: mock the pre-create existence check - returns existing secret ID
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT id FROM vault.secrets WHERE name = 'existing-secret' LIMIT 1",
		}).
		Reply(http.StatusOK).
		JSON([][]interface{}{
			{existingUUID},
		})

	// UPDATE: mock vault.update_secret() since secret already exists
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT vault.update_secret('%s', 'new-value', 'existing-secret', 'Updated via terraform')", existingUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"update_secret": existingUUID},
		})

	// READ after UPDATE: mock decrypted_secrets query response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT decrypted_secret FROM vault.decrypted_secrets WHERE id = '%s'", existingUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"decrypted_secret": "new-value"},
		})

	// DELETE: mock vault.secrets delete response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("DELETE FROM vault.secrets WHERE id = '%s'", existingUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{})

	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVaultSecretResourceConfig("new-value", "existing-secret", "Updated via terraform"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "id", existingUUID),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "value", "new-value"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "name", "existing-secret"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "description", "Updated via terraform"),
				),
			},
		},
	})
}

func TestAccVaultSecretResource_DuplicateKeyRace(t *testing.T) {
	// Test that Create handles duplicate key error (race condition) gracefully
	defer gock.OffAll()

	raceUUID := "race-condition-uuid-5678"

	// CHECK: mock the pre-create existence check - returns no results (secret doesn't exist yet)
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT id FROM vault.secrets WHERE name = 'race-secret' LIMIT 1",
		}).
		Reply(http.StatusOK).
		JSON([][]interface{}{}) // Empty result

	// CREATE: mock vault.create_secret() returning duplicate key error
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT vault.create_secret('race-value', 'race-secret', 'Race condition test')",
		}).
		Reply(http.StatusBadRequest).
		JSON(map[string]interface{}{
			"message": "Failed to run sql query: ERROR:  23505: duplicate key value violates unique constraint \"secrets_name_idx\"\nDETAIL:  Key (name)=(race-secret) already exists.",
		})

	// RE-CHECK: mock the re-check query after duplicate key error - now returns existing secret ID
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "SELECT id FROM vault.secrets WHERE name = 'race-secret' LIMIT 1",
		}).
		Reply(http.StatusOK).
		JSON([][]interface{}{
			{raceUUID},
		})

	// UPDATE: mock vault.update_secret() to update the existing secret
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT vault.update_secret('%s', 'race-value', 'race-secret', 'Race condition test')", raceUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"update_secret": raceUUID},
		})

	// READ after UPDATE: mock decrypted_secrets query response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("SELECT decrypted_secret FROM vault.decrypted_secrets WHERE id = '%s'", raceUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{
			{"decrypted_secret": "race-value"},
		})

	// DELETE: mock vault.secrets delete response
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/database/query", testProjectRef)).
		MatchType("json").
		JSON(map[string]interface{}{
			"query": fmt.Sprintf("DELETE FROM vault.secrets WHERE id = '%s'", raceUUID),
		}).
		Reply(http.StatusOK).
		JSON([]map[string]interface{}{})

	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVaultSecretResourceConfig("race-value", "race-secret", "Race condition test"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "id", raceUUID),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "value", "race-value"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "name", "race-secret"),
					resource.TestCheckResourceAttr("supabase_vault_secret.test", "description", "Race condition test"),
				),
			},
		},
	})
}
