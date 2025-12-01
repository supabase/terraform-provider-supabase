// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

func TestAccSettingsResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	// Step 1: create
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponse{
			StatementTimeout: Ptr("10s"),
		})
	gock.New("https://api.supabase.com").
		Put("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponse{
			StatementTimeout: Ptr("10s"),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Post("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	// Step 2: read
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponse{
			StatementTimeout: Ptr("10s"),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponse{
			StatementTimeout: Ptr("10s"),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	// Step 3: update
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponse{
			StatementTimeout: Ptr("10s"),
		})
	gock.New("https://api.supabase.com").
		Put("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponse{
			StatementTimeout: Ptr("20s"),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponse{
			StatementTimeout: Ptr("20s"),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Post("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"0.0.0.0/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"0.0.0.0/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           100,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           100,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:        nullable.NewNullableWithValue("http://localhost:3000"),
			JwtExp:         nullable.NewNullableWithValue(3600),
			SmtpAdminEmail: nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:        nullable.NewNullableWithValue("http://localhost:3000"),
			JwtExp:         nullable.NewNullableWithValue(1800),
			SmtpAdminEmail: nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:        nullable.NewNullableWithValue("http://localhost:3000"),
			JwtExp:         nullable.NewNullableWithValue(1800),
			SmtpAdminEmail: nullable.NewNullNullable[openapi_types.Email](),
		})
	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: examples.SettingsResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.production", "id", "mayuaycdtijbctgqbycg"),
				),
			},
			// ImportState testing
			{
				ResourceName: "supabase_settings.production",
				ImportState:  true,
				ImportStateCheck: func(is []*terraform.InstanceState) error {
					if len(is) != 1 {
						return errors.New("expected a single resource in the state")
					}

					state := is[0]

					api, err := unmarshalStateAttr(state, "api")
					if err != nil {
						return err
					}
					if api["db_extra_search_path"] != "public,extensions" {
						return fmt.Errorf("expected api.db_extra_search_path to be public,extensions, got %v", api["db_extra_search_path"])
					}
					if api["db_schema"] != "public,storage,graphql_public" {
						return fmt.Errorf("expected api.db_schema to be public,storage,graphql_public, got %v", api["db_schema"])
					}
					if api["max_rows"] != float64(1000) {
						return fmt.Errorf("expected api.max_rows to be 1000, got %v", api["max_rows"])
					}

					auth, err := unmarshalStateAttr(state, "auth")
					if err != nil {
						return err
					}
					if auth["site_url"] != "http://localhost:3000" {
						return fmt.Errorf("expected auth.site_url to be http://localhost:3000, got %v", auth["site_url"])
					}
					if auth["mailer_otp_exp"] != float64(3600) {
						return fmt.Errorf("expected auth.mailer_otp_exp to be 3600, got %v", auth["mailer_otp_exp"])
					}
					if auth["mfa_phone_otp_length"] != float64(6) {
						return fmt.Errorf("expected auth.mfa_phone_otp_length to be 6, got %v", auth["mfa_phone_otp_length"])
					}
					if auth["sms_otp_length"] != float64(6) {
						return fmt.Errorf("expected auth.sms_otp_length to be 6, got %v", auth["sms_otp_length"])
					}
					if _, found := auth["smtp_admin_email"]; found {
						return fmt.Errorf("expected auth.smtp_admin_email to be filtered out, got %v", auth["smtp_admin_email"])
					}

					if projectRef, ok := state.Attributes["project_ref"]; !ok || projectRef != "mayuaycdtijbctgqbycg" {
						return fmt.Errorf("expected project_ref to be mayuaycdtijbctgqbycg, got %v", projectRef)
					}

					return nil
				},
			},
			// Update and Read testing
			{
				Config: testAccSettingsResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("supabase_settings.production", "api"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func unmarshalStateAttr(state *terraform.InstanceState, attr string) (map[string]any, error) {
	raw, ok := state.Attributes[attr]
	if !ok {
		return nil, fmt.Errorf("attribute %s not found in state with ID %s", attr, state.ID)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}

	return out, nil
}

func TestAccSettingsResource_SmtpPass(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		AddMatcher(func(req *http.Request, _ *gock.Request) (bool, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return false, err
			}
			req.Body = io.NopCloser(bytes.NewBuffer(body))
			bodyStr := string(body)
			return strings.Contains(bodyStr, `"smtp_pass"`) &&
				strings.Contains(bodyStr, `"secret_password_123"`), nil
		}).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "supabase_settings" "test" {
  project_ref = "mayuaycdtijbctgqbycg"

  auth = jsonencode({
    site_url = "http://localhost:3000"
    smtp_pass = "secret_password_123"
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", "mayuaycdtijbctgqbycg"),
				),
			},
		},
	})
}

func TestAccSettingsResource_IgnoreChanges(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"

	gock.New("https://api.supabase.com").
		Get("/v1/projects/" + projectRef + "/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponse{})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/" + projectRef + "/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32"}),
			},
		})
	gock.New("https://api.supabase.com").
		Post("/v1/projects/" + projectRef + "/network-restrictions").
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/" + projectRef + "/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/" + projectRef + "/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/" + projectRef + "/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/" + projectRef + "/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/" + projectRef + "/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Post("/v1/projects/" + projectRef + "/network-restrictions").
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32", "198.51.100.1/32"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/" + projectRef + "/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32", "198.51.100.1/32"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/" + projectRef + "/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	for range 5 {
		gock.New("https://api.supabase.com").
			Get("/v1/projects/" + projectRef + "/network-restrictions").
			Reply(http.StatusOK).
			JSON(api.NetworkRestrictionsResponse{
				Config: api.NetworkRestrictionsRequest{
					DbAllowedCidrs: Ptr([]string{"203.0.113.1/32", "198.51.100.1/32"}),
				},
			})
		gock.New("https://api.supabase.com").
			Get("/v1/projects/" + projectRef + "/config/auth").
			Reply(http.StatusOK).
			JSON(api.AuthConfigResponse{
				SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
				MailerOtpExp:      3600,
				MfaPhoneOtpLength: 6,
				SmsOtpLength:      6,
				SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
			})
	}

	authPatchCalled := false
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/" + projectRef + "/config/auth").
		AddMatcher(func(req *http.Request, _ *gock.Request) (bool, error) {
			authPatchCalled = true
			return true, nil
		}).
		Reply(http.StatusBadRequest).
		JSON(map[string]any{
			"message": "Should not be called",
		})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "supabase_settings" "test" {
  project_ref = "mayuaycdtijbctgqbycg"

  network = jsonencode({
    restrictions = ["203.0.113.1/32"]
  })

  auth = jsonencode({
    site_url = "http://localhost:3000"
  })

  lifecycle {
    ignore_changes = [auth]
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", projectRef),
				),
			},
			{
				Config: `
resource "supabase_settings" "test" {
  project_ref = "mayuaycdtijbctgqbycg"

  network = jsonencode({
    restrictions = ["203.0.113.1/32", "198.51.100.1/32"]
  })

  auth = jsonencode({
    site_url = "http://localhost:3000"
  })

  lifecycle {
    ignore_changes = [auth]
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", projectRef),
					func(s *terraform.State) error {
						if authPatchCalled {
							return fmt.Errorf("auth PATCH was called despite lifecycle.ignore_changes")
						}
						return nil
					},
				),
			},
		},
	})
}

const testAccSettingsResourceConfig = `
resource "supabase_settings" "production" {
  project_ref = "mayuaycdtijbctgqbycg"

  database = jsonencode({
    statement_timeout = "20s"
  })

  network = jsonencode({
	restrictions = ["0.0.0.0/0"]
  })

  api = jsonencode({
	db_schema            = "public,storage,graphql_public"
    db_extra_search_path = "public,extensions"
	max_rows             = 100
  })

  auth = jsonencode({
    site_url = "http://localhost:3000"
    jwt_exp  = 1800
  })

  # storage = jsonencode({
  #   file_size_limit = "50MB"
  # })

  # pooler = jsonencode({
  #   default_pool_size         = 15
  #   ignore_startup_parameters = ""
  #   max_client_conn           = 200
  #   pool_mode                 = "transaction"
  # })
}
`
