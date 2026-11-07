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
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/supabase/terraform-provider-supabase/examples"
	"github.com/supabase/terraform-provider-supabase/internal/gen/api"
	"gopkg.in/h2non/gock.v1"
)

var allServicesHealthy = []api.V1ServiceHealthResponseOutput{
	{Name: api.V1ServiceHealthResponseOutputNameAuth, Status: api.V1ServiceHealthResponseOutputStatusACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseOutputNameDb, Status: api.V1ServiceHealthResponseOutputStatusACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseOutputNameDbPostgresUser, Status: api.V1ServiceHealthResponseOutputStatusACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseOutputNamePgBouncer, Status: api.V1ServiceHealthResponseOutputStatusACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseOutputNamePooler, Status: api.V1ServiceHealthResponseOutputStatusACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseOutputNameRealtime, Status: api.V1ServiceHealthResponseOutputStatusACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseOutputNameRest, Status: api.V1ServiceHealthResponseOutputStatusACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseOutputNameStorage, Status: api.V1ServiceHealthResponseOutputStatusACTIVEHEALTHY, Healthy: true},
}

func TestAccSettingsResource(t *testing.T) {
	defer gock.OffAll()
	projectStatusResponse := api.V1ProjectWithDatabaseResponseOutput{
		Id:             testProjectRef,
		Name:           "test",
		OrganizationId: "test-org",
		Region:         "us-east-1",
		Status:         api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
	}
	exactPathMatcher := func(req *http.Request, _ *gock.Request) (bool, error) {
		return req.URL.Path == projectApiPath, nil
	}
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		AddMatcher(exactPathMatcher).
		Reply(http.StatusOK).
		JSON(projectStatusResponse)
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		AddMatcher(exactPathMatcher).
		Reply(http.StatusOK).
		JSON(projectStatusResponse)
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)

	// Step 1: create
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("10s"),
		})
	gock.New(defaultApiEndpoint).
		Put(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("10s"),
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Post(networkRestrictionsApiPath).
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New(defaultApiEndpoint).
		Patch(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(storageConfigApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
			"capabilities": map[string]any{
				"iceberg_catalog": false,
				"list_v2":         true,
			},
			"external": map[string]any{
				"upstreamTarget": "main",
			},
		})
	gock.New(defaultApiEndpoint).
		Patch(storageConfigApiPath).
		Reply(http.StatusOK)
	gock.New(defaultApiEndpoint).
		Get(storageConfigApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
			"capabilities": map[string]any{
				"iceberg_catalog": false,
				"list_v2":         true,
			},
			"external": map[string]any{
				"upstreamTarget": "main",
			},
		})
	// Step 2: read
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("10s"),
		})
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("10s"),
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(storageConfigApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
			"capabilities": map[string]any{
				"iceberg_catalog": false,
				"list_v2":         true,
			},
			"external": map[string]any{
				"upstreamTarget": "main",
			},
		})
	gock.New(defaultApiEndpoint).
		Get(storageConfigApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
			"capabilities": map[string]any{
				"iceberg_catalog": false,
				"list_v2":         true,
			},
			"external": map[string]any{
				"upstreamTarget": "main",
			},
		})
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, false))
	// Import is followed by a refresh Read, so ssl_enforcement is fetched twice.
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, false))
	// Step 3: update
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("10s"),
		})
	gock.New(defaultApiEndpoint).
		Put(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("20s"),
		})
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("20s"),
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Post(networkRestrictionsApiPath).
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"0.0.0.0/0"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"0.0.0.0/0"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New(defaultApiEndpoint).
		Patch(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           100,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           100,
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:        nullable.NewNullableWithValue("http://localhost:3000"),
			JwtExp:         nullable.NewNullableWithValue(3600),
			SmtpAdminEmail: nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:        nullable.NewNullableWithValue("http://localhost:3000"),
			JwtExp:         nullable.NewNullableWithValue(1800),
			SmtpAdminEmail: nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:        nullable.NewNullableWithValue("http://localhost:3000"),
			JwtExp:         nullable.NewNullableWithValue(1800),
			SmtpAdminEmail: nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(storageConfigApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
			"capabilities": map[string]any{
				"iceberg_catalog": false,
				"list_v2":         true,
			},
			"external": map[string]any{
				"upstreamTarget": "main",
			},
		})
	gock.New(defaultApiEndpoint).
		Patch(storageConfigApiPath).
		Reply(http.StatusOK)
	gock.New(defaultApiEndpoint).
		Get(storageConfigApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
			"capabilities": map[string]any{
				"iceberg_catalog": false,
				"list_v2":         true,
			},
			"external": map[string]any{
				"upstreamTarget": "main",
			},
		})
	gock.New(defaultApiEndpoint).
		Get(storageConfigApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
			"capabilities": map[string]any{
				"iceberg_catalog": false,
				"list_v2":         true,
			},
			"external": map[string]any{
				"upstreamTarget": "main",
			},
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
					resource.TestCheckResourceAttr("supabase_settings.production", "id", testProjectRef),
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

					storage, err := unmarshalStateAttr(state, "storage")
					if err != nil {
						return err
					}
					if storage["fileSizeLimit"] != float64(52428800) {
						return fmt.Errorf("expected storage.fileSizeLimit to be 52428800, got %v", storage["fileSizeLimit"])
					}
					if features, ok := storage["features"].(map[string]any); ok {
						if imgTransform, ok := features["imageTransformation"].(map[string]any); ok {
							if imgTransform["enabled"] != true {
								return fmt.Errorf("expected storage.features.imageTransformation.enabled to be true, got %v", imgTransform["enabled"])
							}
						}
					}

					if projectRef, ok := state.Attributes["project_ref"]; !ok || projectRef != testProjectRef {
						return fmt.Errorf("expected project_ref to be %s, got %v", testProjectRef, projectRef)
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

// settingsConfigWithTimeouts returns the default settings resource config with a timeouts block.
func settingsConfigWithTimeouts() string {
	s := examples.SettingsResourceConfig
	idx := strings.LastIndex(s, "}")
	return s[:idx] + `
  timeouts {
    create = "30m"
    update = "30m"
  }
` + s[idx:]
}

func TestAccSettingsResource_Timeouts(t *testing.T) {
	defer gock.OffAll()
	projectStatusResponse := api.V1ProjectWithDatabaseResponseOutput{
		Id:             "mayuaycdtijbctgqbycg",
		Name:           "test",
		OrganizationId: "test-org",
		Region:         "us-east-1",
		Status:         api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
	}
	exactPathMatcher := func(req *http.Request, _ *gock.Request) (bool, error) {
		return req.URL.Path == "/v1/projects/mayuaycdtijbctgqbycg", nil
	}
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		AddMatcher(exactPathMatcher).
		Reply(http.StatusOK).
		JSON(projectStatusResponse)
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		AddMatcher(exactPathMatcher).
		Reply(http.StatusOK).
		JSON(projectStatusResponse)
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/health").
		Reply(http.StatusOK).
		JSON(allServicesHealthy)

	// Create and post-apply refresh
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("10s"),
		})
	gock.New("https://api.supabase.com").
		Put("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("10s"),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Post("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/storage").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
		})
	gock.New("https://api.supabase.com").
		Patch("/v1/projects/mayuaycdtijbctgqbycg/config/storage").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
		})
	// Post-apply refresh: read configs again
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/database/postgres").
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{
			StatementTimeout: Ptr("10s"),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/network-restrictions").
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs:   Ptr([]string{"0.0.0.0/0"}),
				DbAllowedCidrsV6: Ptr([]string{"::/0"}),
			},
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/postgrest").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/auth").
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/storage").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"fileSizeLimit": 52428800,
			"features": map[string]any{
				"imageTransformation": map[string]any{"enabled": true},
				"s3Protocol":          map[string]any{"enabled": false},
			},
		})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: settingsConfigWithTimeouts(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.production", "id", "mayuaycdtijbctgqbycg"),
				),
			},
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

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
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
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
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
				Config: fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref = "%s"

  auth = jsonencode({
    site_url = "http://localhost:3000"
    smtp_pass = "secret_password_123"
  })
}
`, testProjectRef),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", testProjectRef),
				),
			},
		},
	})
}

func TestAccSettingsResource_AppleAdditionalClientIds(t *testing.T) {
	defer gock.OffAll()

	const appleClientID = "com.example.service"
	const appleAdditionalClientIDs = "com.example.ios,com.example.macos"
	mergedAppleClientIDs := appleClientID + "," + appleAdditionalClientIDs
	authResponse := func() api.AuthConfigResponseOutput {
		return api.AuthConfigResponseOutput{
			SiteUrl:                          nullable.NewNullableWithValue("http://localhost:3000"),
			ExternalAppleEnabled:             nullable.NewNullableWithValue(true),
			ExternalAppleClientId:            nullable.NewNullableWithValue(mergedAppleClientIDs),
			ExternalAppleAdditionalClientIds: nullable.NewNullNullable[string](),
			ExternalAppleEmailOptional:       nullable.NewNullableWithValue(false),
			SmtpAdminEmail:                   nullable.NewNullNullable[openapi_types.Email](),
			MailerOtpExp:                     3600,
			MfaPhoneOtpLength:                6,
			SmsOtpLength:                     6,
		}
	}

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		AddMatcher(func(req *http.Request, _ *gock.Request) (bool, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return false, err
			}
			req.Body = io.NopCloser(bytes.NewBuffer(body))

			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				return false, err
			}

			return payload["external_apple_client_id"] == appleClientID &&
				payload["external_apple_additional_client_ids"] == appleAdditionalClientIDs, nil
		}).
		Reply(http.StatusOK).
		JSON(authResponse())
	for range 2 {
		gock.New(defaultApiEndpoint).
			Get(authConfigApiPath).
			Reply(http.StatusOK).
			JSON(authResponse())
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref = "%s"

  auth = jsonencode({
    site_url                             = "http://localhost:3000"
    external_apple_enabled              = true
    external_apple_client_id            = "%s"
    external_apple_additional_client_ids = "%s"
    external_apple_email_optional       = false
  })
}
`, testProjectRef, appleClientID, appleAdditionalClientIDs),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", testProjectRef),
					resource.TestCheckResourceAttrWith("supabase_settings.test", "auth", func(value string) error {
						var auth map[string]any
						if err := json.Unmarshal([]byte(value), &auth); err != nil {
							return err
						}
						if auth["external_apple_client_id"] != appleClientID {
							return fmt.Errorf("expected external_apple_client_id to be %s, got %v", appleClientID, auth["external_apple_client_id"])
						}
						if auth["external_apple_additional_client_ids"] != appleAdditionalClientIDs {
							return fmt.Errorf("expected external_apple_additional_client_ids to be %s, got %v", appleAdditionalClientIDs, auth["external_apple_additional_client_ids"])
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestAccSettingsResource_IgnoreChanges(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponseOutput{})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Post(networkRestrictionsApiPath).
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
	gock.New(defaultApiEndpoint).
		Post(networkRestrictionsApiPath).
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32", "198.51.100.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32", "198.51.100.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	for range 5 {
		gock.New(defaultApiEndpoint).
			Get(networkRestrictionsApiPath).
			Reply(http.StatusOK).
			JSON(api.NetworkRestrictionsResponseOutput{
				Config: api.NetworkRestrictionsRequest{
					DbAllowedCidrs: Ptr([]string{"203.0.113.1/32", "198.51.100.1/32"}),
				},
			})
		gock.New(defaultApiEndpoint).
			Get(authConfigApiPath).
			Reply(http.StatusOK).
			JSON(api.AuthConfigResponseOutput{
				SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
				MailerOtpExp:      3600,
				MfaPhoneOtpLength: 6,
				SmsOtpLength:      6,
				SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
			})
	}

	authPatchCalled := false
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
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
				Config: fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref = "%s"

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
`, testProjectRef),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", testProjectRef),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref = "%s"

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
`, testProjectRef),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", testProjectRef),
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

func TestParseConfigNestedOmitempty(t *testing.T) {
	userConfig := `{
		"fileSizeLimit": 52428800,
		"features": {
			"icebergCatalog": {"enabled": true},
			"imageTransformation": {"enabled": true},
			"s3Protocol": {"enabled": false}
		}
	}`
	// Built from JSON rather than a struct literal so the fixture does not have
	// to mirror the generated type's anonymous structs field for field.
	var apiResponse api.StorageConfigResponseOutput
	if err := json.Unmarshal([]byte(`{
		"fileSizeLimit": 52428800,
		"features": {
			"icebergCatalog": {"enabled": true},
			"imageTransformation": {"enabled": true},
			"s3Protocol": {"enabled": false},
			"vectorBuckets": {"enabled": false}
		}
	}`), &apiResponse); err != nil {
		t.Fatalf("failed to build API response fixture: %v", err)
	}

	field := jsontypes.NewNormalizedValue(userConfig)
	result, err := parseConfig(field, apiResponse)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	var resultMap map[string]any
	if err := json.Unmarshal([]byte(result.ValueString()), &resultMap); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	features, ok := resultMap["features"].(map[string]any)
	if !ok {
		t.Fatalf("expected features in result, got %v", resultMap)
	}

	iceberg, exists := features["icebergCatalog"]
	if !exists {
		t.Fatalf("expected icebergCatalog to be preserved, got %v", features)
	}
	icebergMap, ok := iceberg.(map[string]any)
	if !ok {
		t.Fatalf("expected icebergCatalog to be a map, got %T", iceberg)
	}
	if icebergMap["enabled"] != true {
		t.Errorf("expected icebergCatalog.enabled to be true, got %v", icebergMap["enabled"])
	}
}

func TestParseConfigNullablePreservesUserValue(t *testing.T) {
	userConfig := `{
		"site_url": "http://localhost:3000",
		"external_apple_additional_client_ids": "com.example.app"
	}`

	apiResponse := api.AuthConfigResponseOutput{
		SiteUrl:                          nullable.NewNullableWithValue("http://localhost:3000"),
		ExternalAppleAdditionalClientIds: nullable.NewNullNullable[string](),
	}

	field := jsontypes.NewNormalizedValue(userConfig)
	result, err := parseConfig(field, apiResponse)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	var resultMap map[string]any
	if err := json.Unmarshal([]byte(result.ValueString()), &resultMap); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if resultMap["site_url"] != "http://localhost:3000" {
		t.Errorf("expected site_url to be 'http://localhost:3000', got %v", resultMap["site_url"])
	}

	// external_apple_additional_client_ids should preserve the user's value
	// because the API returned null for it
	if resultMap["external_apple_additional_client_ids"] != "com.example.app" {
		t.Errorf("expected external_apple_additional_client_ids to be preserved as 'com.example.app', got %v",
			resultMap["external_apple_additional_client_ids"])
	}
}

// Every postgres parameter in the plan must reach the API, including ones the
// generated API client does not know about such as log_connections. The old
// typed decoding silently dropped them, so the apply succeeded while the API
// replaced the config without them.
func TestAccSettingsResource_DatabaseForwardsAllParameters(t *testing.T) {
	defer gock.OffAll()

	databaseConfig := fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref = %q

  database = jsonencode({
    log_connections    = true
    log_disconnections = true
    statement_timeout  = "10s"
  })
}
`, testProjectRef)
	applied := map[string]any{
		"log_connections":    true,
		"log_disconnections": true,
		"statement_timeout":  "10s",
	}

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()
	gock.New(defaultApiEndpoint).
		Put(dbConfigApiPath).
		AddMatcher(matchJSONBody(t, applied)).
		Reply(http.StatusOK).
		JSON(applied)
	// Post-apply refresh.
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(applied)

	// Connection logging is then disabled outside Terraform; the refresh must
	// pick that up even though the generated client cannot decode the field.
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(map[string]any{
			"log_connections":   false,
			"statement_timeout": "10s",
		})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: databaseConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", testProjectRef),
					resource.TestCheckResourceAttrWith("supabase_settings.test", "database", func(value string) error {
						var database map[string]any
						if err := json.Unmarshal([]byte(value), &database); err != nil {
							return err
						}
						if !reflect.DeepEqual(database, applied) {
							return fmt.Errorf("expected database settings %v in state, got %v", applied, database)
						}
						return nil
					}),
				),
			},
			{
				Config:             databaseConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// A null database document must be rejected before anything is sent. The old
// typed decoding turned it into {} and wiped the project's postgres config.
func TestAccSettingsResource_DatabaseRejectsNull(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()
	putCalled := false
	gock.New(defaultApiEndpoint).
		Put(dbConfigApiPath).
		AddMatcher(func(*http.Request, *gock.Request) (bool, error) {
			putCalled = true
			return true, nil
		}).
		Reply(http.StatusBadRequest)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref = %q
  database    = jsonencode(null)
}
`, testProjectRef),
				ExpectError: regexp.MustCompile(`must be a JSON object`),
			},
		},
	})
	if putCalled {
		t.Error("database PUT was called with a null document")
	}
}

func TestDecodeDatabaseConfig(t *testing.T) {
	if _, err := decodeDatabaseConfig(http.StatusBadRequest, []byte(`{"message":"bad request"}`)); err == nil {
		t.Error("expected an error for a non-200 status")
	}
	// A null body would otherwise null out every managed key in state.
	if _, err := decodeDatabaseConfig(http.StatusOK, []byte(`null`)); err == nil {
		t.Error("expected an error for a null body")
	}
	config, err := decodeDatabaseConfig(http.StatusOK, []byte(`{"log_connections":true}`))
	if err != nil {
		t.Fatalf("decodeDatabaseConfig failed: %v", err)
	}
	if config["log_connections"] != true {
		t.Errorf("expected log_connections to be true, got %v", config["log_connections"])
	}
}

func TestParseConfigGenericResponse(t *testing.T) {
	userConfig := `{
		"log_connections": true,
		"log_disconnections": true,
		"statement_timeout": "10s",
		"restart_database": true
	}`
	apiResponse := map[string]any{
		"log_connections":   false,
		"statement_timeout": "10s",
		"max_connections":   float64(100),
	}

	result, err := parseConfig(jsontypes.NewNormalizedValue(userConfig), apiResponse)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	var resultMap map[string]any
	if err := json.Unmarshal([]byte(result.ValueString()), &resultMap); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	expected := map[string]any{
		// Refreshed from the API so out-of-band changes show up as drift.
		"log_connections": false,
		// Removed out-of-band: becomes null rather than keeping the stale value.
		"log_disconnections": nil,
		"statement_timeout":  "10s",
		// Request-only flag the API never echoes back: preserved from the plan.
		"restart_database": true,
		// Keys not managed in the plan are ignored.
	}
	if !reflect.DeepEqual(resultMap, expected) {
		t.Errorf("expected %v, got %v", expected, resultMap)
	}
}

func TestParseConfigGenericResponseWithoutState(t *testing.T) {
	apiResponse := map[string]any{
		"log_connections": true,
		"work_mem":        nil,
	}

	result, err := parseConfig(jsontypes.NewNormalizedNull(), apiResponse)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	var resultMap map[string]any
	if err := json.Unmarshal([]byte(result.ValueString()), &resultMap); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	expected := map[string]any{"log_connections": true}
	if !reflect.DeepEqual(resultMap, expected) {
		t.Errorf("expected %v, got %v", expected, resultMap)
	}
}

func TestAccSettingsResource_WaitsForProjectActive(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusCOMINGUP,
		})

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)

	gock.New(defaultApiEndpoint).
		Post(networkRestrictionsApiPath).
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"0.0.0.0/0"}),
			},
		})

	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"0.0.0.0/0"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponseOutput{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"0.0.0.0/0"}),
			},
		})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref = %q
  network = jsonencode({
    restrictions = ["0.0.0.0/0"]
  })
}
`, testProjectRef),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", testProjectRef),
				),
			},
		},
	})
}

var testAccSettingsResourceConfig = fmt.Sprintf(`
resource "supabase_settings" "production" {
  project_ref = "%s"

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

  storage = jsonencode({
    fileSizeLimit = 52428800
    features = {
      imageTransformation = {
        enabled = true
      }
      s3Protocol = {
        enabled = false
      }
    }
  })

  # pooler = jsonencode({
  #   default_pool_size         = 15
  #   ignore_startup_parameters = ""
  #   max_client_conn           = 200
  #   pool_mode                 = "transaction"
  # })
}
`, testProjectRef)

var sslEnforcementEnabled = fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref     = %q
  ssl_enforcement = true
}
`, testProjectRef)

var sslEnforcementDisabled = fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref     = %q
  ssl_enforcement = false
}
`, testProjectRef)

func sslEnforcementResponse(applied, database bool) api.SslEnforcementResponseOutput {
	resp := api.SslEnforcementResponseOutput{AppliedSuccessfully: applied}
	resp.CurrentConfig.Database = database
	return resp
}

// Verify the PUT body sends the expected boolean.
func matchSslPutBody(database bool) gock.MatchFunc {
	return func(req *http.Request, _ *gock.Request) (bool, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return false, err
		}
		req.Body = io.NopCloser(bytes.NewBuffer(body))
		var payload api.SslEnforcementRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			return false, err
		}
		return payload.RequestedConfig.Database == database, nil
	}
}

// Mock the PostgREST config check and services health check.
func mockServicesActiveHealth() {
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponseOutput{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
}

func TestAccSettingsResource_SslEnforcementEnable(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()
	gock.New(defaultApiEndpoint).
		Put(sslEnforcementApiPath).
		AddMatcher(matchSslPutBody(true)).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: sslEnforcementEnabled,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.test", "id", testProjectRef),
					resource.TestCheckResourceAttr("supabase_settings.test", "ssl_enforcement", "true"),
				),
			},
		},
	})
}

func TestAccSettingsResource_SslEnforcementUpdate(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()
	gock.New(defaultApiEndpoint).
		Put(sslEnforcementApiPath).
		AddMatcher(matchSslPutBody(true)).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()
	gock.New(defaultApiEndpoint).
		Put(sslEnforcementApiPath).
		AddMatcher(matchSslPutBody(false)).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, false))
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, false))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: sslEnforcementEnabled,
				Check:  resource.TestCheckResourceAttr("supabase_settings.test", "ssl_enforcement", "true"),
			},
			{
				Config: sslEnforcementDisabled,
				Check:  resource.TestCheckResourceAttr("supabase_settings.test", "ssl_enforcement", "false"),
			},
		},
	})
}

func TestAccSettingsResource_SslEnforcementAsyncConvergence(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()

	// PUT returns appliedSuccessfully=false, the reboot is still in progress.
	gock.New(defaultApiEndpoint).
		Put(sslEnforcementApiPath).
		AddMatcher(matchSslPutBody(true)).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(false, false))

	// Polling 1st GET: still not applied
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(false, false))

	// Polling 2nd GET: converged
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))

	// SSL enforcement triggers a database reboot, wait for services to recover.
	mockServicesActiveHealth()

	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: sslEnforcementEnabled,
				Check:  resource.TestCheckResourceAttr("supabase_settings.test", "ssl_enforcement", "true"),
			},
		},
	})
}

func TestAccSettingsResource_SslEnforcementCapabilityGatedImport(t *testing.T) {
	defer gock.OffAll()

	// Create with no ssl_enforcement so the resource enters state.
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()

	// ImportState reads all configs. Return 404 for everything except SSL.
	// The known capability-gated 400 keeps ssl_enforcement empty.
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Reply(http.StatusNotFound)
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusNotFound)
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusNotFound)
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusNotFound)
	gock.New(defaultApiEndpoint).
		Get(storageConfigApiPath).
		Reply(http.StatusNotFound)
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusBadRequest).
		BodyString(`"not allowed to configure SSL enforcements"`)

	var minimalConfig = fmt.Sprintf(`
resource "supabase_settings" "test" {
  project_ref = %q
}
`, testProjectRef)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: minimalConfig,
				Check:  resource.TestCheckResourceAttr("supabase_settings.test", "id", testProjectRef),
			},
			{
				ResourceName: "supabase_settings.test",
				ImportState:  true,
				ImportStateIdFunc: func(*terraform.State) (string, error) {
					return testProjectRef, nil
				},
				ImportStateCheck: func(is []*terraform.InstanceState) error {
					if len(is) != 1 {
						return errors.New("expected a single resource in the state")
					}
					if v := is[0].Attributes["ssl_enforcement"]; v != "" {
						return fmt.Errorf("ssl_enforcement should be empty for capability-gated project, got %q", v)
					}
					return nil
				},
			},
		},
	})
}

func TestAccSettingsResource_SslEnforcementCapabilityGatedCreate(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()
	gock.New(defaultApiEndpoint).
		Put(sslEnforcementApiPath).
		AddMatcher(matchSslPutBody(true)).
		Reply(http.StatusBadRequest).
		BodyString(`"not allowed to configure SSL enforcements"`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      sslEnforcementEnabled,
				ExpectError: regexp.MustCompile(`not supported`),
			},
		},
	})
}

func TestAccSettingsResource_SslEnforcement400NonCapability(t *testing.T) {
	defer gock.OffAll()

	// Create first so ssl_enforcement is non-null on the next refresh.
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()
	gock.New(defaultApiEndpoint).
		Put(sslEnforcementApiPath).
		AddMatcher(matchSslPutBody(true)).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))

	// Read returns a non-matching 400 body, the provider must surface a real error.
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusBadRequest).
		BodyString(`"bad request"`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: sslEnforcementEnabled,
				Check:  resource.TestCheckResourceAttr("supabase_settings.test", "ssl_enforcement", "true"),
			},
			{
				Config:      sslEnforcementEnabled,
				ExpectError: regexp.MustCompile(`got status 400`),
			},
		},
	})
}

// Read should trust currentConfig.database even while the reboot is still in progress.
func TestAccSettingsResource_SslEnforcementReadIgnoresAppliedSuccessfully(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponseOutput{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY,
		})
	mockServicesActiveHealth()
	gock.New(defaultApiEndpoint).
		Put(sslEnforcementApiPath).
		AddMatcher(matchSslPutBody(true)).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(true, true))

	// Plan-only refresh during the reboot window. The plan must stay empty.
	gock.New(defaultApiEndpoint).
		Get(sslEnforcementApiPath).
		Times(2).
		Reply(http.StatusOK).
		JSON(sslEnforcementResponse(false, true))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: sslEnforcementEnabled,
				Check:  resource.TestCheckResourceAttr("supabase_settings.test", "ssl_enforcement", "true"),
			},
			{
				Config:   sslEnforcementEnabled,
				PlanOnly: true,
				Check:    resource.TestCheckResourceAttr("supabase_settings.test", "ssl_enforcement", "true"),
			},
		},
	})
}

// Branch refs return 404 from /v1/projects/{ref}; their status is served by
// /v1/branches/{ref} instead. Settings must still apply against them.
// https://github.com/supabase/terraform-provider-supabase/issues/345
func TestAccSettingsResource_BranchRef(t *testing.T) {
	defer gock.OffAll()

	exactBranchProjectPath := func(req *http.Request, _ *gock.Request) (bool, error) {
		return req.URL.Path == branchProjectApiPath, nil
	}
	branchAuthResponse := func(jwtExp int) api.AuthConfigResponseOutput {
		return api.AuthConfigResponseOutput{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			JwtExp:            nullable.NewNullableWithValue(jwtExp),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		}
	}
	branchConfig := func(jwtExp int) string {
		return fmt.Sprintf(`
resource "supabase_settings" "branch" {
  project_ref = "%s"

  auth = jsonencode({
    site_url = "http://localhost:3000"
    jwt_exp  = %d
  })
}
`, testBranchRef, jwtExp)
	}

	// Create and update each wait for the project to become active; branch
	// refs 404 on the projects endpoint and resolve on the branches endpoint.
	for range 2 {
		gock.New(defaultApiEndpoint).
			Get(branchProjectApiPath).
			AddMatcher(exactBranchProjectPath).
			Reply(http.StatusNotFound).
			JSON(map[string]string{"message": "Project not found"})
		gock.New(defaultApiEndpoint).
			Get(branchRefApiPath).
			Reply(http.StatusOK).
			JSON(api.BranchDetailResponseOutput{
				Ref:    testBranchRef,
				Status: api.BranchDetailResponseOutputStatusACTIVEHEALTHY,
			})
		gock.New(defaultApiEndpoint).
			Get(branchPostgrestApiPath).
			Reply(http.StatusOK).
			JSON(api.V1PostgrestConfigResponseOutput{DbSchema: "public,storage,graphql_public"})
		gock.New(defaultApiEndpoint).
			Get(branchHealthApiPath).
			Reply(http.StatusOK).
			JSON(allServicesHealthy)
	}

	// Step 1: create
	gock.New(defaultApiEndpoint).
		Patch(branchAuthConfigApiPath).
		Reply(http.StatusOK).
		JSON(branchAuthResponse(3600))
	for range 2 {
		gock.New(defaultApiEndpoint).
			Get(branchAuthConfigApiPath).
			Reply(http.StatusOK).
			JSON(branchAuthResponse(3600))
	}
	// Step 2: update
	gock.New(defaultApiEndpoint).
		Patch(branchAuthConfigApiPath).
		Reply(http.StatusOK).
		JSON(branchAuthResponse(1800))
	for range 2 {
		gock.New(defaultApiEndpoint).
			Get(branchAuthConfigApiPath).
			Reply(http.StatusOK).
			JSON(branchAuthResponse(1800))
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: branchConfig(3600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.branch", "id", testBranchRef),
				),
			},
			{
				Config: branchConfig(1800),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_settings.branch", "id", testBranchRef),
				),
			},
		},
	})
}

func TestReadOnlyStorageWarning(t *testing.T) {
	if w := readOnlyStorageWarning(jsontypes.NewNormalizedValue(`{"fileSizeLimit":52428800,"features":{"s3Protocol":{"enabled":true}}}`)); w != nil {
		t.Errorf("unexpected warning: %v", w)
	}
	if w := readOnlyStorageWarning(jsontypes.NewNormalizedNull()); w != nil {
		t.Errorf("unexpected warning for null: %v", w)
	}
	if w := readOnlyStorageWarning(jsontypes.NewNormalizedUnknown()); w != nil {
		t.Errorf("unexpected warning for unknown: %v", w)
	}
	w := readOnlyStorageWarning(jsontypes.NewNormalizedValue(`{"fileSizeLimit":52428800,"migrationVersion":"lifecycle-writer-coordination","capabilities":{"list_v2":true,"iceberg_catalog":true,"object_versioning":true}}`))
	if w == nil {
		t.Fatal("expected warning")
	}
	if !strings.Contains(w.Detail(), "capabilities, migrationVersion") {
		t.Errorf("detail missing keys: %s", w.Detail())
	}
}

func TestReadStorageConfigReadOnlyKeys(t *testing.T) {
	// Captured from GET /v1/projects/{ref}/config/storage on staging.
	var apiResponse api.StorageConfigResponseOutput
	const apiPayload = `{"fileSizeLimit":52428800,"features":{"imageTransformation":{"enabled":true},"s3Protocol":{"enabled":true},"purgeCache":{"enabled":true},"icebergCatalog":{"enabled":true,"maxNamespaces":10,"maxTables":10,"maxCatalogs":2},"vectorBuckets":{"enabled":false,"maxBuckets":10,"maxIndexes":5}},"capabilities":{"list_v2":true,"iceberg_catalog":true,"object_versioning":true},"external":{"upstreamTarget":"canary"},"migrationVersion":"lifecycle-writer-coordination"}`
	if err := json.Unmarshal([]byte(apiPayload), &apiResponse); err != nil {
		t.Fatal(err)
	}
	// Import: null state must not pick up read-only keys.
	prev := jsontypes.NewNormalizedNull()
	got, err := parseConfig(prev, apiResponse)
	if err != nil {
		t.Fatal(err)
	}
	got, err = restoreReadOnlyStorageKeys(prev, got)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	if err := json.Unmarshal([]byte(got.ValueString()), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range readOnlyStorageKeys {
		if _, ok := m[k]; ok {
			t.Errorf("read-only key %q leaked into state", k)
		}
	}
	if m["fileSizeLimit"] != float64(52428800) {
		t.Errorf("fileSizeLimit lost: %v", m)
	}
	// Refresh: a user-set value is kept as planned, others are untouched.
	prev = jsontypes.NewNormalizedValue(`{"fileSizeLimit":52428800,"migrationVersion":"revoke-grants-to-unused-operations"}`)
	got, err = parseConfig(prev, apiResponse)
	if err != nil {
		t.Fatal(err)
	}
	got, err = restoreReadOnlyStorageKeys(prev, got)
	if err != nil {
		t.Fatal(err)
	}
	m = map[string]any{}
	if err := json.Unmarshal([]byte(got.ValueString()), &m); err != nil {
		t.Fatal(err)
	}
	if m["migrationVersion"] != "revoke-grants-to-unused-operations" {
		t.Errorf("migrationVersion = %v, want the planned value", m["migrationVersion"])
	}
	if _, ok := m["databasePoolMode"]; ok {
		t.Error("databasePoolMode should not be in state")
	}
}

// Some projects return databasePoolMode as null (a legacy internal Storage flag).
func TestReadStorageConfigNullDatabasePoolMode(t *testing.T) {
	var apiResponse api.StorageConfigResponseOutput
	const apiPayload = `{"fileSizeLimit":52428800,"features":{"imageTransformation":{"enabled":true},"s3Protocol":{"enabled":true},"icebergCatalog":{"enabled":true,"maxNamespaces":10,"maxTables":10,"maxCatalogs":2},"vectorBuckets":{"enabled":false,"maxBuckets":10,"maxIndexes":5}},"capabilities":{"list_v2":true,"iceberg_catalog":true,"object_versioning":true},"external":{"upstreamTarget":"canary"},"databasePoolMode":null,"migrationVersion":"lifecycle-writer-coordination"}`
	if err := json.Unmarshal([]byte(apiPayload), &apiResponse); err != nil {
		t.Fatal(err)
	}
	for name, prev := range map[string]jsontypes.Normalized{
		"import":       jsontypes.NewNormalizedNull(),
		"refresh":      jsontypes.NewNormalizedValue(`{"fileSizeLimit":52428800}`),
		"user-set-key": jsontypes.NewNormalizedValue(`{"fileSizeLimit":52428800,"databasePoolMode":null}`),
	} {
		got, err := parseConfig(prev, apiResponse)
		if err != nil {
			t.Fatal(err)
		}
		got, err = restoreReadOnlyStorageKeys(prev, got)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(got.ValueString()), &m); err != nil {
			t.Fatal(err)
		}
		v, ok := m["databasePoolMode"]
		if name == "user-set-key" {
			// A user-set key is preserved exactly as planned.
			if !ok || v != nil {
				t.Errorf("%s: databasePoolMode = %v (present=%v), want preserved null", name, v, ok)
			}
		} else if ok {
			t.Errorf("%s: databasePoolMode leaked into state: %v", name, v)
		}
		for _, k := range []string{"migrationVersion", "capabilities"} {
			if _, ok := m[k]; ok {
				t.Errorf("%s: read-only key %q leaked into state", name, k)
			}
		}
	}
}
