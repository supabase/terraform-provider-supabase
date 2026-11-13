// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math/big"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
	"gopkg.in/h2non/gock.v1"
)

func thirdPartyAuthUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()

	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("failed to parse test UUID: %v", err)
	}
	return parsed
}

func oidcThirdPartyAuthResponse(t *testing.T, issuerURL string) api.ThirdPartyAuth {
	t.Helper()

	return api.ThirdPartyAuth{
		Id:            thirdPartyAuthUUID(t, testThirdPartyAuthUUID),
		Type:          "oidc",
		OidcIssuerUrl: nullable.NewNullableWithValue(issuerURL),
		JwksUrl:       nullable.NewNullNullable[string](),
		CustomJwks:    nullable.NewNullNullable[interface{}](),
		ResolvedJwks: nullable.NewNullableWithValue[interface{}](map[string]any{
			"keys": []any{
				map[string]any{
					"kty": "RSA",
					"kid": "test-key",
					"n":   "abc",
					"e":   "AQAB",
				},
			},
		}),
		InsertedAt: "2026-01-01T00:00:00Z",
		UpdatedAt:  "2026-01-01T00:00:00Z",
		ResolvedAt: nullable.NewNullableWithValue("2026-01-01T00:01:00Z"),
	}
}

func matchJSONBody(t *testing.T, expected map[string]any) func(*http.Request, *gock.Request) (bool, error) {
	t.Helper()

	return func(req *http.Request, _ *gock.Request) (bool, error) {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return false, err
		}

		var actual map[string]any
		if err := json.Unmarshal(bodyBytes, &actual); err != nil {
			return false, err
		}

		if !reflect.DeepEqual(actual, expected) {
			return false, fmt.Errorf("unexpected request body: got %#v, want %#v", actual, expected)
		}

		return true, nil
	}
}

func mockThirdPartyAuthCreateReadiness() {
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "test",
			OrganizationId: "test-org",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON([]api.V1ServiceHealthResponse{
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		})
}

// The API reports every integration as type "custom", whichever source
// (oidc_issuer_url, jwks_url or custom_jwks) was configured.
const thirdPartyAuthAPIType = "custom"

const (
	thirdPartyAuthOIDCIssuer = "https://accounts.google.com"
	thirdPartyAuthJWKSURL    = "https://www.googleapis.com/oauth2/v3/certs"
)

// testAccThirdPartyAuthCustomJWKS returns a public-only JWKS containing a
// freshly generated RSA key, so the API can parse the key material.
func testAccThirdPartyAuthCustomJWKS(t *testing.T) map[string]any {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}
	return map[string]any{
		"keys": []any{
			map[string]any{
				"kty": "RSA",
				"kid": "test-key",
				"use": "sig",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			},
		},
	}
}

func testAccThirdPartyAuthLiveVariables(organizationID, projectName string, extra config.Variables) config.Variables {
	vars := config.Variables{
		"organization_id": config.StringVariable(organizationID),
		"project_name":    config.StringVariable(projectName),
	}
	maps.Copy(vars, extra)
	return vars
}

func testAccThirdPartyAuthImportStateID(projectResource, resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		project, ok := s.RootModule().Resources[projectResource]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", projectResource)
		}
		res, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceName)
		}
		return fmt.Sprintf("%s/%s", project.Primary.ID, res.Primary.ID), nil
	}
}

func TestAccThirdPartyAuthResource_basic(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_third_party_auth.test"
	vars := testAccThirdPartyAuthLiveVariables(organizationID, acctest.RandomName(), config.Variables{
		"oidc_issuer_url": config.StringVariable(thirdPartyAuthOIDCIssuer),
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
				ConfigVariables: vars,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.CompareValuePairs(
						resourceName, tfjsonpath.New("project_ref"),
						projectResourceName, tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("type"), knownvalue.StringExact(thirdPartyAuthAPIType)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("oidc_issuer_url"), knownvalue.StringExact(thirdPartyAuthOIDCIssuer)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("jwks_url"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("custom_jwks"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("resolved_jwks"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("inserted_at"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("updated_at"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("resolved_at"), knownvalue.NotNull()),
				},
			},
			// ImportState testing
			{
				ConfigDirectory:   config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
				ConfigVariables:   vars,
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccThirdPartyAuthImportStateID(projectResourceName, resourceName),
			},
		},
	})
}

func TestAccThirdPartyAuthResource_jwksURL(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_third_party_auth.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
				ConfigVariables: testAccThirdPartyAuthLiveVariables(organizationID, acctest.RandomName(), config.Variables{
					"jwks_url": config.StringVariable(thirdPartyAuthJWKSURL),
				}),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("type"), knownvalue.StringExact(thirdPartyAuthAPIType)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("jwks_url"), knownvalue.StringExact(thirdPartyAuthJWKSURL)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("oidc_issuer_url"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("custom_jwks"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("resolved_jwks"), knownvalue.NotNull()),
				},
			},
		},
	})
}

func TestAccThirdPartyAuthResource_customJWKS(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_third_party_auth.test"
	projectName := acctest.RandomName()

	jwks := testAccThirdPartyAuthCustomJWKS(t)
	pretty, err := json.MarshalIndent(jwks, "", "  ")
	if err != nil {
		t.Fatalf("Failed to encode JWKS: %v", err)
	}
	compact, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("Failed to encode JWKS: %v", err)
	}
	idUnchanged := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
				ConfigVariables: testAccThirdPartyAuthLiveVariables(organizationID, projectName, config.Variables{
					"custom_jwks": config.StringVariable(string(pretty)),
				}),
				ConfigStateChecks: []statecheck.StateCheck{
					idUnchanged.AddStateValue(resourceName, tfjsonpath.New("id")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("type"), knownvalue.StringExact(thirdPartyAuthAPIType)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("custom_jwks"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("oidc_issuer_url"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("jwks_url"), knownvalue.Null()),
					// inline keys are never resolved remotely, so the API returns no resolved JWKS
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("resolved_jwks"), knownvalue.Null()),
				},
			},
			// Re-formatting the same JWKS updates in place (the string differs) but must not replace the resource
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
				ConfigVariables: testAccThirdPartyAuthLiveVariables(organizationID, projectName, config.Variables{
					"custom_jwks": config.StringVariable(string(compact)),
				}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					idUnchanged.AddStateValue(resourceName, tfjsonpath.New("id")),
				},
			},
		},
	})
}

// The integration source cannot be updated in place: changing it replaces the resource.
func TestAccThirdPartyAuthResource_update(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_third_party_auth.test"
	projectName := acctest.RandomName()
	idChanges := statecheck.CompareValue(compare.ValuesDiffer())

	step := func(action plancheck.ResourceActionType, extra config.Variables, checks ...statecheck.StateCheck) resource.TestStep {
		return resource.TestStep{
			ConfigDirectory: config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
			ConfigVariables: testAccThirdPartyAuthLiveVariables(organizationID, projectName, extra),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(resourceName, action),
				},
			},
			ConfigStateChecks: append([]statecheck.StateCheck{
				idChanges.AddStateValue(resourceName, tfjsonpath.New("id")),
			}, checks...),
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			step(plancheck.ResourceActionCreate,
				config.Variables{"oidc_issuer_url": config.StringVariable(thirdPartyAuthOIDCIssuer)},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("type"), knownvalue.StringExact(thirdPartyAuthAPIType)),
			),
			// oidc_issuer_url -> jwks_url
			step(plancheck.ResourceActionDestroyBeforeCreate,
				config.Variables{"jwks_url": config.StringVariable(thirdPartyAuthJWKSURL)},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("type"), knownvalue.StringExact(thirdPartyAuthAPIType)),
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("oidc_issuer_url"), knownvalue.Null()),
			),
			// Changing the URL itself also replaces
			step(plancheck.ResourceActionDestroyBeforeCreate,
				config.Variables{"jwks_url": config.StringVariable("https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com")},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("jwks_url"), knownvalue.StringExact("https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com")),
			),
		},
	})
}

func TestAccThirdPartyAuthResource_ExactlyOneSourceRequired(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref = %q
}
`, testProjectRef),
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured`),
			},
		},
	})
}

func TestAccThirdPartyAuthResource_ExactlyOneSourceAllowed(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref     = %q
  oidc_issuer_url = "https://issuer.example.com"
  jwks_url        = "https://issuer.example.com/.well-known/jwks.json"
}
`, testProjectRef),
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured`),
			},
		},
	})
}

func TestAccThirdPartyAuthResource_CustomJWKSRejectsPrivateMaterial(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref = %q
  custom_jwks = jsonencode({
    keys = [
      {
        kty = "RSA"
        kid = "test-key"
        n   = "abc"
        e   = "AQAB"
        d   = "private"
      }
    ]
  })
}
`, testProjectRef),
				ExpectError: regexp.MustCompile(`private or symmetric JWK member`),
			},
		},
	})
}

func TestBuildThirdPartyAuthCreateBody_CustomJWKSRejectsPrivateMaterial(t *testing.T) {
	data := &ThirdPartyAuthResourceModel{
		ProjectRef: types.StringValue(testProjectRef),
		CustomJWKS: jsontypes.NewNormalizedValue(`{
  "keys": [
    {
      "kty": "RSA",
      "kid": "test-key",
      "n": "abc",
      "e": "AQAB",
      "d": "private"
    }
  ]
}`),
	}

	_, diags := buildThirdPartyAuthCreateBody(data)
	if !diags.HasError() {
		t.Fatalf("expected diagnostics for private custom_jwks")
	}
	if got := diags.Errors()[0].Detail(); !strings.Contains(got, "private or symmetric JWK member") {
		t.Fatalf("expected private JWK diagnostic, got %q", got)
	}
}

func TestBuildThirdPartyAuthCreateBody_RejectsUnknownSources(t *testing.T) {
	testCases := []struct {
		name string
		data ThirdPartyAuthResourceModel
	}{
		{
			name: "oidc_issuer_url",
			data: ThirdPartyAuthResourceModel{
				ProjectRef:    types.StringValue(testProjectRef),
				OIDCIssuerURL: types.StringUnknown(),
				JWKSURL:       types.StringNull(),
				CustomJWKS:    jsontypes.NewNormalizedNull(),
			},
		},
		{
			name: "jwks_url",
			data: ThirdPartyAuthResourceModel{
				ProjectRef:    types.StringValue(testProjectRef),
				OIDCIssuerURL: types.StringNull(),
				JWKSURL:       types.StringUnknown(),
				CustomJWKS:    jsontypes.NewNormalizedNull(),
			},
		},
		{
			name: "custom_jwks",
			data: ThirdPartyAuthResourceModel{
				ProjectRef:    types.StringValue(testProjectRef),
				OIDCIssuerURL: types.StringNull(),
				JWKSURL:       types.StringNull(),
				CustomJWKS:    jsontypes.NewNormalizedUnknown(),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, diags := buildThirdPartyAuthCreateBody(&testCase.data)
			if !diags.HasError() {
				t.Fatalf("expected diagnostics for unknown %s", testCase.name)
			}
			if got := diags.Errors()[0].Summary(); got != "Unknown Third-Party Auth Source" {
				t.Fatalf("expected Unknown Third-Party Auth Source diagnostic, got %q", got)
			}
		})
	}
}

func TestBuildThirdPartyAuthCreateBody_RequiresExactlyOneKnownSource(t *testing.T) {
	testCases := []struct {
		name string
		data ThirdPartyAuthResourceModel
	}{
		{
			name: "zero",
			data: ThirdPartyAuthResourceModel{
				ProjectRef:    types.StringValue(testProjectRef),
				OIDCIssuerURL: types.StringNull(),
				JWKSURL:       types.StringNull(),
				CustomJWKS:    jsontypes.NewNormalizedNull(),
			},
		},
		{
			name: "multiple",
			data: ThirdPartyAuthResourceModel{
				ProjectRef:    types.StringValue(testProjectRef),
				OIDCIssuerURL: types.StringValue("https://issuer.example.com"),
				JWKSURL:       types.StringValue("https://issuer.example.com/.well-known/jwks.json"),
				CustomJWKS:    jsontypes.NewNormalizedNull(),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, diags := buildThirdPartyAuthCreateBody(&testCase.data)
			if !diags.HasError() {
				t.Fatalf("expected diagnostics for %s sources", testCase.name)
			}
			if got := diags.Errors()[0].Summary(); got != "Invalid Third-Party Auth Source" {
				t.Fatalf("expected Invalid Third-Party Auth Source diagnostic, got %q", got)
			}
		})
	}
}

func TestSetThirdPartyAuthState_CustomJWKSRejectsPrivateMaterial(t *testing.T) {
	data := &ThirdPartyAuthResourceModel{
		ProjectRef: types.StringValue(testProjectRef),
	}
	diags := setThirdPartyAuthState(data, api.ThirdPartyAuth{
		Id:   thirdPartyAuthUUID(t, testThirdPartyAuthUUID),
		Type: "custom_jwks",
		CustomJwks: nullable.NewNullableWithValue[interface{}](map[string]any{
			"keys": []any{
				map[string]any{
					"kty": "RSA",
					"kid": "test-key",
					"n":   "abc",
					"e":   "AQAB",
					"d":   "private",
				},
			},
		}),
		ResolvedJwks: nullable.NewNullNullable[interface{}](),
		InsertedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:    "2026-01-01T00:00:00Z",
	})
	if !diags.HasError() {
		t.Fatalf("expected diagnostics for private custom_jwks returned by API")
	}
	if got := diags.Errors()[0].Detail(); !strings.Contains(got, "private or symmetric JWK member") {
		t.Fatalf("expected private JWK diagnostic, got %q", got)
	}
}

func TestSetThirdPartyAuthState_ResolvedJWKSRejectsPrivateMaterial(t *testing.T) {
	data := &ThirdPartyAuthResourceModel{
		ProjectRef: types.StringValue(testProjectRef),
	}
	diags := setThirdPartyAuthState(data, api.ThirdPartyAuth{
		Id:         thirdPartyAuthUUID(t, testThirdPartyAuthUUID),
		Type:       "oidc",
		CustomJwks: nullable.NewNullNullable[interface{}](),
		ResolvedJwks: nullable.NewNullableWithValue[interface{}](map[string]any{
			"keys": []any{
				map[string]any{
					"kty": "RSA",
					"kid": "test-key",
					"n":   "abc",
					"e":   "AQAB",
					"d":   "private",
				},
			},
		}),
		InsertedAt: "2026-01-01T00:00:00Z",
		UpdatedAt:  "2026-01-01T00:00:00Z",
	})
	if !diags.HasError() {
		t.Fatalf("expected diagnostics for private resolved_jwks returned by API")
	}
	if got := diags.Errors()[0].Detail(); !strings.Contains(got, "private or symmetric JWK member") {
		t.Fatalf("expected private JWK diagnostic, got %q", got)
	}
}

func TestAccThirdPartyAuthResource_ReadRemovesDeletedResource(t *testing.T) {
	defer gock.OffAll()

	issuerURL := "https://issuer.example.com"
	response := oidcThirdPartyAuthResponse(t, issuerURL)

	mockThirdPartyAuthCreateReadiness()
	gock.New(defaultApiEndpoint).
		Post(thirdPartyAuthApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"oidc_issuer_url": issuerURL,
		})).
		Reply(http.StatusCreated).
		JSON(response)
	gock.New(defaultApiEndpoint).
		Get(thirdPartyAuthItemApiPath).
		Reply(http.StatusOK).
		JSON(response)
	gock.New(defaultApiEndpoint).
		Get(thirdPartyAuthItemApiPath).
		Reply(http.StatusNotFound).
		JSON(map[string]any{"message": "not found"})
	gock.New(defaultApiEndpoint).
		Delete(thirdPartyAuthItemApiPath).
		Reply(http.StatusNotFound).
		JSON(map[string]any{"message": "not found"})

	config := fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref     = %q
  oidc_issuer_url = %q
}
`, testProjectRef, issuerURL)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "id", testThirdPartyAuthUUID),
				),
			},
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
