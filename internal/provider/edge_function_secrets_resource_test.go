package provider

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

// testDigest returns the hex-encoded SHA-256 digest for use in test assertions,
// mirroring the computeSecretDigest function used by the resource.
func testDigest(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

const apiEndpoint = "https://api.supabase.com"

func TestAccEdgeFunctionSecretsResource(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"

	apiKeyPlain := "secret-api-key-123"
	dbUrlPlain := "postgresql://user:pass@localhost:5432/db"

	// Pre-compute SHA-256 digests matching what the API returns
	apiKeyDigest := testDigest(apiKeyPlain)
	dbUrlDigest := testDigest(dbUrlPlain)

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
`, projectRef, apiKeyPlain, dbUrlPlain)

	// Mock create secrets
	gock.New(apiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	// Mock read secrets after create – API returns SHA-256 digests, not plaintext
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
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

	// Mock read secrets for refresh
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
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
	gock.New(apiEndpoint).
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", projectRef),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "2"),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "2"),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
		},
	})
}

// TestAccEdgeFunctionSecretsResource_Update verifies that changing secret values
// triggers a delete-then-recreate cycle and updates digests in state.
func TestAccEdgeFunctionSecretsResource_Update(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"

	apiKeyV1 := "secret-v1"
	apiKeyV2 := "secret-v2"
	dbUrlPlain := "postgresql://user:pass@localhost:5432/db"

	digestV1 := testDigest(apiKeyV1)
	digestV2 := testDigest(apiKeyV2)
	dbUrlDigest := testDigest(dbUrlPlain)

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
`, projectRef, apiKeyV1, dbUrlPlain)

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
`, projectRef, apiKeyV2, dbUrlPlain)

	// Step 1: create
	gock.New(apiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	// Step 1: read after create
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: digestV1},
			{Name: "DATABASE_URL", Value: dbUrlDigest},
		})

	// Step 1: read for refresh (plan check)
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: digestV1},
			{Name: "DATABASE_URL", Value: dbUrlDigest},
		})

	// Step 2: update – delete existing then recreate
	gock.New(apiEndpoint).
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	gock.New(apiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	// Step 2: read after update
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: digestV2},
			{Name: "DATABASE_URL", Value: dbUrlDigest},
		})

	// Step 2: read for refresh
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: digestV2},
			{Name: "DATABASE_URL", Value: dbUrlDigest},
		})

	// Teardown: delete
	gock.New(apiEndpoint).
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", digestV1),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
			{
				Config: config2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", digestV2),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
		},
	})
}

// TestAccEdgeFunctionSecretsResource_Import verifies that a resource can be
// imported by project_ref and that the resulting state contains the correct
// secret_digests. Because the API never returns plaintext values, the imported
// state holds API digests as the secret values; ImportStateVerifyIgnore is used
// for the secret values themselves.
func TestAccEdgeFunctionSecretsResource_Import(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"

	apiKeyPlain := "secret-api-key-123"
	dbUrlPlain := "postgresql://user:pass@localhost:5432/db"

	apiKeyDigest := testDigest(apiKeyPlain)
	dbUrlDigest := testDigest(dbUrlPlain)

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
`, projectRef, apiKeyPlain, dbUrlPlain)

	// Step 1: create
	gock.New(apiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	// Step 1: read after create
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON(secretsResponse)

	// Step 1: read for refresh
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON(secretsResponse)

	// Step 2 (ImportState): read during import
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON(secretsResponse)

	// Step 2 (ImportState): read for import verification
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON(secretsResponse)

	// Teardown: delete
	gock.New(apiEndpoint).
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", projectRef),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
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
				ImportStateId:                        projectRef,
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
					if state.Attributes["project_ref"] != projectRef {
						return fmt.Errorf("expected project_ref %q, got %q", projectRef, state.Attributes["project_ref"])
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

// TestAccEdgeFunctionSecretsResource_ReadDrift verifies the drift-detection
// behaviour: when the API returns a digest that does not match the locally
// stored digest, the resource stores the API digest in the value field so that
// Terraform detects the drift and plans an update on the next apply.
func TestAccEdgeFunctionSecretsResource_ReadDrift(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"

	apiKeyPlain := "original-secret"
	apiKeyDigest := testDigest(apiKeyPlain)

	// The "drifted" digest simulates someone updating the secret out-of-band.
	driftedDigest := testDigest("some-other-value-set-outside-terraform")

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
`, projectRef, apiKeyPlain)

	// Step 1: create
	gock.New(apiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	// Step 1: read after create and refresh - return original digest
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Times(2).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	// Step 2: all subsequent reads return drifted digest
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Persist().
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: driftedDigest},
		})

	// Teardown: delete
	gock.New(apiEndpoint).
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: create with original value – verify matching digests in state
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
				),
			},
			// Step 2: same config, but API now returns drifted digest.
			// The resource's Read will detect the mismatch and update the state
			// with the drifted digest, causing an ExpectNonEmptyPlan.
			{
				Config:             testConfig,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccEdgeFunctionSecretsResource_EmptySecrets verifies that configuring an
// empty secrets set is safe and does not trigger any API delete calls.
func TestAccEdgeFunctionSecretsResource_EmptySecrets(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets     = []
}
`, projectRef)

	// Create call: empty body – still expected to succeed
	gock.New(apiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	// All reads return an empty list (post-create read, refresh read, and
	// the framework's post-step consistency-check refresh).
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Persist().
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{})

	// Teardown: delete is a no-op (no secret names to send), so no HTTP call expected.

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", projectRef),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "0"),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "0"),
				),
			},
		},
	})
}

// TestAccEdgeFunctionSecretsResource_CreateAPIError verifies that a non-2xx
// response from the create endpoint surfaces a useful error diagnostic.
func TestAccEdgeFunctionSecretsResource_CreateAPIError(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"

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
`, projectRef)

	gock.New(apiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusInternalServerError).
		BodyString(`{"message":"internal server error"}`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testConfig,
				ExpectError: regexp.MustCompile("API Error"),
			},
		},
	})
}

// TestAccEdgeFunctionSecretsResource_ReadAPIError verifies that when the list
// secrets endpoint returns an unexpected error status the provider surfaces an
// error diagnostic rather than silently storing empty state.
func TestAccEdgeFunctionSecretsResource_ReadAPIError(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"

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
`, projectRef)

	// Create succeeds...
	gock.New(apiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	// ...but the subsequent read fails with 500
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusInternalServerError).
		BodyString(`{"message":"internal server error"}`)

	// If the provider committed partial state before the error, the framework
	// will attempt a destroy. No state should be committed on a failed Create,
	// but register a mock as a safety net to prevent a spurious teardown error.
	gock.New(apiEndpoint).
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testConfig,
				ExpectError: regexp.MustCompile("API Error"),
			},
		},
	})
}

// TestAccEdgeFunctionSecretsResource_ImportNotFound verifies that importing a
// project_ref that 404s results in a clear "Resource Not Found" error.
// A real first step is needed to provide a config context for the import step.
func TestAccEdgeFunctionSecretsResource_ImportNotFound(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"
	notFoundRef := "nonexistentprojectref"

	apiKeyPlain := "secret-api-key"
	apiKeyDigest := testDigest(apiKeyPlain)

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
`, projectRef, apiKeyPlain)

	// Step 1: create and read the real resource
	gock.New(apiEndpoint).
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{Name: "API_KEY", Value: apiKeyDigest},
		})

	// Step 2: import a different project_ref that returns 404
	gock.New(apiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/secrets", notFoundRef)).
		Reply(http.StatusNotFound)

	// Teardown: delete the real resource
	gock.New(apiEndpoint).
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: create a real resource to provide config context
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", projectRef),
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
