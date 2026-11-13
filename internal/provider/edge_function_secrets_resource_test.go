package provider

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/config"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
	"gopkg.in/h2non/gock.v1"
)

type testAccSecret struct{ name, value string }

func testAccEdgeFunctionSecretsLiveVariables(organizationID, projectName string, secrets ...testAccSecret) config.Variables {
	items := make([]config.Variable, 0, len(secrets))
	for _, secret := range secrets {
		items = append(items, config.ObjectVariable(map[string]config.Variable{
			"name":  config.StringVariable(secret.name),
			"value": config.StringVariable(secret.value),
		}))
	}
	return config.Variables{
		"organization_id": config.StringVariable(organizationID),
		"project_name":    config.StringVariable(projectName),
		"secrets":         config.ListVariable(items...),
	}
}

// testAccSecretChecks asserts the secrets set and the digests computed from it.
func testAccSecretChecks(resourceName string, secrets ...testAccSecret) []statecheck.StateCheck {
	setChecks := make([]knownvalue.Check, 0, len(secrets))
	digests := make(map[string]knownvalue.Check, len(secrets))
	for _, secret := range secrets {
		setChecks = append(setChecks, knownvalue.ObjectExact(map[string]knownvalue.Check{
			"name":  knownvalue.StringExact(secret.name),
			"value": knownvalue.StringExact(secret.value),
		}))
		digests[secret.name] = knownvalue.StringExact(computeSecretDigest(secret.value))
	}
	return []statecheck.StateCheck{
		statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("secrets"), knownvalue.SetExact(setChecks)),
		statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("secret_digests"), knownvalue.MapExact(digests)),
	}
}

func testAccEdgeFunctionSecretsImportStateID(projectResource string) testresource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		project, ok := s.RootModule().Resources[projectResource]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", projectResource)
		}
		return project.Primary.ID, nil
	}
}

func TestAccEdgeFunctionSecretsResource_basic(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_edge_function_secrets.test"
	projectName := acctest.RandomName()
	secrets := []testAccSecret{
		{"API_KEY", "secret-api-key-123"},
		{"DATABASE_URL", "postgresql://user:pass@localhost:5432/db"},
	}

	testresource.Test(t, testresource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []testresource.TestStep{
			// Create and Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunctionSecrets/basic/"),
				ConfigVariables: testAccEdgeFunctionSecretsLiveVariables(organizationID, projectName, secrets...),
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: append([]statecheck.StateCheck{
					statecheck.CompareValuePairs(
						resourceName, tfjsonpath.New("id"),
						projectResourceName, tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					statecheck.CompareValuePairs(
						resourceName, tfjsonpath.New("project_ref"),
						projectResourceName, tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					// Exact digests also prove reserved SUPABASE_ secrets are filtered out of state
				}, testAccSecretChecks(resourceName, secrets...)...),
			},
			// ImportState testing (the API only returns digests, so values are not verified)
			{
				ConfigDirectory:         config.StaticDirectory("test-fixtures/EdgeFunctionSecrets/basic/"),
				ConfigVariables:         testAccEdgeFunctionSecretsLiveVariables(organizationID, projectName, secrets...),
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       testAccEdgeFunctionSecretsImportStateID(projectResourceName),
				ImportStateVerifyIgnore: []string{"secrets"},
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_update(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_edge_function_secrets.test"
	projectName := acctest.RandomName()

	step := func(action plancheck.ResourceActionType, secrets ...testAccSecret) testresource.TestStep {
		return testresource.TestStep{
			ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunctionSecrets/basic/"),
			ConfigVariables: testAccEdgeFunctionSecretsLiveVariables(organizationID, projectName, secrets...),
			ConfigPlanChecks: testresource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(resourceName, action),
				},
			},
			ConfigStateChecks: testAccSecretChecks(resourceName, secrets...),
		}
	}

	testresource.Test(t, testresource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []testresource.TestStep{
			step(plancheck.ResourceActionCreate,
				testAccSecret{"API_KEY", "value-1"}, testAccSecret{"OTHER_KEY", "value-1"}),
			// Change a secret value
			step(plancheck.ResourceActionUpdate,
				testAccSecret{"API_KEY", "value-2"}, testAccSecret{"OTHER_KEY", "value-1"}),
			// Add a secret
			step(plancheck.ResourceActionUpdate,
				testAccSecret{"API_KEY", "value-2"}, testAccSecret{"OTHER_KEY", "value-1"}, testAccSecret{"THIRD_KEY", "value-1"}),
			// Remove a secret
			step(plancheck.ResourceActionUpdate,
				testAccSecret{"API_KEY", "value-2"}, testAccSecret{"THIRD_KEY", "value-1"}),
		},
	})
}

func TestAccEdgeFunctionSecretsResource_empty(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_edge_function_secrets.test"

	testresource.Test(t, testresource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []testresource.TestStep{
			// An explicit empty set is valid and must not import or manage reserved secrets
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunctionSecrets/basic/"),
				ConfigVariables: testAccEdgeFunctionSecretsLiveVariables(organizationID, acctest.RandomName()),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("secrets"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("secret_digests"), knownvalue.MapSizeExact(0)),
				},
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_reservedPrefixCreate(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	testresource.Test(t, testresource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []testresource.TestStep{
			// The API rejects SUPABASE_-prefixed names and the provider surfaces the error
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunctionSecrets/basic/"),
				ConfigVariables: testAccEdgeFunctionSecretsLiveVariables(organizationID, acctest.RandomName(),
					testAccSecret{"API_KEY", "valid-secret"},
					testAccSecret{"SUPABASE_URL", "https://example.supabase.co"},
				),
				ExpectError: regexp.MustCompile(`Unable to create/update edge function secrets`),
			},
		},
	})
}

func TestAccEdgeFunctionSecretsResource_reservedPrefixUpdate(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectName := acctest.RandomName()

	testresource.Test(t, testresource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []testresource.TestStep{
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunctionSecrets/basic/"),
				ConfigVariables: testAccEdgeFunctionSecretsLiveVariables(organizationID, projectName, testAccSecret{"API_KEY", "valid-secret"}),
			},
			// Adding a reserved name to an existing resource is rejected too
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunctionSecrets/basic/"),
				ConfigVariables: testAccEdgeFunctionSecretsLiveVariables(organizationID, projectName,
					testAccSecret{"API_KEY", "valid-secret"},
					testAccSecret{"SUPABASE_URL", "https://example.supabase.co"},
				),
				ExpectError: regexp.MustCompile(`Unable to create/update edge function secrets`),
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
