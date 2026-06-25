package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

// baseAuthConfigResponse returns a minimal valid AuthConfigResponse suitable
// for use in test mocks.
func baseAuthConfigResponse() api.AuthConfigResponse {
	return api.AuthConfigResponse{
		SiteUrl:              nullable.NewNullableWithValue("https://example.com"),
		JwtExp:               nullable.NewNullableWithValue(3600),
		DisableSignup:        nullable.NewNullableWithValue(false),
		ExternalEmailEnabled: nullable.NewNullableWithValue(true),
		ExternalPhoneEnabled: nullable.NewNullableWithValue(false),
		PasswordMinLength:    nullable.NewNullableWithValue(8),
		SmtpAdminEmail:       nullable.NewNullNullable[openapi_types.Email](),
		SmtpHost:             nullable.NewNullableWithValue("smtp.example.com"),
		SmtpPort:             nullable.NewNullableWithValue("587"),
		SmtpUser:             nullable.NewNullableWithValue("user@example.com"),
		SmtpPass:             nullable.NewNullableWithValue("hash-of-smtp-pass"),
		SmtpSenderName:       nullable.NewNullableWithValue("Test"),
		SmtpMaxFrequency:     nullable.NewNullableWithValue(60),
		MfaPhoneOtpLength:    6,
	}
}

func mockProjectReady(withPostgrest bool) {
	projectStatusResponse := api.V1ProjectWithDatabaseResponse{
		Id:             testProjectRef,
		Name:           "test",
		OrganizationId: "test-org",
		Region:         "us-east-1",
		Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
	}
	exactPathMatcher := func(req *http.Request, _ *gock.Request) (bool, error) {
		return req.URL.Path == projectApiPath, nil
	}
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		AddMatcher(exactPathMatcher).
		Reply(http.StatusOK).
		JSON(projectStatusResponse)
	if withPostgrest {
		gock.New(defaultApiEndpoint).
			Get(postgrestApiPath).
			Reply(http.StatusOK).
			JSON(api.V1PostgrestConfigResponse{DbSchema: "public,storage,graphql_public"})
	}
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
}

func TestAccAuthSettingsResource_CreateRead(t *testing.T) {
	defer gock.OffAll()

	// Create: project readiness check
	mockProjectReady(true)

	// Create: PATCH auth config
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())

	// Read (refresh after create)
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())

	// Read (plan check / second refresh)
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAuthSettingsConfig_basic(testProjectRef),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "id", testProjectRef),
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "site_url", "https://example.com"),
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "jwt_exp", "3600"),
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "smtp.host", "smtp.example.com"),
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "smtp.pass", "my-plaintext-pass"),
					// secret_hashes should be populated from the API-returned "hash"
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "secret_hashes.smtp_pass", "hash-of-smtp-pass"),
				),
			},
		},
	})
}

func TestAccAuthSettingsResource_SecretDrift(t *testing.T) {
	defer gock.OffAll()

	// Create: project readiness + PATCH
	mockProjectReady(true)
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse()) // hash = "hash-of-smtp-pass"

	// First Read (after create): same hash → no drift
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())

	// RefreshState step: different hash → drift (smtp.pass nulled in state → non-empty plan)
	drifted := baseAuthConfigResponse()
	drifted.SmtpPass = nullable.NewNullableWithValue("NEW-HASH-AFTER-OOB-CHANGE")
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(drifted)
	// post-refresh plan check triggers a second Read
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(drifted)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAuthSettingsConfig_basic(testProjectRef),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "smtp.pass", "my-plaintext-pass"),
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "secret_hashes.smtp_pass", "hash-of-smtp-pass"),
				),
			},
			{
				// RefreshState triggers a Read with the drifted response.
				// smtp.pass should be marked unknown → plan is non-empty.
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccAuthSettingsResource_Update(t *testing.T) {
	defer gock.OffAll()

	// Step 1 Create
	mockProjectReady(true)
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())

	// Step 2: pre-apply refresh returns same hash as step 1 → no drift
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())

	// Step 2 Update: project readiness + PATCH + Read
	mockProjectReady(false)
	updated := baseAuthConfigResponse()
	updated.SiteUrl = nullable.NewNullableWithValue("https://updated.example.com")
	updated.SmtpPass = nullable.NewNullableWithValue("hash-of-new-pass")
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(updated)
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(updated)
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(updated)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAuthSettingsConfig_basic(testProjectRef),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "site_url", "https://example.com"),
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "secret_hashes.smtp_pass", "hash-of-smtp-pass"),
				),
			},
			{
				Config: testAccAuthSettingsConfig_updated(testProjectRef),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "site_url", "https://updated.example.com"),
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "smtp.pass", "my-new-pass"),
					resource.TestCheckResourceAttr("supabase_auth_settings.test", "secret_hashes.smtp_pass", "hash-of-new-pass"),
				),
			},
		},
	})
}

func TestAccAuthSettingsResource_ImportState(t *testing.T) {
	defer gock.OffAll()

	// Create
	mockProjectReady(true)
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())

	// Import reads
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(baseAuthConfigResponse())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAuthSettingsConfig_basic(testProjectRef),
			},
			{
				ResourceName:            "supabase_auth_settings.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"smtp.pass", "timeouts"},
				ImportStateIdFunc: func(*terraform.State) (string, error) {
					return testProjectRef, nil
				},
			},
		},
	})
}

// ── TF configs ───────────────────────────────────────────────────────────────

func testAccAuthSettingsConfig_basic(projectRef string) string {
	return fmt.Sprintf(`
resource "supabase_auth_settings" "test" {
  project_ref           = %q
  site_url              = "https://example.com"
  jwt_exp               = 3600
  disable_signup        = false
  external_email_enabled = true
  external_phone_enabled = false
  password_min_length   = 8

  smtp = {
    host         = "smtp.example.com"
    port         = "587"
    user         = "user@example.com"
    pass         = "my-plaintext-pass"
    sender_name  = "Test"
    max_frequency = 60
  }
}
`, projectRef)
}

func testAccAuthSettingsConfig_updated(projectRef string) string {
	return fmt.Sprintf(`
resource "supabase_auth_settings" "test" {
  project_ref           = %q
  site_url              = "https://updated.example.com"
  jwt_exp               = 3600
  disable_signup        = false
  external_email_enabled = true
  external_phone_enabled = false
  password_min_length   = 8

  smtp = {
    host         = "smtp.example.com"
    port         = "587"
    user         = "user@example.com"
    pass         = "my-new-pass"
    sender_name  = "Test"
    max_frequency = 60
  }
}
`, projectRef)
}
