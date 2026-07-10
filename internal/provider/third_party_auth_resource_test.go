// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
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

func jwksURLThirdPartyAuthResponse(t *testing.T, id string, jwksURL string) api.ThirdPartyAuth {
	t.Helper()

	return api.ThirdPartyAuth{
		Id:            thirdPartyAuthUUID(t, id),
		Type:          "jwks_url",
		OidcIssuerUrl: nullable.NewNullNullable[string](),
		JwksUrl:       nullable.NewNullableWithValue(jwksURL),
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

func customJWKSThirdPartyAuthResponse(t *testing.T, customJWKS map[string]any) api.ThirdPartyAuth {
	t.Helper()

	return api.ThirdPartyAuth{
		Id:            thirdPartyAuthUUID(t, testThirdPartyAuthUUID),
		Type:          "custom_jwks",
		OidcIssuerUrl: nullable.NewNullNullable[string](),
		JwksUrl:       nullable.NewNullNullable[string](),
		CustomJwks:    nullable.NewNullableWithValue[interface{}](customJWKS),
		ResolvedJwks:  nullable.NewNullableWithValue[interface{}](customJWKS),
		InsertedAt:    "2026-01-01T00:00:00Z",
		UpdatedAt:     "2026-01-01T00:00:00Z",
		ResolvedAt:    nullable.NewNullableWithValue("2026-01-01T00:01:00Z"),
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

func TestAccThirdPartyAuthResource_OIDC(t *testing.T) {
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
		Delete(thirdPartyAuthItemApiPath).
		Reply(http.StatusOK).
		JSON(response)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref     = %q
  oidc_issuer_url = %q
}
`, testProjectRef, issuerURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "id", testThirdPartyAuthUUID),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "type", "oidc"),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "oidc_issuer_url", issuerURL),
					resource.TestCheckResourceAttrSet("supabase_third_party_auth.test", "resolved_jwks"),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "inserted_at", "2026-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "updated_at", "2026-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "resolved_at", "2026-01-01T00:01:00Z"),
				),
			},
		},
	})
}

func TestAccThirdPartyAuthResource_CustomJWKS(t *testing.T) {
	defer gock.OffAll()

	customJWKS := map[string]any{
		"keys": []any{
			map[string]any{
				"kty": "RSA",
				"kid": "test-key",
				"n":   "abc",
				"e":   "AQAB",
			},
		},
	}
	response := customJWKSThirdPartyAuthResponse(t, customJWKS)

	mockThirdPartyAuthCreateReadiness()
	gock.New(defaultApiEndpoint).
		Post(thirdPartyAuthApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"custom_jwks": customJWKS,
		})).
		Reply(http.StatusCreated).
		JSON(response)
	gock.New(defaultApiEndpoint).
		Get(thirdPartyAuthItemApiPath).
		Reply(http.StatusOK).
		JSON(response)
	gock.New(defaultApiEndpoint).
		Delete(thirdPartyAuthItemApiPath).
		Reply(http.StatusOK).
		JSON(response)

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
      }
    ]
  })
}
`, testProjectRef),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "id", testThirdPartyAuthUUID),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "type", "custom_jwks"),
					resource.TestCheckResourceAttrSet("supabase_third_party_auth.test", "custom_jwks"),
					resource.TestCheckResourceAttrSet("supabase_third_party_auth.test", "resolved_jwks"),
				),
			},
		},
	})
}

func TestAccThirdPartyAuthResource_CustomJWKSFormattingDoesNotReplace(t *testing.T) {
	defer gock.OffAll()

	customJWKS := map[string]any{
		"keys": []any{
			map[string]any{
				"kty": "RSA",
				"kid": "test-key",
				"n":   "abc",
				"e":   "AQAB",
			},
		},
	}
	response := customJWKSThirdPartyAuthResponse(t, customJWKS)

	mockThirdPartyAuthCreateReadiness()
	gock.New(defaultApiEndpoint).
		Post(thirdPartyAuthApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"custom_jwks": customJWKS,
		})).
		Reply(http.StatusCreated).
		JSON(response)
	gock.New(defaultApiEndpoint).
		Get(thirdPartyAuthItemApiPath).
		Times(3).
		Reply(http.StatusOK).
		JSON(response)
	gock.New(defaultApiEndpoint).
		Delete(thirdPartyAuthItemApiPath).
		Reply(http.StatusOK).
		JSON(response)

	config := func(customJWKS string) string {
		return fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref = %q
  custom_jwks = %q
}
`, testProjectRef, customJWKS)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(`{
  "keys": [
    {
      "kty": "RSA",
      "kid": "test-key",
      "n": "abc",
      "e": "AQAB"
    }
  ]
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "id", testThirdPartyAuthUUID),
				),
			},
			{
				Config: config(`{"keys":[{"e":"AQAB","n":"abc","kid":"test-key","kty":"RSA"}]}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "id", testThirdPartyAuthUUID),
				),
			},
		},
	})
}

func TestAccThirdPartyAuthResource_Import(t *testing.T) {
	defer gock.OffAll()

	issuerURL := "https://issuer.example.com"
	response := oidcThirdPartyAuthResponse(t, issuerURL)

	gock.New(defaultApiEndpoint).
		Get(thirdPartyAuthItemApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON(response)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref     = %q
  oidc_issuer_url = %q
}
`, testProjectRef, issuerURL),
				ResourceName:  "supabase_third_party_auth.test",
				ImportState:   true,
				ImportStateId: fmt.Sprintf("%s/%s", testProjectRef, testThirdPartyAuthUUID),
				ImportStateCheck: func(s []*terraform.InstanceState) error {
					if len(s) != 1 {
						return fmt.Errorf("expected 1 instance state, got %d", len(s))
					}
					state := s[0]
					if state.Attributes["project_ref"] != testProjectRef {
						return fmt.Errorf("expected project_ref %q, got %q", testProjectRef, state.Attributes["project_ref"])
					}
					if state.Attributes["id"] != testThirdPartyAuthUUID {
						return fmt.Errorf("expected id %q, got %q", testThirdPartyAuthUUID, state.Attributes["id"])
					}
					if state.Attributes["oidc_issuer_url"] != issuerURL {
						return fmt.Errorf("expected oidc_issuer_url %q, got %q", issuerURL, state.Attributes["oidc_issuer_url"])
					}
					return nil
				},
			},
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

func TestAccThirdPartyAuthResource_ReplaceOnSourceChange(t *testing.T) {
	defer gock.OffAll()

	replacementID := "99999999-9999-4999-9999-999999999999"
	issuerURL := "https://issuer.example.com"
	jwksURL := "https://issuer.example.com/.well-known/jwks.json"
	initialResponse := oidcThirdPartyAuthResponse(t, issuerURL)
	replacementResponse := jwksURLThirdPartyAuthResponse(t, replacementID, jwksURL)
	replacementPath := fmt.Sprintf("%s/%s", thirdPartyAuthApiPath, replacementID)

	mockThirdPartyAuthCreateReadiness()
	gock.New(defaultApiEndpoint).
		Post(thirdPartyAuthApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"oidc_issuer_url": issuerURL,
		})).
		Reply(http.StatusCreated).
		JSON(initialResponse)
	gock.New(defaultApiEndpoint).
		Get(thirdPartyAuthItemApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON(initialResponse)
	gock.New(defaultApiEndpoint).
		Delete(thirdPartyAuthItemApiPath).
		Reply(http.StatusOK).
		JSON(initialResponse)
	mockThirdPartyAuthCreateReadiness()
	gock.New(defaultApiEndpoint).
		Post(thirdPartyAuthApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"jwks_url": jwksURL,
		})).
		Reply(http.StatusCreated).
		JSON(replacementResponse)
	gock.New(defaultApiEndpoint).
		Get(replacementPath).
		Reply(http.StatusOK).
		JSON(replacementResponse)
	gock.New(defaultApiEndpoint).
		Delete(replacementPath).
		Reply(http.StatusOK).
		JSON(replacementResponse)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref     = %q
  oidc_issuer_url = %q
}
`, testProjectRef, issuerURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "id", testThirdPartyAuthUUID),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "oidc_issuer_url", issuerURL),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref = %q
  jwks_url    = %q
}
`, testProjectRef, jwksURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "id", replacementID),
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "jwks_url", jwksURL),
				),
			},
		},
	})
}

func TestAccThirdPartyAuthResource_UpdateTimeoutOnly(t *testing.T) {
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
		Times(3).
		Reply(http.StatusOK).
		JSON(response)
	gock.New(defaultApiEndpoint).
		Delete(thirdPartyAuthItemApiPath).
		Reply(http.StatusOK).
		JSON(response)

	configWithTimeout := func(createTimeout string) string {
		return fmt.Sprintf(`
resource "supabase_third_party_auth" "test" {
  project_ref     = %q
  oidc_issuer_url = %q

  timeouts {
    create = %q
  }
}
`, testProjectRef, issuerURL, createTimeout)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configWithTimeout("5m"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "id", testThirdPartyAuthUUID),
				),
			},
			{
				Config: configWithTimeout("10m"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_third_party_auth.test", "id", testThirdPartyAuthUUID),
				),
			},
		},
	})
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
