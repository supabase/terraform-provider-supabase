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
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
	"gopkg.in/h2non/gock.v1"
)

var allServicesHealthy = []api.V1ServiceHealthResponse{
	{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseNameDb, Status: api.ACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseNameDbPostgresUser, Status: api.ACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseNamePgBouncer, Status: api.ACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseNamePooler, Status: api.ACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseNameRealtime, Status: api.ACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseNameRest, Status: api.ACTIVEHEALTHY, Healthy: true},
	{Name: api.V1ServiceHealthResponseNameStorage, Status: api.ACTIVEHEALTHY, Healthy: true},
}

// jsonSubsetCheck is a knownvalue.Check for JSON-encoded string attributes. It
// passes when every expected key is present with an equal value, ignoring any
// extra keys the API returns.
type jsonSubsetCheck struct {
	expected map[string]any
}

func (c jsonSubsetCheck) CheckValue(other any) error {
	raw, ok := other.(string)
	if !ok {
		return fmt.Errorf("expected string value for jsonSubsetCheck, got %T", other)
	}
	return checkJSONSubset(raw, c.expected)
}

func (c jsonSubsetCheck) String() string {
	b, _ := json.Marshal(c.expected)
	return string(b)
}

func checkJSONSubset(raw string, expected map[string]any) error {
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return fmt.Errorf("unable to parse %q as JSON: %w", raw, err)
	}
	for key, want := range expected {
		// Round-trip the expected value so numbers compare as float64, like decoded JSON.
		b, _ := json.Marshal(want)
		var wantNorm any
		_ = json.Unmarshal(b, &wantNorm)
		if !reflect.DeepEqual(got[key], wantNorm) {
			return fmt.Errorf("%s: got %v, want %v", key, got[key], wantNorm)
		}
	}
	return nil
}

func testAccSettingsLiveVariables(organizationID, projectName string, settings map[string]map[string]any) config.Variables {
	vars := config.Variables{
		"organization_id": config.StringVariable(organizationID),
		"project_name":    config.StringVariable(projectName),
	}
	for name, values := range settings {
		obj := map[string]config.Variable{}
		for k, v := range values {
			switch v := v.(type) {
			case string:
				obj[k] = config.StringVariable(v)
			case int:
				obj[k] = config.IntegerVariable(v)
			case bool:
				obj[k] = config.BoolVariable(v)
			default:
				panic(fmt.Sprintf("unsupported settings variable type %T", v))
			}
		}
		vars[name] = config.ObjectVariable(obj)
	}
	return vars
}

func testAccSettingsImportStateID(projectResource string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		project, ok := s.RootModule().Resources[projectResource]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", projectResource)
		}
		return project.Primary.ID, nil
	}
}

func TestAccSettingsResource_basic(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_settings.test"
	projectName := acctest.RandomName()

	api := map[string]any{"db_schema": "public,storage", "max_rows": 1000}
	auth := map[string]any{"site_url": "http://localhost:3000", "mailer_otp_exp": 3600}
	database := map[string]any{"statement_timeout": "10s"}
	storage := map[string]any{"fileSizeLimit": 52428800}
	vars := testAccSettingsLiveVariables(organizationID, projectName, map[string]map[string]any{
		"api": api, "auth": auth, "database": database, "storage": storage,
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables: vars,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
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
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("api"), jsonSubsetCheck{api}),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("auth"), jsonSubsetCheck{auth}),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("database"), jsonSubsetCheck{database}),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("storage"), jsonSubsetCheck{storage}),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("network"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("pooler"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("ssl_enforcement"), knownvalue.Null()),
				},
			},
			// ImportState testing (import reads every config, so verify a subset)
			{
				ConfigDirectory:   config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables:   vars,
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateIdFunc: testAccSettingsImportStateID(projectResourceName),
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return errors.New("expected a single resource in the state")
					}
					for attr, expected := range map[string]map[string]any{
						"api": api, "auth": auth, "database": database, "storage": storage,
					} {
						if err := checkJSONSubset(states[0].Attributes[attr], expected); err != nil {
							return fmt.Errorf("%s: %w", attr, err)
						}
					}
					return nil
				},
			},
		},
	})
}

func TestAccSettingsResource_update(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_settings.test"
	projectName := acctest.RandomName()

	api := map[string]any{"db_schema": "public,storage", "max_rows": 1000}
	auth := map[string]any{"site_url": "http://localhost:3000", "mailer_otp_exp": 3600}
	database := map[string]any{"statement_timeout": "10s"}
	storage := map[string]any{"fileSizeLimit": 52428800}

	updatedAPI := map[string]any{"db_schema": "public", "max_rows": 500}
	updatedAuth := map[string]any{"site_url": "http://localhost:4000", "mailer_otp_exp": 1800}
	updatedDatabase := map[string]any{"statement_timeout": "20s"}
	updatedStorage := map[string]any{"fileSizeLimit": 41943040}

	update := func(name string, settings map[string]map[string]any, checks ...statecheck.StateCheck) resource.TestStep {
		return resource.TestStep{
			ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
			ConfigVariables: testAccSettingsLiveVariables(organizationID, projectName, settings),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
				},
			},
			ConfigStateChecks: checks,
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables: testAccSettingsLiveVariables(organizationID, projectName, map[string]map[string]any{
					"api": api, "auth": auth, "database": database, "storage": storage,
				}),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
			},
			// Update api
			update("api", map[string]map[string]any{
				"api": updatedAPI, "auth": auth, "database": database, "storage": storage,
			},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("api"), jsonSubsetCheck{updatedAPI}),
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("auth"), jsonSubsetCheck{auth}),
			),
			// Update auth
			update("auth", map[string]map[string]any{
				"api": updatedAPI, "auth": updatedAuth, "database": database, "storage": storage,
			},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("auth"), jsonSubsetCheck{updatedAuth}),
			),
			// Update database
			update("database", map[string]map[string]any{
				"api": updatedAPI, "auth": updatedAuth, "database": updatedDatabase, "storage": storage,
			},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("database"), jsonSubsetCheck{updatedDatabase}),
			),
			// Update storage
			update("storage", map[string]map[string]any{
				"api": updatedAPI, "auth": updatedAuth, "database": updatedDatabase, "storage": updatedStorage,
			},
				statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("storage"), jsonSubsetCheck{updatedStorage}),
			),
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

func TestAccSettingsResource_smtpPass(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_settings.test"
	vars := testAccSettingsLiveVariables(organizationID, acctest.RandomName(), map[string]map[string]any{
		"auth": {
			"smtp_admin_email": "admin@example.com",
			"smtp_host":        "smtp.example.com",
			"smtp_port":        "587",
			"smtp_user":        "smtp-user",
			"smtp_pass":        "secret_password_123",
			"smtp_sender_name": "Test",
		},
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// The API returns smtp_pass as a hash; state must keep the configured plaintext.
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables: vars,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("auth"), jsonSubsetCheck{map[string]any{
						"smtp_host": "smtp.example.com",
						"smtp_pass": "secret_password_123",
					}}),
				},
			},
			// A refresh must not show drift from the hashed value.
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables: vars,
				PlanOnly:        true,
			},
		},
	})
}

func TestAccSettingsResource_apple(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_settings.test"
	auth := map[string]any{
		"site_url":                             "http://localhost:3000",
		"external_apple_enabled":               true,
		"external_apple_client_id":             "com.example.service",
		"external_apple_additional_client_ids": "com.example.ios,com.example.macos",
		"external_apple_email_optional":        false,
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// The API merges additional client ids into client_id; state must keep them separate.
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables: testAccSettingsLiveVariables(organizationID, acctest.RandomName(), map[string]map[string]any{"auth": auth}),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("auth"), jsonSubsetCheck{auth}),
				},
			},
		},
	})
}

func TestAccSettingsResource_database(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_settings.test"
	database := map[string]any{
		"log_connections":    true,
		"log_disconnections": true,
		"statement_timeout":  "10s",
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Every configured parameter is sent to the API and read back
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables: testAccSettingsLiveVariables(organizationID, acctest.RandomName(), map[string]map[string]any{"database": database}),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("database"), jsonSubsetCheck{database}),
				},
			},
		},
	})
}

func TestAccSettingsResource_ssl(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_settings.test"
	projectName := acctest.RandomName()
	withSSL := func(enabled bool) config.Variables {
		vars := testAccSettingsLiveVariables(organizationID, projectName, nil)
		vars["ssl_enforcement"] = config.BoolVariable(enabled)
		return vars
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Enable (reboots the database, so the provider waits for services to recover)
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables: withSSL(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("ssl_enforcement"), knownvalue.Bool(true)),
				},
			},
			// A plan-only refresh must stay empty
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables: withSSL(true),
				PlanOnly:        true,
			},
			// Disable
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/Settings/basic/"),
				ConfigVariables: withSSL(false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("ssl_enforcement"), knownvalue.Bool(false)),
				},
			},
		},
	})
}

func TestAccSettingsResource_IgnoreChanges(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
	gock.New(defaultApiEndpoint).
		Get(dbConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.PostgresConfigResponse{})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Post(networkRestrictionsApiPath).
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Patch(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
			SiteUrl:           nullable.NewNullableWithValue("http://localhost:3000"),
			MailerOtpExp:      3600,
			MfaPhoneOtpLength: 6,
			SmsOtpLength:      6,
			SmtpAdminEmail:    nullable.NewNullNullable[openapi_types.Email](),
		})
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
	gock.New(defaultApiEndpoint).
		Post(networkRestrictionsApiPath).
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32", "198.51.100.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"203.0.113.1/32", "198.51.100.1/32"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(authConfigApiPath).
		Reply(http.StatusOK).
		JSON(api.AuthConfigResponse{
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
			JSON(api.NetworkRestrictionsResponse{
				Config: api.NetworkRestrictionsRequest{
					DbAllowedCidrs: Ptr([]string{"203.0.113.1/32", "198.51.100.1/32"}),
				},
			})
		gock.New(defaultApiEndpoint).
			Get(authConfigApiPath).
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
	apiResponse := api.StorageConfigResponse{
		FileSizeLimit: 52428800,
		Features: struct {
			IcebergCatalog struct {
				Enabled       bool `json:"enabled"`
				MaxCatalogs   int  `json:"maxCatalogs"`
				MaxNamespaces int  `json:"maxNamespaces"`
				MaxTables     int  `json:"maxTables"`
			} `json:"icebergCatalog"`
			ImageTransformation struct {
				Enabled bool `json:"enabled"`
			} `json:"imageTransformation"`
			S3Protocol struct {
				Enabled bool `json:"enabled"`
			} `json:"s3Protocol"`
			VectorBuckets struct {
				Enabled    bool `json:"enabled"`
				MaxBuckets int  `json:"maxBuckets"`
				MaxIndexes int  `json:"maxIndexes"`
			} `json:"vectorBuckets"`
		}{
			IcebergCatalog: struct {
				Enabled       bool `json:"enabled"`
				MaxCatalogs   int  `json:"maxCatalogs"`
				MaxNamespaces int  `json:"maxNamespaces"`
				MaxTables     int  `json:"maxTables"`
			}{Enabled: true, MaxCatalogs: 0, MaxNamespaces: 0, MaxTables: 0},
			ImageTransformation: struct {
				Enabled bool `json:"enabled"`
			}{Enabled: true},
			S3Protocol: struct {
				Enabled bool `json:"enabled"`
			}{Enabled: false},
			VectorBuckets: struct {
				Enabled    bool `json:"enabled"`
				MaxBuckets int  `json:"maxBuckets"`
				MaxIndexes int  `json:"maxIndexes"`
			}{Enabled: false, MaxBuckets: 0, MaxIndexes: 0},
		},
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

	apiResponse := api.AuthConfigResponse{
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
func TestAccSettingsResource_DatabaseRejectsNull(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
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
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusCOMINGUP,
		})

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)

	gock.New(defaultApiEndpoint).
		Post(networkRestrictionsApiPath).
		Reply(http.StatusCreated).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"0.0.0.0/0"}),
			},
		})

	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
			Config: api.NetworkRestrictionsRequest{
				DbAllowedCidrs: Ptr([]string{"0.0.0.0/0"}),
			},
		})
	gock.New(defaultApiEndpoint).
		Get(networkRestrictionsApiPath).
		Reply(http.StatusOK).
		JSON(api.NetworkRestrictionsResponse{
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

func sslEnforcementResponse(applied, database bool) api.SslEnforcementResponse {
	resp := api.SslEnforcementResponse{AppliedSuccessfully: applied}
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
		JSON(api.V1PostgrestConfigResponse{DbSchema: "public,storage,graphql_public"})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON(allServicesHealthy)
}

func TestAccSettingsResource_SslEnforcementAsyncConvergence(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
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
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
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
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
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
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
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
func TestAccSettingsResource_BranchRef(t *testing.T) {
	defer gock.OffAll()

	exactBranchProjectPath := func(req *http.Request, _ *gock.Request) (bool, error) {
		return req.URL.Path == branchProjectApiPath, nil
	}
	branchAuthResponse := func(jwtExp int) api.AuthConfigResponse {
		return api.AuthConfigResponse{
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
			JSON(api.BranchDetailResponse{
				Ref:    testBranchRef,
				Status: api.BranchDetailResponseStatusACTIVEHEALTHY,
			})
		gock.New(defaultApiEndpoint).
			Get(branchPostgrestApiPath).
			Reply(http.StatusOK).
			JSON(api.V1PostgrestConfigResponse{DbSchema: "public,storage,graphql_public"})
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
	var apiResponse api.StorageConfigResponse
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
	var apiResponse api.StorageConfigResponse
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
