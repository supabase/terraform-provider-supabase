package provider

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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
	"github.com/supabase/terraform-provider-supabase/examples"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
)

// The API derives the type from the configured URL. The live tests use Google's
// issuer, which isn't one of the providers Supabase recognises, so they see "custom".
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
	var projectRef, integrationID string

	resource.ParallelTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create the integration and check every attribute.
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
				ConfigVariables: vars,
				Check: func(s *terraform.State) error {
					rs, ok := s.RootModule().Resources[resourceName]
					if !ok {
						return fmt.Errorf("resource %s not found in state", resourceName)
					}
					projectRef, integrationID = rs.Primary.Attributes["project_ref"], rs.Primary.ID
					return nil
				},
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
			// Import it and check the imported state matches.
			{
				ConfigDirectory:   config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
				ConfigVariables:   vars,
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccThirdPartyAuthImportStateID(projectResourceName, resourceName),
			},
			// Importing an integration that doesn't exist fails with a clear error.
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
				ConfigVariables: vars,
				ResourceName:    resourceName,
				ImportState:     true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					project, ok := s.RootModule().Resources[projectResourceName]
					if !ok {
						return "", fmt.Errorf("resource %s not found in state", projectResourceName)
					}
					return project.Primary.ID + "/" + uuid.NewString(), nil
				},
				ExpectError: regexp.MustCompile(`Cannot import non-existent remote object`),
			},
			// After the integration is deleted outside Terraform, Read removes it from state, so Terraform plans to
			// create it again. The final destroy then deletes an integration that is already gone, which covers
			// Delete treating a 404 as success.
			{
				PreConfig: func() {
					client, err := sweeperClient()
					if err != nil {
						t.Fatal(err)
					}
					resp, err := client.V1DeleteProjectTpaIntegrationWithResponse(t.Context(), projectRef, uuid.MustParse(integrationID))
					if err != nil {
						t.Fatalf("deleting the integration outside Terraform: %v", err)
					}
					if resp.StatusCode() != http.StatusOK {
						t.Fatalf("deleting the integration outside Terraform: got status %d: %s", resp.StatusCode(), resp.Body)
					}
				},
				ConfigDirectory: config.StaticDirectory("test-fixtures/ThirdPartyAuth/basic/"),
				ConfigVariables: vars,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
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

	resource.ParallelTest(t, resource.TestCase{
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

	resource.ParallelTest(t, resource.TestCase{
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
					// Inline keys are never fetched from a URL, so the API returns no resolved JWKS.
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("resolved_jwks"), knownvalue.Null()),
				},
			},
			// Reformatting the same JWKS changes the string, so it updates in place, but it must not replace the resource.
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

	resource.ParallelTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			step(plancheck.ResourceActionCreate,
				config.Variables{"oidc_issuer_url": config.StringVariable(thirdPartyAuthOIDCIssuer)},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("type"), knownvalue.StringExact(thirdPartyAuthAPIType)),
			),
			// Switching from oidc_issuer_url to jwks_url replaces the integration.
			step(plancheck.ResourceActionDestroyBeforeCreate,
				config.Variables{"jwks_url": config.StringVariable(thirdPartyAuthJWKSURL)},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("type"), knownvalue.StringExact(thirdPartyAuthAPIType)),
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("oidc_issuer_url"), knownvalue.Null()),
			),
			// Changing the URL itself also replaces it.
			step(plancheck.ResourceActionDestroyBeforeCreate,
				config.Variables{"jwks_url": config.StringVariable("https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com")},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("jwks_url"), knownvalue.StringExact("https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com")),
			),
		},
	})
}

// Plans the embedded documentation examples, so a broken example fails the tests
// before it reaches the docs. Planning new resources makes no API calls, and the
// examples' project ref is a placeholder, so this is the one plan-only test that
// doesn't run against the live API.
func TestAccThirdPartyAuthResource_example(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			{
				Config: offlineProviderConfig + strings.Join([]string{
					examples.ThirdPartyAuthResourceConfig,
					examples.ThirdPartyAuthFirebaseConfig,
					examples.ThirdPartyAuthAuth0Config,
					examples.ThirdPartyAuthAWSCognitoConfig,
					examples.ThirdPartyAuthClerkConfig,
					examples.ThirdPartyAuthWorkOSConfig,
				}, "\n"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

const (
	testPublicJWKS        = `{"keys":[{"kty":"RSA","kid":"k","n":"abc","e":"AQAB"}]}`
	testPublicJWKSPretty  = "{\n  \"keys\": [\n    {\"kty\": \"RSA\", \"kid\": \"k\", \"n\": \"abc\", \"e\": \"AQAB\"}\n  ]\n}"
	testPrivateJWKS       = `{"keys":[{"kty":"RSA","kid":"k","n":"abc","e":"AQAB","d":"private"}]}`
	testOtherPublicJWKS   = `{"keys":[{"kty":"RSA","kid":"other","n":"def","e":"AQAB"}]}`
	testOIDCIssuerExample = "https://issuer.example.com"
)

func TestThirdPartyAuthResource_ConfigValidatorsRequireExactlyOneSource(t *testing.T) {
	t.Parallel()
	s := testResourceSchema(t, NewThirdPartyAuthResource())

	testCases := map[string]struct {
		attrs   map[string]string
		wantErr bool
	}{
		"none":     {attrs: map[string]string{"project_ref": testProjectRef}, wantErr: true},
		"one":      {attrs: map[string]string{"project_ref": testProjectRef, "oidc_issuer_url": testOIDCIssuerExample}},
		"multiple": {attrs: map[string]string{"project_ref": testProjectRef, "oidc_issuer_url": testOIDCIssuerExample, "jwks_url": testOIDCIssuerExample + "/jwks.json"}, wantErr: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := fwresource.ValidateConfigRequest{Config: tfsdk.Config{Schema: s, Raw: testObjectValue(t, s, tc.attrs)}}
			resp := &fwresource.ValidateConfigResponse{}
			for _, v := range (&ThirdPartyAuthResource{}).ConfigValidators(t.Context()) {
				v.ValidateResource(t.Context(), req, resp)
			}
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestThirdPartyAuthResource_CustomJWKSUsesPublicJWKSValidator(t *testing.T) {
	t.Parallel()
	attribute, ok := testResourceSchema(t, NewThirdPartyAuthResource()).Attributes["custom_jwks"].(schema.StringAttribute)
	if !ok {
		t.Fatal("custom_jwks is not a string attribute")
	}
	for _, v := range attribute.Validators {
		if _, ok := v.(publicJWKSValidator); ok {
			return
		}
	}
	t.Fatal("custom_jwks does not use publicJWKSValidator")
}

func TestThirdPartyAuthResource_URLAttributesAreValidated(t *testing.T) {
	t.Parallel()
	s := testResourceSchema(t, NewThirdPartyAuthResource())

	testCases := map[string]struct {
		attribute string
		value     string
		wantErr   bool
	}{
		"issuer https":           {attribute: "oidc_issuer_url", value: "https://login.example.com"},
		"issuer http":            {attribute: "oidc_issuer_url", value: "http://login.example.com", wantErr: true},
		"issuer without scheme":  {attribute: "oidc_issuer_url", value: "login.example.com", wantErr: true},
		"jwks_url https":         {attribute: "jwks_url", value: "https://login.example.com/.well-known/jwks.json"},
		"jwks_url http":          {attribute: "jwks_url", value: "http://login.example.com/.well-known/jwks.json"},
		"jwks_url other scheme":  {attribute: "jwks_url", value: "ftp://login.example.com/jwks.json", wantErr: true},
		"jwks_url without host":  {attribute: "jwks_url", value: "https:///jwks.json", wantErr: true},
		"jwks_url not a URL":     {attribute: "jwks_url", value: "not a url", wantErr: true},
		"jwks_url relative path": {attribute: "jwks_url", value: "/.well-known/jwks.json", wantErr: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			attribute, ok := s.Attributes[tc.attribute].(schema.StringAttribute)
			if !ok {
				t.Fatalf("%s is not a string attribute", tc.attribute)
			}
			req := validator.StringRequest{Path: path.Root(tc.attribute), ConfigValue: types.StringValue(tc.value)}
			resp := &validator.StringResponse{}
			for _, v := range attribute.Validators {
				v.ValidateString(t.Context(), req, resp)
			}
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestPublicJWKSValidator(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		value   types.String
		wantErr string
	}{
		"null":           {value: types.StringNull()},
		"unknown":        {value: types.StringUnknown()},
		"public key":     {value: types.StringValue(testPublicJWKS)},
		"private member": {value: types.StringValue(testPrivateJWKS), wantErr: `private or symmetric JWK member "d"`},
		"symmetric key":  {value: types.StringValue(`{"keys":[{"kty":"oct","k":"secret"}]}`), wantErr: `private or symmetric JWK member "k"`},
		"empty keys":     {value: types.StringValue(`{"keys":[]}`), wantErr: "at least one public JWK"},
		"key not object": {value: types.StringValue(`{"keys":["nope"]}`), wantErr: "must be an object"},
		"no keys":        {value: types.StringValue(`{}`), wantErr: "keys array"},
		"not JSON":       {value: types.StringValue(`nope`), wantErr: "valid JSON"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := &validator.StringResponse{}
			publicJWKSValidator{}.ValidateString(t.Context(), validator.StringRequest{ConfigValue: tc.value}, resp)
			if tc.wantErr == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected error: %v", resp.Diagnostics)
				}
				return
			}
			if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), tc.wantErr) {
				t.Fatalf("expected an error containing %q, got %v", tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestThirdPartyAuthResource_CustomJWKSRequiresReplaceOnlyWhenJSONChanges(t *testing.T) {
	t.Parallel()
	s := testResourceSchema(t, NewThirdPartyAuthResource())
	attribute, ok := s.Attributes["custom_jwks"].(schema.StringAttribute)
	if !ok {
		t.Fatal("custom_jwks is not a string attribute")
	}

	testCases := map[string]struct {
		state, plan string
		want        bool
	}{
		"unchanged":       {state: testPublicJWKS, plan: testPublicJWKS},
		"formatting only": {state: testPublicJWKS, plan: testPublicJWKSPretty},
		"different keys":  {state: testPublicJWKS, plan: testOtherPublicJWKS, want: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := planmodifier.StringRequest{
				Path:       path.Root("custom_jwks"),
				StateValue: types.StringValue(tc.state),
				PlanValue:  types.StringValue(tc.plan),
				State:      tfsdk.State{Schema: s, Raw: testObjectValue(t, s, map[string]string{"custom_jwks": tc.state})},
				Plan:       tfsdk.Plan{Schema: s, Raw: testObjectValue(t, s, map[string]string{"custom_jwks": tc.plan})},
			}
			resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
			for _, m := range attribute.PlanModifiers {
				m.PlanModifyString(t.Context(), req, resp)
			}
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error: %v", resp.Diagnostics)
			}
			if resp.RequiresReplace != tc.want {
				t.Fatalf("RequiresReplace = %v, want %v", resp.RequiresReplace, tc.want)
			}
		})
	}
}

func TestThirdPartyAuthResource_UpdateOnlyAcceptsFormattingChanges(t *testing.T) {
	t.Parallel()
	s := testResourceSchema(t, NewThirdPartyAuthResource())

	testCases := map[string]struct {
		planJWKS string
		wantErr  bool
	}{
		"formatting only": {planJWKS: testPublicJWKSPretty},
		"different keys":  {planJWKS: testOtherPublicJWKS, wantErr: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := map[string]string{"project_ref": testProjectRef, "id": testThirdPartyAuthUUID}
			withJWKS := func(jwks string) map[string]string {
				attrs := maps.Clone(base)
				attrs["custom_jwks"] = jwks
				return attrs
			}
			stateRaw := testObjectValue(t, s, withJWKS(testPublicJWKS))
			req := fwresource.UpdateRequest{
				State: tfsdk.State{Schema: s, Raw: stateRaw},
				Plan:  tfsdk.Plan{Schema: s, Raw: testObjectValue(t, s, withJWKS(tc.planJWKS))},
			}
			resp := &fwresource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: stateRaw}}

			// A nil client proves Update never calls the API.
			(&ThirdPartyAuthResource{}).Update(t.Context(), req, resp)

			if tc.wantErr {
				if !resp.Diagnostics.HasError() || resp.Diagnostics.Errors()[0].Summary() != "Update Not Supported" {
					t.Fatalf("expected Update Not Supported, got %v", resp.Diagnostics)
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error: %v", resp.Diagnostics)
			}
			var got jsontypes.Normalized
			resp.Diagnostics.Append(resp.State.GetAttribute(t.Context(), path.Root("custom_jwks"), &got)...)
			if resp.Diagnostics.HasError() {
				t.Fatalf("reading state: %v", resp.Diagnostics)
			}
			if got.ValueString() != tc.planJWKS {
				t.Fatalf("state custom_jwks = %q, want the planned value %q", got.ValueString(), tc.planJWKS)
			}
		})
	}
}

func TestSameJSONValue(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		left, right types.String
		want        bool
		wantErr     bool
	}{
		"identical":        {left: types.StringValue(`{"a":1}`), right: types.StringValue(`{"a":1}`), want: true},
		"formatting only":  {left: types.StringValue(`{"a": 1, "b": [1, 2]}`), right: types.StringValue(`{"b":[1,2],"a":1}`), want: true},
		"different values": {left: types.StringValue(`{"a":1}`), right: types.StringValue(`{"a":2}`)},
		"both null":        {left: types.StringNull(), right: types.StringNull(), want: true},
		"null and value":   {left: types.StringNull(), right: types.StringValue(`{}`)},
		"unknown":          {left: types.StringUnknown(), right: types.StringValue(`{}`)},
		"invalid JSON":     {left: types.StringValue(`{`), right: types.StringValue(`{}`), wantErr: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := sameJSONValue(tc.left, tc.right)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("sameJSONValue() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestThirdPartyAuthResource_ImportState(t *testing.T) {
	t.Parallel()
	s := testResourceSchema(t, NewThirdPartyAuthResource())

	testCases := map[string]struct {
		id      string
		wantErr bool
	}{
		"valid":             {id: testProjectRef + "/" + testThirdPartyAuthUUID},
		"valid with spaces": {id: " " + testProjectRef + " / " + testThirdPartyAuthUUID + " "},
		"empty":             {id: "", wantErr: true},
		"no slash":          {id: testProjectRef, wantErr: true},
		"empty ID":          {id: testProjectRef + "/", wantErr: true},
		"empty project ref": {id: "/" + testThirdPartyAuthUUID, wantErr: true},
		"blank project ref": {id: "  /" + testThirdPartyAuthUUID, wantErr: true},
		"ID not a UUID":     {id: testProjectRef + "/not-a-uuid", wantErr: true},
		"extra segment":     {id: testProjectRef + "/" + testThirdPartyAuthUUID + "/extra", wantErr: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := &fwresource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: testObjectValue(t, s, nil)}}
			// A nil client proves ImportState never calls the API: Terraform calls Read afterwards.
			(&ThirdPartyAuthResource{}).ImportState(t.Context(), fwresource.ImportStateRequest{ID: tc.id}, resp)

			if tc.wantErr {
				if !resp.Diagnostics.HasError() || resp.Diagnostics.Errors()[0].Summary() != "Invalid Import ID" {
					t.Fatalf("expected Invalid Import ID for %q, got %v", tc.id, resp.Diagnostics)
				}
				return
			}
			var id, projectRef types.String
			resp.Diagnostics.Append(resp.State.GetAttribute(t.Context(), path.Root("id"), &id)...)
			resp.Diagnostics.Append(resp.State.GetAttribute(t.Context(), path.Root("project_ref"), &projectRef)...)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error: %v", resp.Diagnostics)
			}
			if id.ValueString() != testThirdPartyAuthUUID || projectRef.ValueString() != testProjectRef {
				t.Fatalf("state id = %q, project_ref = %q; want %q and %q", id.ValueString(), projectRef.ValueString(), testThirdPartyAuthUUID, testProjectRef)
			}
		})
	}
}

func TestBuildThirdPartyAuthCreateBody(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		data    ThirdPartyAuthResourceModel
		want    api.V1CreateProjectTpaIntegrationJSONRequestBody
		wantErr bool
	}{
		"oidc_issuer_url": {
			data: ThirdPartyAuthResourceModel{OIDCIssuerURL: types.StringValue(testOIDCIssuerExample), JWKSURL: types.StringNull(), CustomJWKS: jsontypes.NewNormalizedNull()},
			want: api.V1CreateProjectTpaIntegrationJSONRequestBody{OidcIssuerUrl: Ptr(testOIDCIssuerExample)},
		},
		"jwks_url": {
			data: ThirdPartyAuthResourceModel{OIDCIssuerURL: types.StringNull(), JWKSURL: types.StringValue(testOIDCIssuerExample + "/jwks.json"), CustomJWKS: jsontypes.NewNormalizedNull()},
			want: api.V1CreateProjectTpaIntegrationJSONRequestBody{JwksUrl: Ptr(testOIDCIssuerExample + "/jwks.json")},
		},
		"custom_jwks": {
			data: ThirdPartyAuthResourceModel{OIDCIssuerURL: types.StringNull(), JWKSURL: types.StringNull(), CustomJWKS: jsontypes.NewNormalizedValue(testPublicJWKS)},
			want: func() api.V1CreateProjectTpaIntegrationJSONRequestBody {
				var jwks any
				if err := json.Unmarshal([]byte(testPublicJWKS), &jwks); err != nil {
					panic(err)
				}
				return api.V1CreateProjectTpaIntegrationJSONRequestBody{CustomJwks: &jwks}
			}(),
		},
		"invalid custom_jwks": {
			data:    ThirdPartyAuthResourceModel{OIDCIssuerURL: types.StringNull(), JWKSURL: types.StringNull(), CustomJWKS: jsontypes.NewNormalizedValue(`{`)},
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := buildThirdPartyAuthCreateBody(&tc.data)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("body = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestSetThirdPartyAuthState(t *testing.T) {
	t.Parallel()

	jwks := func(raw string) nullable.Nullable[any] {
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			panic(err)
		}
		return nullable.NewNullableWithValue(v)
	}

	testCases := map[string]struct {
		custom, resolved         nullable.Nullable[any]
		wantCustom, wantResolved jsontypes.Normalized
	}{
		"public keys are stored": {
			custom:       nullable.NewNullNullable[any](),
			resolved:     jwks(testPublicJWKS),
			wantCustom:   jsontypes.NewNormalizedNull(),
			wantResolved: jsontypes.NewNormalizedValue(testPublicJWKS),
		},
		"private custom_jwks is dropped": {
			custom:       jwks(testPrivateJWKS),
			resolved:     nullable.NewNullNullable[any](),
			wantCustom:   jsontypes.NewNormalizedNull(),
			wantResolved: jsontypes.NewNormalizedNull(),
		},
		"private resolved_jwks is dropped": {
			custom:       nullable.NewNullNullable[any](),
			resolved:     jwks(testPrivateJWKS),
			wantCustom:   jsontypes.NewNormalizedNull(),
			wantResolved: jsontypes.NewNormalizedNull(),
		},
		// custom_jwks isn't validated by the API, so private material is dropped even when the document isn't a well-formed JWKS.
		"a bare private JWK is dropped": {
			custom:       jwks(`{"kty":"RSA","kid":"k","n":"abc","e":"AQAB","d":"private"}`),
			resolved:     nullable.NewNullNullable[any](),
			wantCustom:   jsontypes.NewNormalizedNull(),
			wantResolved: jsontypes.NewNormalizedNull(),
		},
		"a private key after a non-object key is dropped": {
			custom:       jwks(`{"keys":["not a key",{"kty":"oct","k":"secret"}]}`),
			resolved:     nullable.NewNullNullable[any](),
			wantCustom:   jsontypes.NewNormalizedNull(),
			wantResolved: jsontypes.NewNormalizedNull(),
		},
		"a private key in a keys object is dropped": {
			custom:       jwks(`{"keys":{"kty":"RSA","d":"private"}}`),
			resolved:     nullable.NewNullNullable[any](),
			wantCustom:   jsontypes.NewNormalizedNull(),
			wantResolved: jsontypes.NewNormalizedNull(),
		},
		"a private key under a capitalised Keys member is dropped": {
			custom:       jwks(`{"Keys":[{"kty":"RSA","d":"private"}]}`),
			resolved:     nullable.NewNullNullable[any](),
			wantCustom:   jsontypes.NewNormalizedNull(),
			wantResolved: jsontypes.NewNormalizedNull(),
		},
		"a public document that isn't a JWKS is stored as returned": {
			custom:       jwks(`{"Keys":[{"kty":"RSA","kid":"k","n":"abc","e":"AQAB"}]}`),
			resolved:     nullable.NewNullNullable[any](),
			wantCustom:   jsontypes.NewNormalizedValue(`{"Keys":[{"kty":"RSA","kid":"k","n":"abc","e":"AQAB"}]}`),
			wantResolved: jsontypes.NewNormalizedNull(),
		},
		"a provider with no keys is stored as returned": {
			custom:       nullable.NewNullNullable[any](),
			resolved:     jwks(`{"keys":[]}`),
			wantCustom:   jsontypes.NewNormalizedNull(),
			wantResolved: jsontypes.NewNormalizedValue(`{"keys":[]}`),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var data ThirdPartyAuthResourceModel
			err := setThirdPartyAuthState(t.Context(), &data, api.ThirdPartyAuth{
				Id:            uuid.MustParse(testThirdPartyAuthUUID),
				Type:          thirdPartyAuthAPIType,
				OidcIssuerUrl: nullable.NewNullableWithValue(testOIDCIssuerExample),
				CustomJwks:    tc.custom,
				ResolvedJwks:  tc.resolved,
				InsertedAt:    "2026-01-01T00:00:00Z",
				UpdatedAt:     "2026-01-01T00:00:00Z",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if data.ID.ValueString() != testThirdPartyAuthUUID {
				t.Fatalf("ID = %q, want %q", data.ID.ValueString(), testThirdPartyAuthUUID)
			}
			// Encoding sorts object keys, so compare the JSON rather than the raw strings.
			sameCustom, err := sameJSONValue(data.CustomJWKS.StringValue, tc.wantCustom.StringValue)
			if err != nil {
				t.Fatal(err)
			}
			sameResolved, err := sameJSONValue(data.ResolvedJWKS.StringValue, tc.wantResolved.StringValue)
			if err != nil {
				t.Fatal(err)
			}
			if !sameCustom || !sameResolved {
				t.Fatalf("custom_jwks = %v, resolved_jwks = %v; want %v and %v", data.CustomJWKS, data.ResolvedJWKS, tc.wantCustom, tc.wantResolved)
			}
		})
	}
}
