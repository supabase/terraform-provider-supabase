package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"

	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestAccEdgeFunctionSecretsResource(t *testing.T) {
	// Verify that new secret values creates the secrets
	// on the server.
	defer gock.OffAll()

	apiKeyPlain := "secret-api-key-123"
	dbUrlPlain := "postgresql://user:pass@localhost:5432/db"

	// Pre-compute SHA-256 digests matching what the API returns
	apiKeyDigest := computeSecretDigest(apiKeyPlain)
	dbUrlDigest := computeSecretDigest(dbUrlPlain)

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		},
		{
			name  = "DATABASE_URL"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyPlain, dbUrlPlain)

	// Mock create secrets
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Mock read secrets after create and refresh – API returns SHA-256 digests, not plaintext
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{
				Name:  "API_KEY",
				Value: apiKeyDigest,
			},
			{
				Name:  "DATABASE_URL",
				Value: dbUrlDigest,
			},
		})

	// Mock delete secrets
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", testProjectRef),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "2"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "2"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_Update(t *testing.T) {
	// Verify that changing secret values updates the secrets
	// on the server.
	defer gock.OffAll()

	apiKeyV1 := "secret-v1"
	apiKeyV2 := "secret-v2"
	dbUrlPlain := "postgresql://user:pass@localhost:5432/db"

	digestV1 := computeSecretDigest(apiKeyV1)
	digestV2 := computeSecretDigest(apiKeyV2)
	dbUrlDigest := computeSecretDigest(dbUrlPlain)

	config1 := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		},
		{
			name  = "DATABASE_URL"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyV1, dbUrlPlain)

	config2 := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		},
		{
			name  = "DATABASE_URL"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyV2, dbUrlPlain)

	// Step 1: create
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Step 1: read after create and refresh
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: digestV1},
			{Name: "DATABASE_URL", Value: dbUrlDigest},
		})

	// Step 2: update – create/upsert secrets
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Step 2: read after update and refresh
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: digestV2},
			{Name: "DATABASE_URL", Value: dbUrlDigest},
		})

	// Teardown: delete
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: config1,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", digestV1),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
			{
				Config: config2,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", digestV2),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_DeleteOneSecret(t *testing.T) {
	// Verify that when a secret is removed from Terraform config,
	// only that secret is deleted from the server and the other remains.
	defer gock.OffAll()

	keyAPlain := "secret-key-a"
	keyBPlain := "secret-key-b"

	keyADigest := computeSecretDigest(keyAPlain)
	keyBDigest := computeSecretDigest(keyBPlain)

	// Config with 2 secrets
	config1 := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "KEY_A"
			value = "%s"
		},
		{
			name  = "KEY_B"
			value = "%s"
		}
	]
}
`, testProjectRef, keyAPlain, keyBPlain)

	// Config with only 1 secret (KEY_B remains, KEY_A removed)
	config2 := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "KEY_B"
			value = "%s"
		}
	]
}
`, testProjectRef, keyBPlain)

	// Step 1: Create both secrets
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Step 1: Read after create and refresh
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "KEY_A", Value: keyADigest},
			{Name: "KEY_B", Value: keyBDigest},
		})

	// Step 2: Delete the removed secret (KEY_A)
	deleteCalled := false
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		AddMatcher(func(req *http.Request, greq *gock.Request) (bool, error) {
			// Read and verify that only KEY_A is sent in the delete request body
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				return false, fmt.Errorf("failed to read delete request body: %w", err)
			}
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			// Decode the JSON array
			var secretNames []string
			if err := json.Unmarshal(bodyBytes, &secretNames); err != nil {
				return false, fmt.Errorf("failed to decode JSON body: %w", err)
			}

			// Check that exactly one secret is being deleted and it's KEY_A
			if len(secretNames) != 1 {
				return false, fmt.Errorf("expected 1 secret to delete, got %d: %v", len(secretNames), secretNames)
			}
			if secretNames[0] != "KEY_A" {
				return false, fmt.Errorf("expected KEY_A to be deleted, got %s", secretNames[0])
			}

			deleteCalled = true
			return true, nil
		}).
		Reply(http.StatusOK)

	// Step 2: Update remaining secret (bulk upsert KEY_B)
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Step 2: Read after update and refresh
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "KEY_B", Value: keyBDigest},
		})

	// Teardown: Delete remaining secrets
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: config1,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "2"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "2"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.KEY_A", keyADigest),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.KEY_B", keyBDigest),
				),
			},
			{
				Config: config2,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "1"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "1"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.KEY_B", keyBDigest),
					testresource.TestCheckNoResourceAttr("supabase_edge_function_secrets.test", "secret_digests.KEY_A"),
					func(s *terraform.State) error {
						if !deleteCalled {
							return fmt.Errorf("DELETE API was not called to remove KEY_A")
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_Import(t *testing.T) {
	// Verify that importing resources succeeds
	defer gock.OffAll()

	apiKeyPlain := "secret-api-key-123"
	dbUrlPlain := "postgresql://user:pass@localhost:5432/db"

	apiKeyDigest := computeSecretDigest(apiKeyPlain)
	dbUrlDigest := computeSecretDigest(dbUrlPlain)

	secretsResponse := []api.SecretResponse{
		{Name: "API_KEY", Value: apiKeyDigest},
		{Name: "DATABASE_URL", Value: dbUrlDigest},
	}

	testConfig := fmt.Sprintf(`
	resource "supabase_edge_function_secrets" "test" {
		project_ref = "%s"
		secrets = []
	}
	`, testProjectRef)

	// Read after refresh, import and import verification
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(3).
		Reply(http.StatusOK).
		JSON(secretsResponse)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				// Import by project_ref. The imported state uses API digests in
				// the value field (no plaintext available on import).
				// We use ImportStateCheck to verify the imported state rather than
				// ImportStateVerify because we're importing without creating first.
				Config:        testConfig,
				ResourceName:  "supabase_edge_function_secrets.test",
				ImportState:   true,
				ImportStateId: testProjectRef,
				ImportStateCheck: func(s []*terraform.InstanceState) error {
					if len(s) != 1 {
						return fmt.Errorf("expected 1 instance state, got %d", len(s))
					}
					state := s[0]
					if state.Attributes["project_ref"] != testProjectRef {
						return fmt.Errorf("expected project_ref %q, got %q", testProjectRef, state.Attributes["project_ref"])
					}
					if state.Attributes["secret_digests.%"] != "2" {
						return fmt.Errorf("expected 2 secret_digests, got %s", state.Attributes["secret_digests.%"])
					}
					if state.Attributes["secret_digests.API_KEY"] != apiKeyDigest {
						return fmt.Errorf("expected API_KEY digest %q, got %q", apiKeyDigest, state.Attributes["secret_digests.API_KEY"])
					}
					if state.Attributes["secret_digests.DATABASE_URL"] != dbUrlDigest {
						return fmt.Errorf("expected DATABASE_URL digest %q, got %q", dbUrlDigest, state.Attributes["secret_digests.DATABASE_URL"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_CreateImport(t *testing.T) {
	// Verify that importing the same resources as already created
	// successfully imports them
	defer gock.OffAll()

	apiKeyPlain := "secret-api-key-123"
	dbUrlPlain := "postgresql://user:pass@localhost:5432/db"

	apiKeyDigest := computeSecretDigest(apiKeyPlain)
	dbUrlDigest := computeSecretDigest(dbUrlPlain)

	secretsResponse := []api.SecretResponse{
		{Name: "API_KEY", Value: apiKeyDigest},
		{Name: "DATABASE_URL", Value: dbUrlDigest},
	}

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		},
		{
			name  = "DATABASE_URL"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyPlain, dbUrlPlain)

	// Step 1: create
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Step 1: read after create, refresh, import and import verification
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(4).
		Reply(http.StatusOK).
		JSON(secretsResponse)

	// Teardown: delete
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", testProjectRef),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
			{
				// Import by project_ref. The imported state uses API digests in
				// the value field (no plaintext available on import), so we ignore
				// the secrets attribute during verification (values differ from
				// original plaintext). The resource has no "id" attribute; use
				// project_ref as the identifier for verification.
				ResourceName:                         "supabase_edge_function_secrets.test",
				ImportState:                          true,
				ImportStateId:                        testProjectRef,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "project_ref",
				ImportStateVerifyIgnore: []string{
					// secret values are not returned by the API; the imported
					// state will contain the digest rather than the original plaintext
					"secrets",
				},
				ImportStateCheck: func(s []*terraform.InstanceState) error {
					if len(s) != 1 {
						return fmt.Errorf("expected 1 instance state, got %d", len(s))
					}
					state := s[0]
					if state.Attributes["project_ref"] != testProjectRef {
						return fmt.Errorf("expected project_ref %q, got %q", testProjectRef, state.Attributes["project_ref"])
					}
					if state.Attributes["secret_digests.%"] != "2" {
						return fmt.Errorf("expected 2 secret_digests, got %s", state.Attributes["secret_digests.%"])
					}
					if state.Attributes["secret_digests.API_KEY"] != apiKeyDigest {
						return fmt.Errorf("expected API_KEY digest %q, got %q", apiKeyDigest, state.Attributes["secret_digests.API_KEY"])
					}
					if state.Attributes["secret_digests.DATABASE_URL"] != dbUrlDigest {
						return fmt.Errorf("expected DATABASE_URL digest %q, got %q", dbUrlDigest, state.Attributes["secret_digests.DATABASE_URL"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_ReadDrift(t *testing.T) {
	// Verify the drift-detection behaviour: when the API returns a digest that
	// does not match the locally stored digest, the resource stores null in the
	// value field so that Terraform detects the drift and plans an update on
	// the next apply.
	defer gock.OffAll()

	apiKeyPlain := "original-secret"
	apiKeyDigest := computeSecretDigest(apiKeyPlain)

	// The "drifted" digest simulates someone updating the secret out-of-band.
	driftedDigest := computeSecretDigest("some-other-value-set-outside-terraform")

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyPlain)

	// Step 1: create
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Step 1: read after create and refresh - return original digest
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	// Step 2: reads for drift detection - return drifted digest
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: driftedDigest},
		})

	// Step 3: apply to fix drift - update back to original
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Step 3: reads after apply and refresh - return original digest
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	// Teardown: delete
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			// Step 1: create with original value – verify matching digests in state
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
				),
			},
			// Step 2: same config, but API now returns drifted digest.
			// The resource's Read will detect the mismatch and update the state
			// with the drifted digest, causing an ExpectNonEmptyPlan.
			{
				Config:             testConfig,
				ExpectNonEmptyPlan: true,
			},
			// Step 3: apply the original value back to fix the drift.
			// After applying, the plan should be empty (no more drift).
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
				),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_EmptySecrets(t *testing.T) {
	// Verify that configuring an empty secrets set is safe
	// and does not trigger any API delete calls.
	defer gock.OffAll()

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets     = []
}
`, testProjectRef)

	// Create call: empty body – still expected to succeed
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// All reads return an empty list (post-create read and refresh read)
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(3).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{})

	// Teardown: delete is a no-op (no secret names to send), so no HTTP call expected.

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", testProjectRef),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "0"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "0"),
				),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_CreateAPIError(t *testing.T) {
	// Verify that a non-2xx response from the create endpoint
	// surfaces a useful error diagnostic.
	defer gock.OffAll()

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "some-value"
		}
	]
}
`, testProjectRef)

	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusInternalServerError).
		BodyString(`{"message":"internal server error"}`)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config:      testConfig,
				ExpectError: regexp.MustCompile("API Error"),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_ReadAPIError(t *testing.T) {
	// Verify that when the list secrets endpoint returns an unexpected
	// error status the provider surfaces an error diagnostic rather than
	// silently storing empty state.
	defer gock.OffAll()

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "some-value"
		}
	]
}
`, testProjectRef)

	// Create succeeds...
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// ...but the subsequent read fails with 500. Persist the mock because the
	// provider retries transient GET failures before surfacing the error.
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Persist().
		Reply(http.StatusInternalServerError).
		BodyString(`{"message":"internal server error"}`)

	// If the provider committed partial state before the error, the framework
	// will attempt a destroy. No state should be committed on a failed Create,
	// but register a mock as a safety net to prevent a spurious teardown error.
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config:      testConfig,
				ExpectError: regexp.MustCompile("API Error"),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_ImportNotFound(t *testing.T) {
	// Verify that importing a project_ref that 404s results in a clear
	// "Resource Not Found" error. A real first step is needed to provide
	// a config context for the import step.
	defer gock.OffAll()

	notFoundRef := "nonexistentprojectref"

	apiKeyPlain := "secret-api-key"
	apiKeyDigest := computeSecretDigest(apiKeyPlain)

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyPlain)

	// Step 1: create and read the real resource
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	// Step 2: import a different project_ref that returns 404
	gock.New(defaultApiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", notFoundRef)).
		Reply(http.StatusNotFound)

	// Teardown: delete the real resource
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			// Step 1: create a real resource to provide config context
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", testProjectRef),
				),
			},
			// Step 2: attempt to import a non-existent project – should error
			{
				Config:        testConfig,
				ResourceName:  "supabase_edge_function_secrets.test",
				ImportState:   true,
				ImportStateId: notFoundRef,
				ExpectError:   regexp.MustCompile("Resource Not Found"),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_FilterSupabaseSecrets(t *testing.T) {
	// Verify that secrets with names starting with SUPABASE_ are filtered out
	// from the read operation, as the API does not allow create/update/delete
	// operations on these secrets.
	defer gock.OffAll()

	apiKeyPlain := "secret-api-key-123"
	apiKeyDigest := computeSecretDigest(apiKeyPlain)

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyPlain)

	// Mock create secrets
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Mock read secrets after create – API returns both user secrets and SUPABASE_ prefixed secrets
	// The SUPABASE_ secrets should be filtered out and not appear in state
	// Also the second read is for refresh
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{
				Name:  "API_KEY",
				Value: apiKeyDigest,
			},
			{
				Name:  "SUPABASE_URL",
				Value: computeSecretDigest("https://example.supabase.co"),
			},
			{
				Name:  "SUPABASE_ANON_KEY",
				Value: computeSecretDigest("anon-key-value"),
			},
		})

	// Mock delete secrets
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", testProjectRef),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "1"),
					// Verify only 1 secret digest (API_KEY) – SUPABASE_ secrets should be filtered
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "1"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
					// Verify SUPABASE_ secrets are NOT in state
					testresource.TestCheckNoResourceAttr("supabase_edge_function_secrets.test", "secret_digests.SUPABASE_URL"),
					testresource.TestCheckNoResourceAttr("supabase_edge_function_secrets.test", "secret_digests.SUPABASE_ANON_KEY"),
				),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_CreateReservedPrefixFails(t *testing.T) {
	// Verify that attempting to create secrets with names starting with
	// SUPABASE_ returns an error from the API, which the provider surfaces
	// to the user.
	defer gock.OffAll()

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "valid-secret"
		},
		{
			name  = "SUPABASE_URL"
			value = "https://example.supabase.co"
		}
	]
}
`, testProjectRef)

	// Mock create secrets - API rejects request containing SUPABASE_ prefix
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusBadRequest).
		BodyString(`{"message":"Secret names starting with SUPABASE_ are reserved"}`)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config:      testConfig,
				ExpectError: regexp.MustCompile(supabasePrefix),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_UpdateReservedPrefixFails(t *testing.T) {
	// Verify that attempting to update (add or modify) secrets with names
	// starting with SUPABASE_ returns an error from the API, which the
	// provider surfaces to the user.
	defer gock.OffAll()

	apiKeyPlain := "secret-api-key-123"
	apiKeyDigest := computeSecretDigest(apiKeyPlain)

	// Initial config with valid secrets only
	config1 := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyPlain)

	// Updated config attempting to add a SUPABASE_ prefixed secret
	config2 := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		},
		{
			name  = "SUPABASE_ANON_KEY"
			value = "anon-key-value"
		}
	]
}
`, testProjectRef, apiKeyPlain)

	// Step 1: create initial valid secrets
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Step 1: read after create and refresh
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	// Step 2: attempt to update with reserved prefix - API rejects
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusBadRequest).
		BodyString(`{"message":"Secret names starting with SUPABASE_ are reserved"}`)

	// Final cleanup: delete resources (since first step succeeded, state exists)
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: config1,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", testProjectRef),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "1"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
				),
			},
			{
				Config:      config2,
				ExpectError: regexp.MustCompile(supabasePrefix),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_IgnoresUnmanagedSecrets(t *testing.T) {
	// Verify that when the API returns secrets that are not declared in the
	// Terraform configuration, those unmanaged secrets are NOT added to the
	// state. This prevents Terraform from planning unnecessary updates when
	// pre-existing secrets exist in the project.
	defer gock.OffAll()

	apiKeyPlain := "test-api-key"
	apiKeyDigest := computeSecretDigest(apiKeyPlain)

	// Pre-existing secret that exists in the project but is NOT managed by Terraform
	managementApiUrlDigest := computeSecretDigest("https://api.management.example.com")

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyPlain)

	// Step 1: create - user only creates API_KEY
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Step 1: read after create and refresh
	// API returns BOTH the managed API_KEY AND the pre-existing MANAGEMENT_API_URL
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
			{Name: "MANAGEMENT_API_URL", Value: managementApiUrlDigest}, // Pre-existing, unmanaged
		})

	// Step 2: additional refresh to verify no plan needed
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(1).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
			{Name: "MANAGEMENT_API_URL", Value: managementApiUrlDigest},
		})

	// Teardown: delete - should only delete API_KEY (the managed secret)
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", testProjectRef),
					// CRITICAL: only 1 secret should be in state (API_KEY), not 2
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "1"),
					// CRITICAL: only 1 digest should be tracked
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "1"),
					// Verify API_KEY is present
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
					// Verify MANAGEMENT_API_URL is NOT in state
					testresource.TestCheckNoResourceAttr("supabase_edge_function_secrets.test", "secret_digests.MANAGEMENT_API_URL"),
				),
			},
			// Step 2: second plan should be empty (no updates needed)
			{
				Config:             testConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestSecretDigestsPlanModifier_ComputesDigests(t *testing.T) {
	// Verify that the plan modifier correctly computes digests from
	// known secret values during the plan phase, WITHOUT reading from the API.
	defer gock.OffAll()

	apiKeyPlain := "test-api-key"
	dbUrlPlain := "postgresql://localhost:5432/db"

	apiKeyDigest := computeSecretDigest(apiKeyPlain)
	dbUrlDigest := computeSecretDigest(dbUrlPlain)

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		},
		{
			name  = "DATABASE_URL"
			value = "%s"
		}
	]
}
`, testProjectRef, apiKeyPlain, dbUrlPlain)

	// Mock create secrets
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Mock read secrets - return digests
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
			{Name: "DATABASE_URL", Value: dbUrlDigest},
		})

	// Mock delete
	gock.New(defaultApiEndpoint).
		Delete(secretsApiPath).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: testConfig,
				// ConfigPlanChecks verify the PLAN before apply/read
				// This proves digests are computed during plan, not read from API
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectKnownValue(
							"supabase_edge_function_secrets.test",
							tfjsonpath.New("secret_digests").AtMapKey("API_KEY"),
							knownvalue.StringExact(apiKeyDigest),
						),
						plancheck.ExpectKnownValue(
							"supabase_edge_function_secrets.test",
							tfjsonpath.New("secret_digests").AtMapKey("DATABASE_URL"),
							knownvalue.StringExact(dbUrlDigest),
						),
					},
				},
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "2"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_EmptySecretsNotTreatedAsImport(t *testing.T) {
	// Verify that when a user explicitly configures secrets = [],
	// the Read operation does NOT import remote secrets into the state
	// (i.e., it doesn't conflate an explicit empty list with an import
	// operation).
	defer gock.OffAll()

	apiKeyDigest := computeSecretDigest("remote-secret-value")

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets     = []
}
`, testProjectRef)

	// Create call with empty secrets
	gock.New(defaultApiEndpoint).
		Post(secretsApiPath).
		Reply(http.StatusOK)

	// Read operations: API returns remote secrets that were created out-of-band.
	// With the bug, these would be imported into state even though user said secrets = [].
	// After the fix, these should NOT be imported.
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", testProjectRef),
					// The user explicitly set secrets = [], so the state should remain empty
					// even if remote secrets exist (they were created out-of-band)
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "0"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "0"),
				),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_ImportThenRefresh(t *testing.T) {
	// Verify that after importing secrets (where values are null), a subsequent
	// refresh preserves null values rather than converting them to empty strings
	// when digests match. This ensures imported secrets maintain the "unknown/needs
	// re-supply" signal.
	defer gock.OffAll()

	apiKeyPlain := "secret-api-key-123"
	dbUrlPlain := "postgresql://user:pass@localhost:5432/db"

	apiKeyDigest := computeSecretDigest(apiKeyPlain)
	dbUrlDigest := computeSecretDigest(dbUrlPlain)

	secretsResponse := []api.SecretResponse{
		{Name: "API_KEY", Value: apiKeyDigest},
		{Name: "DATABASE_URL", Value: dbUrlDigest},
	}

	// Empty config for import
	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = []
}
`, testProjectRef)

	// Import: read after import and import verification (2 reads)
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON(secretsResponse)

	// Refresh: read during plan-only step (1 read)
	// This is where the bug would manifest: if null values in state are converted
	// to empty strings, the state would show secret values as ""
	gock.New(defaultApiEndpoint).
		Get(secretsApiPath).
		Times(1).
		Reply(http.StatusOK).
		JSON(secretsResponse)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				// Import secrets - values become null (no plaintext available)
				Config:        testConfig,
				ResourceName:  "supabase_edge_function_secrets.test",
				ImportState:   true,
				ImportStateId: testProjectRef,
				ImportStateCheck: func(s []*terraform.InstanceState) error {
					if len(s) != 1 {
						return fmt.Errorf("expected 1 instance state, got %d", len(s))
					}
					state := s[0]

					// After import, we should have 2 secrets in state
					if state.Attributes["secrets.#"] != "2" {
						return fmt.Errorf("expected 2 secrets in state after import, got %s", state.Attributes["secrets.#"])
					}

					// Check digests are populated
					if state.Attributes["secret_digests.API_KEY"] != apiKeyDigest {
						return fmt.Errorf("expected API_KEY digest %q, got %q", apiKeyDigest, state.Attributes["secret_digests.API_KEY"])
					}
					if state.Attributes["secret_digests.DATABASE_URL"] != dbUrlDigest {
						return fmt.Errorf("expected DATABASE_URL digest %q, got %q", dbUrlDigest, state.Attributes["secret_digests.DATABASE_URL"])
					}

					// CRITICAL BUG CHECK: Secret values should be null (not empty string)
					// After import, plaintext is not available, so values must remain null.
					// With the bug, null becomes "" during Read when digests match.
					// We check that the value attributes don't exist or are explicitly null.
					// If the bug exists, we'd see secrets.0.value = "" or secrets.1.value = ""
					for i := 0; i < 2; i++ {
						valueKey := fmt.Sprintf("secrets.%d.value", i)
						if val, exists := state.Attributes[valueKey]; exists && val == "" {
							nameKey := fmt.Sprintf("secrets.%d.name", i)
							secretName := state.Attributes[nameKey]
							return fmt.Errorf("secret %q has empty string value (should be null after import)", secretName)
						}
					}

					return nil
				},
			},
			{
				// Plan-only step: verifies refresh behavior without applying changes.
				// The imported secrets (with null values) should remain in state after refresh,
				// and null values should be preserved (not converted to empty strings).
				// ExpectNonEmptyPlan because config has secrets=[] but state has 2 imported secrets,
				// so Terraform will plan to delete them (but won't apply since PlanOnly=true).
				Config:             testConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", testProjectRef),
					// After refresh, the imported secrets should still be in state
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "2"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "2"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_ProjectRef_RequiresReplace(t *testing.T) {
	// Verify that changing project_ref triggers resource replacement (destroy + create).
	// Without RequiresReplace, changing project_ref would run Update and upsert secrets
	// into the new project while leaving the old project's managed secrets behind.
	defer gock.OffAll()

	projectRefA := "project-ref-a"
	projectRefB := "project-ref-b"

	apiKeyPlain := "secret-api-key-123"
	apiKeyDigest := computeSecretDigest(apiKeyPlain)

	config1 := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		}
	]
}
`, projectRefA, apiKeyPlain)

	config2 := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		}
	]
}
`, projectRefB, apiKeyPlain)

	// Step 1: Create secrets for project A
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRefA)).
		Reply(http.StatusOK)

	// Step 1: Read after create and refresh for project A
	gock.New(defaultApiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRefA)).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	// Step 1: Delete from project A (when replacing)
	gock.New(defaultApiEndpoint).
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRefA)).
		Reply(http.StatusOK)

	// Step 2: Create secrets for project B (after destroy of project A resource)
	gock.New(defaultApiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRefB)).
		Reply(http.StatusOK)

	// Step 2: Read after create and refresh for project B
	gock.New(defaultApiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRefB)).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	// Teardown: Delete from project B
	gock.New(defaultApiEndpoint).
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRefB)).
		Reply(http.StatusOK)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: config1,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", projectRefA),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "1"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
				),
			},
			{
				Config: config2,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", projectRefB),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "1"),
					testresource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
				),
			},
		},
	})
}
