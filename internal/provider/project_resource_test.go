// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

func TestAccProjectResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	// Step 1: create
	gock.New(defaultApiEndpoint).
		Post(projectsApiPath).
		Reply(http.StatusCreated).
		JSON(api.V1ProjectResponse{
			Id:   testProjectRef,
			Name: "foo",
		})
	// Polling for ACTIVE status after create
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	// Disable legacy API keys after create
	gock.New(defaultApiEndpoint).
		Put(legacyApiKeysApiPath).
		MatchParam("enabled", "false").
		Reply(http.StatusOK)
	// readProject after create
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(legacyApiKeysApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
	gock.New(defaultApiEndpoint).
		Get(billingApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0CiMicro,
						"name":  "Micro",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Terraform refresh after create
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(legacyApiKeysApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
	gock.New(defaultApiEndpoint).
		Get(billingApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0CiMicro,
						"name":  "Micro",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Step 2: update instance size
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(legacyApiKeysApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
	gock.New(defaultApiEndpoint).
		Get(billingApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0CiMicro,
						"name":  "Micro",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	gock.New(defaultApiEndpoint).
		Patch(projectApiPath).
		Reply(http.StatusOK)
	gock.New(defaultApiEndpoint).
		Patch(dbPasswordApiPath).
		Reply(http.StatusOK)
	gock.New(defaultApiEndpoint).
		Patch(billingApiPath).
		Reply(http.StatusOK)
	// Polling for ACTIVE status after instance_size update
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "bar",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	// Terraform refresh after update
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "bar",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(legacyApiKeysApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
	gock.New(defaultApiEndpoint).
		Get(billingApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0Ci16xlarge,
						"name":  "16XL",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Step 3: toggle legacy API keys
	// Plan refresh read
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "bar",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(legacyApiKeysApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
	gock.New(defaultApiEndpoint).
		Get(billingApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0Ci16xlarge,
						"name":  "16XL",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Enable legacy API keys
	gock.New(defaultApiEndpoint).
		Put(legacyApiKeysApiPath).
		MatchParam("enabled", "true").
		Reply(http.StatusOK)
	// Post-apply refresh read
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "bar",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(legacyApiKeysApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": true})
	gock.New(defaultApiEndpoint).
		Get(billingApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0Ci16xlarge,
						"name":  "16XL",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Step 4: import state
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "bar",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(legacyApiKeysApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": true})
	gock.New(defaultApiEndpoint).
		Get(billingApiPath).
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0Ci16xlarge,
						"name":  "16XL",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Step 5: delete
	gock.New(defaultApiEndpoint).
		Delete(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})
	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: examples.ProjectResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_project.test", "id", testProjectRef),
					resource.TestCheckResourceAttr("supabase_project.test", "name", "foo"),
					resource.TestCheckResourceAttr("supabase_project.test", "instance_size", "micro"),
					resource.TestCheckResourceAttr("supabase_project.test", "database_password", "barbaz"),
					resource.TestCheckResourceAttr("supabase_project.test", "legacy_api_keys_enabled", "false"),
				),
			},
			// Update instance size testing
			{
				Config: projectResourceConfig(ProjectResourceModel{
					OrganizationId:   types.StringValue("continued-brown-smelt"),
					Name:             types.StringValue("bar"),
					DatabasePassword: types.StringValue("barbaznew"),
					Region:           types.StringValue("us-east-1"),
					InstanceSize:     types.StringValue("16xlarge"),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_project.test", "id", testProjectRef),
					resource.TestCheckResourceAttr("supabase_project.test", "name", "bar"),
					resource.TestCheckResourceAttr("supabase_project.test", "instance_size", "16xlarge"),
					resource.TestCheckResourceAttr("supabase_project.test", "database_password", "barbaznew"),
					resource.TestCheckResourceAttr("supabase_project.test", "legacy_api_keys_enabled", "false"),
				),
			},
			// Toggle legacy API keys
			{
				Config: projectResourceConfig(ProjectResourceModel{
					OrganizationId:       types.StringValue("continued-brown-smelt"),
					Name:                 types.StringValue("bar"),
					DatabasePassword:     types.StringValue("barbaznew"),
					Region:               types.StringValue("us-east-1"),
					InstanceSize:         types.StringValue("16xlarge"),
					LegacyApiKeysEnabled: types.BoolValue(true),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_project.test", "id", testProjectRef),
					resource.TestCheckResourceAttr("supabase_project.test", "name", "bar"),
					resource.TestCheckResourceAttr("supabase_project.test", "instance_size", "16xlarge"),
					resource.TestCheckResourceAttr("supabase_project.test", "database_password", "barbaznew"),
					resource.TestCheckResourceAttr("supabase_project.test", "legacy_api_keys_enabled", "true"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "supabase_project.test",
				ImportState:       true,
				ImportStateVerify: true,

				// database_password is not refreshed from the API
				ImportStateVerifyIgnore: []string{"database_password"},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func projectResourceConfig(p ProjectResourceModel) string {
	rv := fmt.Sprintf(`resource "supabase_project" "test" {
  organization_id         = "%s"
  name                    = "%s"
  database_password       = "%s"
  region                  = "%s"
  instance_size           = "%s"
`,
		p.OrganizationId.ValueString(),
		p.Name.ValueString(),
		p.DatabasePassword.ValueString(),
		p.Region.ValueString(),
		p.InstanceSize.ValueString(),
	)
	if !p.LegacyApiKeysEnabled.IsNull() {
		rv += fmt.Sprintf("\n  legacy_api_keys_enabled = %t", p.LegacyApiKeysEnabled.ValueBool())
	}

	return rv + "\n}"
}

// projectResourceConfigWithTimeouts returns a project resource config string with a timeouts block.
func projectResourceConfigWithTimeouts(p ProjectResourceModel) string {
	s := projectResourceConfig(p)
	idx := strings.LastIndex(s, "}")
	return s[:idx] + `
  timeouts {
    create = "30m"
    update = "30m"
  }
` + s[idx:]
}

func TestAccProjectResource_Timeouts(t *testing.T) {
	defer gock.OffAll()
	// Create
	gock.New("https://api.supabase.com").
		Post("/v1/projects").
		Reply(http.StatusCreated).
		JSON(api.V1ProjectResponse{
			Id:   "mayuaycdtijbctgqbycg",
			Name: "foo",
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New("https://api.supabase.com").
		Put("/v1/projects/mayuaycdtijbctgqbycg/api-keys/legacy").
		MatchParam("enabled", "false").
		Reply(http.StatusOK)
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/api-keys/legacy").
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/billing/addons").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0CiMicro,
						"name":  "Micro",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Post-apply refresh: read project, legacy keys, addons
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             "mayuaycdtijbctgqbycg",
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/api-keys/legacy").
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/billing/addons").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0CiMicro,
						"name":  "Micro",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
	// Delete
	gock.New("https://api.supabase.com").
		Delete("/v1/projects/mayuaycdtijbctgqbycg").
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbExtraSearchPath: "public,extensions",
			DbSchema:          "public,storage,graphql_public",
			MaxRows:           1000,
		})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: projectResourceConfigWithTimeouts(ProjectResourceModel{
					OrganizationId:       types.StringValue("continued-brown-smelt"),
					Name:                 types.StringValue("foo"),
					DatabasePassword:     types.StringValue("barbaz"),
					Region:               types.StringValue("us-east-1"),
					InstanceSize:         types.StringValue("micro"),
					LegacyApiKeysEnabled: types.BoolValue(false),
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_project.test", "id", "mayuaycdtijbctgqbycg"),
					resource.TestCheckResourceAttr("supabase_project.test", "name", "foo"),
				),
			},
		},
	})
}

func projectResourceWriteOnlyConfig(name string, woVersion int64) string {
	return fmt.Sprintf(`resource "supabase_project" "test" {
  organization_id              = "continued-brown-smelt"
  name                         = "%s"
  database_password_wo         = "barbaz"
  database_password_wo_version = %d
  region                       = "us-east-1"
  instance_size                = "micro"
}
`, name, woVersion)
}

// matchJSONField asserts one field of a request body, so a mock can prove the
// write-only secret reached the API without restating the whole payload.
func matchJSONField(key, want string) gock.MatchFunc {
	return func(req *http.Request, _ *gock.Request) (bool, error) {
		if req.Body == nil {
			return false, nil
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return false, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return false, err
		}
		return payload[key] == want, nil
	}
}

func persistProjectReadMocks() {
	// Anchored: gock matches paths as regexes, so an unanchored project path
	// would also swallow the billing and legacy-key reads below.
	gock.New(defaultApiEndpoint).
		Get(projectApiPath + "$").
		Persist().
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "foo",
			OrganizationId: "continued-brown-smelt",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})
	gock.New(defaultApiEndpoint).
		Get(legacyApiKeysApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(map[string]any{"enabled": false})
	gock.New(defaultApiEndpoint).
		Get(billingApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(map[string]any{
			"selected_addons": []map[string]any{
				{
					"type": "compute_instance",
					"variant": map[string]any{
						"id":    api.ListProjectAddonsResponseAvailableAddonsVariantsId0CiMicro,
						"name":  "Micro",
						"price": map[string]any{},
					},
				},
			},
			"available_addons": []map[string]any{},
		})
}

// The write-only password must never reach state, and because it is absent from
// state a rotation can only be driven by the companion version counter.
func TestAccProjectResource_WriteOnlyPassword(t *testing.T) {
	defer gock.OffAll()

	// Create sends the write-only value even though it is absent from the plan.
	gock.New(defaultApiEndpoint).
		Post(projectsApiPath).
		AddMatcher(matchJSONField("db_pass", "barbaz")).
		Reply(http.StatusCreated).
		JSON(api.V1ProjectResponse{Id: testProjectRef, Name: "foo"})
	// Bumping the version counter rotates the password.
	rotatePassword := gock.New(defaultApiEndpoint).
		Patch(dbPasswordApiPath).
		AddMatcher(matchJSONField("password", "barbaz")).
		Reply(http.StatusOK)
	gock.New(defaultApiEndpoint).
		Delete(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{})
	persistProjectReadMocks()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Write-only attributes were introduced in Terraform 1.11.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: projectResourceWriteOnlyConfig("foo", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_project.test", "id", testProjectRef),
					resource.TestCheckResourceAttr("supabase_project.test", "database_password_wo_version", "1"),
					// The secret is kept out of state; only the counter persists.
					resource.TestCheckNoResourceAttr("supabase_project.test", "database_password_wo"),
					resource.TestCheckNoResourceAttr("supabase_project.test", "database_password"),
					checkMockDone("password rotation", rotatePassword, false),
				),
			},
			{
				Config: projectResourceWriteOnlyConfig("foo", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_project.test", "database_password_wo_version", "2"),
					resource.TestCheckNoResourceAttr("supabase_project.test", "database_password_wo"),
					checkMockDone("password rotation", rotatePassword, true),
				),
			},
		},
	})
}

// gock.OffAll does not fail on a mock that was never called, so a test that
// needs a request to be sent must check the mock itself.
func checkMockDone(name string, mock *gock.Response, want bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := mock.Done(); got != want {
			return fmt.Errorf("%s mock done = %t, want %t", name, got, want)
		}
		return nil
	}
}

func TestAccProjectResource_PasswordExactlyOneOf(t *testing.T) {
	defer gock.OffAll()
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: `resource "supabase_project" "test" {
  organization_id = "continued-brown-smelt"
  name            = "foo"
  region          = "us-east-1"
}`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config: `resource "supabase_project" "test" {
  organization_id              = "continued-brown-smelt"
  name                         = "foo"
  region                       = "us-east-1"
  database_password            = "barbaz"
  database_password_wo         = "barbaz"
  database_password_wo_version = 1
}`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config: `resource "supabase_project" "test" {
  organization_id      = "continued-brown-smelt"
  name                 = "foo"
  region               = "us-east-1"
  database_password_wo = "barbaz"
}`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

func TestDatabasePasswordChanged(t *testing.T) {
	for _, tt := range []struct {
		name        string
		plan, state ProjectResourceModel
		want        bool
	}{{
		name:  "inline password unchanged",
		plan:  ProjectResourceModel{DatabasePassword: types.StringValue("a")},
		state: ProjectResourceModel{DatabasePassword: types.StringValue("a")},
		want:  false,
	}, {
		name:  "inline password rotated",
		plan:  ProjectResourceModel{DatabasePassword: types.StringValue("b")},
		state: ProjectResourceModel{DatabasePassword: types.StringValue("a")},
		want:  true,
	}, {
		// The secret is null in both plan and state, so only the counter can say.
		name:  "write-only version unchanged",
		plan:  ProjectResourceModel{DatabasePasswordWoVersion: types.Int64Value(1)},
		state: ProjectResourceModel{DatabasePasswordWoVersion: types.Int64Value(1)},
		want:  false,
	}, {
		name:  "write-only version bumped",
		plan:  ProjectResourceModel{DatabasePasswordWoVersion: types.Int64Value(2)},
		state: ProjectResourceModel{DatabasePasswordWoVersion: types.Int64Value(1)},
		want:  true,
	}, {
		name:  "migrating from inline to write-only rotates",
		plan:  ProjectResourceModel{DatabasePasswordWoVersion: types.Int64Value(1)},
		state: ProjectResourceModel{DatabasePassword: types.StringValue("a")},
		want:  true,
	}} {
		t.Run(tt.name, func(t *testing.T) {
			if got := databasePasswordChanged(&tt.plan, &tt.state); got != tt.want {
				t.Errorf("databasePasswordChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}
