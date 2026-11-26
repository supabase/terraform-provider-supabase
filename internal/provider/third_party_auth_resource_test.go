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
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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

	resource.ParallelTest(t, resource.TestCase{
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

	resource.ParallelTest(t, resource.TestCase{
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

const (
	testPublicJWKS        = `{"keys":[{"kty":"RSA","kid":"k","n":"abc","e":"AQAB"}]}`
	testPublicJWKSPretty  = "{\n  \"keys\": [\n    {\"kty\": \"RSA\", \"kid\": \"k\", \"n\": \"abc\", \"e\": \"AQAB\"}\n  ]\n}"
	testPrivateJWKS       = `{"keys":[{"kty":"RSA","kid":"k","n":"abc","e":"AQAB","d":"private"}]}`
	testOtherPublicJWKS   = `{"keys":[{"kty":"RSA","kid":"other","n":"def","e":"AQAB"}]}`
	testOIDCIssuerExample = "https://issuer.example.com"
)

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
