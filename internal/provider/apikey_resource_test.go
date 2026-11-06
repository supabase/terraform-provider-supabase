// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

const testAccApikeyResourceConfig = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "test"
}
`

const testAccApikeyResourceConfigWithDescription = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "test"
  description = "Service key for test"
}
`

const testAccApikeyResourceConfigInvalidName = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "Invalid-Name-123" # Contains capital letters and hyphens
}
`

func TestAccApiKeyResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()
	// Step 1: create
	gock.New(defaultApiEndpoint).
		Get(apiKeysApiPath).
		Reply(http.StatusOK).
		JSON([]api.ApiKeyResponse{
			{
				Name:   "anon",
				Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeLegacy),
				ApiKey: nullable.NewNullableWithValue("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.anon"),
			},
			{
				Name:   "service_role",
				Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeLegacy),
				ApiKey: nullable.NewNullableWithValue("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.service_role"),
			},
		})
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(uuid.New().String()),
			Name:   "default",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
			ApiKey: nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(testApiKeyUUID),
			Name:   "test",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey: nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(testApiKeyUUID),
			Name:   "test",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey: nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	mockApiKeyRoles(map[string]string{testApiKeyUUID: "service_role"})
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
		Reply(http.StatusOK)

	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: examples.ApiKeyResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "id", testApiKeyUUID),
					resource.TestCheckResourceAttr("supabase_apikey.new", "secret_jwt_template.role", "service_role"),
				),
			},
			// ImportState testing
			{
				ResourceName:            "supabase_apikey.new",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"name", "project_ref"},
				ImportStateId:           fmt.Sprintf("%s/%s", testProjectRef, testApiKeyUUID),
			},
			// Update and Read testing
			{
				Config: testAccApikeyResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "name", "test"),
					resource.TestCheckResourceAttr("supabase_apikey.new", "project_ref", testProjectRef),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// Mocks the list of keys that a secret key's role is read from. Reading a single key doesn't return it.
func mockApiKeyRoles(roles map[string]string) {
	keys := make([]api.ApiKeyResponse, 0, len(roles))
	for id, role := range roles {
		keys = append(keys, api.ApiKeyResponse{
			Id:                nullable.NewNullableWithValue(id),
			Name:              "test",
			Type:              nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			SecretJwtTemplate: nullable.NewNullableWithValue(map[string]interface{}{"role": role}),
		})
	}
	gock.New(defaultApiEndpoint).
		Get(apiKeysApiPath+"$").
		MatchParam("reveal", "false").
		Persist().
		Reply(http.StatusOK).
		JSON(keys)
}

// Mocks the project's existing keys, so creating a secret key doesn't also create a default publishable key.
func mockApiKeyDefaultPublishable() {
	gock.New(defaultApiEndpoint).
		Get(apiKeysApiPath+"$").
		MatchParam("reveal", "true").
		Persist().
		Reply(http.StatusOK).
		JSON([]api.ApiKeyResponse{{
			Id:   nullable.NewNullableWithValue(uuid.New().String()),
			Name: "default",
			Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
		}})
}

func secretApiKeyResponse(id, description string) api.ApiKeyResponse {
	resp := api.ApiKeyResponse{
		Id:     nullable.NewNullableWithValue(id),
		Name:   "test",
		Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
		ApiKey: nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
	}
	if description != "" {
		resp.Description = nullable.NewNullableWithValue(description)
	}
	return resp
}

func testAccApikeyResourceConfigRole(role, description string) string {
	config := fmt.Sprintf(`
resource "supabase_apikey" "new" {
  project_ref = %q
  name        = "test"
`, testProjectRef)
	if description != "" {
		config += fmt.Sprintf("  description = %q\n", description)
	}
	return config + fmt.Sprintf(`
  secret_jwt_template = {
    role = %q
  }
}
`, role)
}

func TestAccApiKeyResource_CustomRole(t *testing.T) {
	defer gock.OffAll()

	updated := false
	mockApiKeyDefaultPublishable()
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"name":                "test",
			"type":                "secret",
			"secret_jwt_template": map[string]any{"role": "app_reader"},
		})).
		Reply(http.StatusCreated).
		JSON(secretApiKeyResponse(testApiKeyUUID, ""))
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		AddMatcher(func(*http.Request, *gock.Request) (bool, error) { return !updated, nil }).
		Persist().
		Reply(http.StatusOK).
		JSON(secretApiKeyResponse(testApiKeyUUID, ""))
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		AddMatcher(func(*http.Request, *gock.Request) (bool, error) { return updated, nil }).
		Persist().
		Reply(http.StatusOK).
		JSON(secretApiKeyResponse(testApiKeyUUID, "Read only key"))
	mockApiKeyRoles(map[string]string{testApiKeyUUID: "app_reader"})
	// An update sends the key's current role, never service_role.
	matchUpdateBody := matchJSONBody(t, map[string]any{
		"name":                "test",
		"description":         "Read only key",
		"secret_jwt_template": map[string]any{"role": "app_reader"},
	})
	gock.New(defaultApiEndpoint).
		Patch(apiKeyApiPath).
		AddMatcher(func(req *http.Request, ereq *gock.Request) (bool, error) {
			ok, err := matchUpdateBody(req, ereq)
			updated = ok
			return ok, err
		}).
		Reply(http.StatusOK).
		JSON(secretApiKeyResponse(testApiKeyUUID, "Read only key"))
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApikeyResourceConfigRole("app_reader", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "id", testApiKeyUUID),
					resource.TestCheckResourceAttr("supabase_apikey.new", "secret_jwt_template.role", "app_reader"),
				),
			},
			{
				ResourceName:            "supabase_apikey.new",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"project_ref"},
				ImportStateId:           fmt.Sprintf("%s/%s", testProjectRef, testApiKeyUUID),
			},
			{
				Config: testAccApikeyResourceConfigRole("app_reader", "Read only key"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("supabase_apikey.new", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "description", "Read only key"),
					resource.TestCheckResourceAttr("supabase_apikey.new", "secret_jwt_template.role", "app_reader"),
				),
			},
			// Removing the role from config keeps the key and its role.
			{
				Config:   testAccApikeyResourceConfigWithDescriptionReadOnly,
				PlanOnly: true,
			},
		},
	})

	if !updated {
		t.Fatal("expected the key to be updated")
	}
}

const testAccApikeyResourceConfigWithDescriptionReadOnly = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "test"
  description = "Read only key"
}
`

func TestAccApiKeyResource_RoleChangeReplaces(t *testing.T) {
	defer gock.OffAll()

	replacementID := "99999999-9999-4999-9999-999999999999"
	replacementPath := apiKeysApiPath + "/" + replacementID

	drifted := false
	mockApiKeyDefaultPublishable()
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"name":                "test",
			"type":                "secret",
			"secret_jwt_template": map[string]any{"role": "app_reader"},
		})).
		Reply(http.StatusCreated).
		JSON(secretApiKeyResponse(testApiKeyUUID, ""))
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(secretApiKeyResponse(testApiKeyUUID, ""))
	// The role changed outside Terraform.
	gock.New(defaultApiEndpoint).
		Get(apiKeysApiPath+"$").
		MatchParam("reveal", "false").
		AddMatcher(func(*http.Request, *gock.Request) (bool, error) { return drifted, nil }).
		Persist().
		Reply(http.StatusOK).
		JSON([]api.ApiKeyResponse{{
			Id:                nullable.NewNullableWithValue(testApiKeyUUID),
			Name:              "test",
			Type:              nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			SecretJwtTemplate: nullable.NewNullableWithValue(map[string]interface{}{"role": "service_role"}),
		}})
	mockApiKeyRoles(map[string]string{testApiKeyUUID: "app_reader", replacementID: "app_writer"})
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
		Reply(http.StatusOK)
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"name":                "test",
			"type":                "secret",
			"secret_jwt_template": map[string]any{"role": "app_writer"},
		})).
		Reply(http.StatusCreated).
		JSON(secretApiKeyResponse(replacementID, ""))
	gock.New(defaultApiEndpoint).
		Get(replacementPath).
		Persist().
		Reply(http.StatusOK).
		JSON(secretApiKeyResponse(replacementID, ""))
	gock.New(defaultApiEndpoint).
		Delete(replacementPath).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApikeyResourceConfigRole("app_reader", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "id", testApiKeyUUID),
					resource.TestCheckResourceAttr("supabase_apikey.new", "secret_jwt_template.role", "app_reader"),
				),
			},
			// A role changed outside Terraform is detected on refresh and replaces the key.
			{
				PreConfig:          func() { drifted = true },
				Config:             testAccApikeyResourceConfigRole("app_reader", ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("supabase_apikey.new", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
			},
			{
				PreConfig: func() { drifted = false },
				Config:    testAccApikeyResourceConfigRole("app_writer", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("supabase_apikey.new", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "id", replacementID),
					resource.TestCheckResourceAttr("supabase_apikey.new", "secret_jwt_template.role", "app_writer"),
				),
			},
		},
	})
}

func TestAccApiKeyResource_ReadRemovesDeletedResource(t *testing.T) {
	defer gock.OffAll()

	deleted := false
	mockApiKeyDefaultPublishable()
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		Reply(http.StatusCreated).
		JSON(secretApiKeyResponse(testApiKeyUUID, ""))
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		AddMatcher(func(*http.Request, *gock.Request) (bool, error) { return !deleted, nil }).
		Persist().
		Reply(http.StatusOK).
		JSON(secretApiKeyResponse(testApiKeyUUID, ""))
	// The API answers 406, not 404, for a key that doesn't exist.
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		AddMatcher(func(*http.Request, *gock.Request) (bool, error) { return deleted, nil }).
		Persist().
		Reply(http.StatusNotAcceptable).
		JSON(map[string]any{"message": "Failed to find API key for project"})
	mockApiKeyRoles(map[string]string{testApiKeyUUID: "service_role"})
	// Destroy treats a key that is already gone as deleted.
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
		Reply(http.StatusNotAcceptable).
		JSON(map[string]any{"message": "Failed to find API key for project"})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApikeyResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "id", testApiKeyUUID),
				),
			},
			{
				PreConfig:          func() { deleted = true },
				Config:             testAccApikeyResourceConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("supabase_apikey.new", plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

func TestIsApiKeyNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
		want       bool
	}{
		{name: "404", statusCode: http.StatusNotFound, body: `{"message":"Not Found"}`, want: true},
		{name: "406 missing key", statusCode: http.StatusNotAcceptable, body: `{"message":"Failed to find API key for project"}`, want: true},
		{name: "406 other error", statusCode: http.StatusNotAcceptable, body: `{"message":"Project is paused"}`, want: false},
		{name: "500", statusCode: http.StatusInternalServerError, body: `{"message":"Failed to find API key for project"}`, want: false},
		{name: "200", statusCode: http.StatusOK, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isApiKeyNotFound(tt.statusCode, []byte(tt.body)); got != tt.want {
				t.Errorf("isApiKeyNotFound(%d, %q) = %v, want %v", tt.statusCode, tt.body, got, tt.want)
			}
		})
	}
}

func TestReadApiKeyDatabase(t *testing.T) {
	tests := []struct {
		name        string
		stateID     string
		statusCode  int
		body        any
		listRoles   map[string]string
		wantFound   bool
		wantRole    string
		expectError bool
	}{
		{name: "found", statusCode: http.StatusOK, body: secretApiKeyResponse(testApiKeyUUID, ""), listRoles: map[string]string{testApiKeyUUID: "app_reader"}, wantFound: true, wantRole: "app_reader"},
		// An ID imported in upper case still finds the role in the list of keys.
		{name: "upper case id", stateID: strings.ToUpper(testApiKeyUUID), statusCode: http.StatusOK, body: secretApiKeyResponse(testApiKeyUUID, ""), listRoles: map[string]string{testApiKeyUUID: "app_reader"}, wantFound: true, wantRole: "app_reader"},
		// A key missing from the list of keys reads with no role, as plain Read always has.
		{name: "role not listed", statusCode: http.StatusOK, body: secretApiKeyResponse(testApiKeyUUID, ""), listRoles: map[string]string{}, wantFound: true},
		{name: "gone (404)", statusCode: http.StatusNotFound, body: map[string]any{"message": "Not Found"}},
		{name: "gone (406)", statusCode: http.StatusNotAcceptable, body: map[string]any{"message": "Failed to find API key for project"}},
		{name: "other 406", statusCode: http.StatusNotAcceptable, body: map[string]any{"message": "Project is paused"}, expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gock.InterceptClient(http.DefaultClient)
			defer gock.RestoreClient(http.DefaultClient)
			defer gock.OffAll()

			gock.New(defaultApiEndpoint).
				Get(apiKeyApiPath).
				Reply(tt.statusCode).
				JSON(tt.body)
			if tt.listRoles != nil {
				mockApiKeyRoles(tt.listRoles)
			}

			client, err := api.NewClientWithResponses(defaultApiEndpoint)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			stateID := tt.stateID
			if stateID == "" {
				stateID = testApiKeyUUID
			}
			state := ApiKeyResourceModel{
				ProjectRef: types.StringValue(testProjectRef),
				Id:         types.StringValue(stateID),
			}
			found, diags := readApiKeyDatabase(t.Context(), &state, client)
			if diags.HasError() != tt.expectError {
				t.Fatalf("expected error %v, got: %v", tt.expectError, diags)
			}
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if tt.wantFound {
				if got := secretJwtRoleFromObject(state.SecretJwtTemplate).ValueString(); got != tt.wantRole {
					t.Errorf("role = %q, want %q", got, tt.wantRole)
				}
				if state.Id.ValueString() != testApiKeyUUID {
					t.Errorf("id = %q, want %q", state.Id.ValueString(), testApiKeyUUID)
				}
			}
		})
	}
}

func TestUpdateApiKeyKeepsCustomRole(t *testing.T) {
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)
	defer gock.OffAll()

	patch := gock.New(defaultApiEndpoint).
		Patch(apiKeyApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"name":                "test",
			"description":         "changed",
			"secret_jwt_template": map[string]any{"role": "app_reader"},
		})).
		Reply(http.StatusOK).
		JSON(secretApiKeyResponse(testApiKeyUUID, "changed"))
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Reply(http.StatusOK).
		JSON(secretApiKeyResponse(testApiKeyUUID, "changed"))
	mockApiKeyRoles(map[string]string{testApiKeyUUID: "app_reader"})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	template, diags := types.ObjectValue(secretJwtTemplateAttrTypes, map[string]attr.Value{"role": types.StringValue("app_reader")})
	if diags.HasError() {
		t.Fatalf("building template: %v", diags)
	}
	plan := ApiKeyResourceModel{
		ProjectRef:        types.StringValue(testProjectRef),
		Id:                types.StringValue(testApiKeyUUID),
		Name:              types.StringValue("test"),
		Description:       types.StringValue("changed"),
		Type:              types.StringValue(string(api.ApiKeyResponseTypeSecret)),
		SecretJwtTemplate: template,
	}
	if diags := updateApiKey(t.Context(), &plan, client); diags.HasError() {
		t.Fatalf("updateApiKey: %v", diags)
	}
	if got := secretJwtRoleFromObject(plan.SecretJwtTemplate).ValueString(); got != "app_reader" {
		t.Errorf("role = %q, want app_reader", got)
	}
	if !patch.Done() {
		t.Fatal("PATCH with the custom role was not sent")
	}
}

// State written by an earlier provider version has no role. An update made without a refresh
// must send the key's current role, not drop or overwrite it, and must return the role as planned.
func TestUpdateApiKeyRoleNotInState(t *testing.T) {
	tests := []struct {
		name     string
		role     types.String
		wantRole types.String
	}{
		{name: "null role", role: types.StringNull(), wantRole: types.StringNull()},
		{name: "unknown role", role: types.StringUnknown(), wantRole: types.StringValue("app_reader")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gock.InterceptClient(http.DefaultClient)
			defer gock.RestoreClient(http.DefaultClient)
			defer gock.OffAll()

			patch := gock.New(defaultApiEndpoint).
				Patch(apiKeyApiPath).
				AddMatcher(matchJSONBody(t, map[string]any{
					"name":                "test",
					"description":         "changed",
					"secret_jwt_template": map[string]any{"role": "app_reader"},
				})).
				Reply(http.StatusOK).
				JSON(secretApiKeyResponse(testApiKeyUUID, "changed"))
			gock.New(defaultApiEndpoint).
				Get(apiKeyApiPath).
				Reply(http.StatusOK).
				JSON(secretApiKeyResponse(testApiKeyUUID, "changed"))
			mockApiKeyRoles(map[string]string{testApiKeyUUID: "app_reader"})

			client, err := api.NewClientWithResponses(defaultApiEndpoint)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			template, diags := types.ObjectValue(secretJwtTemplateAttrTypes, map[string]attr.Value{"role": tt.role})
			if diags.HasError() {
				t.Fatalf("building template: %v", diags)
			}
			plan := ApiKeyResourceModel{
				ProjectRef:        types.StringValue(testProjectRef),
				Id:                types.StringValue(testApiKeyUUID),
				Name:              types.StringValue("test"),
				Description:       types.StringValue("changed"),
				Type:              types.StringValue(string(api.ApiKeyResponseTypeSecret)),
				SecretJwtTemplate: template,
			}
			if diags := updateApiKey(t.Context(), &plan, client); diags.HasError() {
				t.Fatalf("updateApiKey: %v", diags)
			}
			if got := secretJwtRoleFromObject(plan.SecretJwtTemplate); !got.Equal(tt.wantRole) {
				t.Errorf("role = %s, want %s", got, tt.wantRole)
			}
			if !patch.Done() {
				t.Fatal("PATCH with the current role was not sent")
			}
		})
	}
}

func TestRequiresReplaceSecretJwtRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config types.String
		state  types.String
		want   bool
	}{
		{name: "configured role changes", config: types.StringValue("app_writer"), state: types.StringValue("app_reader"), want: true},
		{name: "role removed from config", config: types.StringNull(), state: types.StringValue("app_reader"), want: false},
		{name: "role not in state yet", config: types.StringValue("service_role"), state: types.StringNull(), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := &stringplanmodifier.RequiresReplaceIfFuncResponse{}
			requiresReplaceSecretJwtRole(t.Context(), planmodifier.StringRequest{ConfigValue: tt.config, StateValue: tt.state, PlanValue: tt.config}, resp)
			if resp.RequiresReplace != tt.want {
				t.Errorf("RequiresReplace = %v, want %v", resp.RequiresReplace, tt.want)
			}
		})
	}
}

func TestValidateSecretJwtRoleForType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		keyType   types.String
		role      types.String
		wantError bool
	}{
		{name: "secret with role", keyType: types.StringValue("secret"), role: types.StringValue("app_reader")},
		{name: "secret without role", keyType: types.StringValue("secret"), role: types.StringNull()},
		{name: "publishable without role", keyType: types.StringValue("publishable"), role: types.StringNull()},
		{name: "publishable with role", keyType: types.StringValue("publishable"), role: types.StringValue("anon"), wantError: true},
		{name: "publishable with unknown role", keyType: types.StringValue("publishable"), role: types.StringUnknown(), wantError: true},
		{name: "legacy with role", keyType: types.StringValue("legacy"), role: types.StringValue("anon"), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := validateSecretJwtRoleForType(tt.keyType, tt.role); (got != "") != tt.wantError {
				t.Errorf("validateSecretJwtRoleForType(%s, %s) = %q, want error %v", tt.keyType, tt.role, got, tt.wantError)
			}
		})
	}
}

func TestAccApiKeyResource_RoleOnPublishableKey(t *testing.T) {
	defer gock.OffAll()

	publishable := api.ApiKeyResponse{
		Id:     nullable.NewNullableWithValue(testApiKeyUUID),
		Name:   "test",
		Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
		ApiKey: nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
	}
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(publishable)
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
		Reply(http.StatusOK)

	config := testAccApikeyResourceConfigRole("anon", "")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             config,
				ResourceName:       "supabase_apikey.new",
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateId:      fmt.Sprintf("%s/%s", testProjectRef, testApiKeyUUID),
			},
			// Replacing the key would delete the publishable key and create a secret one.
			{
				Config:      config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`can only be set on a secret API key`),
			},
		},
	})
}

func TestDeleteApiKey(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        map[string]any
		expectError bool
	}{
		{name: "deleted", statusCode: http.StatusOK, body: map[string]any{"name": "test"}},
		{name: "already deleted (404)", statusCode: http.StatusNotFound, body: map[string]any{"message": "Not Found"}},
		{name: "already deleted (406)", statusCode: http.StatusNotAcceptable, body: map[string]any{"message": "Failed to find API key for project"}},
		{name: "other 406", statusCode: http.StatusNotAcceptable, body: map[string]any{"message": "Project is paused"}, expectError: true},
		{name: "server error", statusCode: http.StatusInternalServerError, body: map[string]any{"message": "boom"}, expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gock.InterceptClient(http.DefaultClient)
			defer gock.RestoreClient(http.DefaultClient)
			defer gock.OffAll()

			gock.New(defaultApiEndpoint).
				Delete(apiKeyApiPath).
				Reply(tt.statusCode).
				JSON(tt.body)

			client, err := api.NewClientWithResponses(defaultApiEndpoint)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			state := ApiKeyResourceModel{
				ProjectRef: types.StringValue(testProjectRef),
				Id:         types.StringValue(testApiKeyUUID),
			}
			diags := deleteApiKey(t.Context(), &state, client)
			if diags.HasError() != tt.expectError {
				t.Errorf("expected error %v, got: %v", tt.expectError, diags)
			}
			if !gock.IsDone() {
				t.Fatalf("unconsumed mocks: %v", gock.Pending())
			}
		})
	}
}

func TestAccApiKeyResource_InvalidName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccApikeyResourceConfigInvalidName,
				ExpectError: regexp.MustCompile(`Name must start with a lowercase letter or an underscore`),
			},
		},
	})
}

func TestAccApiKeyResource_WithDescription(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(apiKeysApiPath).
		Reply(http.StatusOK).
		JSON([]api.ApiKeyResponse{
			{
				Name:   "anon",
				Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeLegacy),
				ApiKey: nullable.NewNullableWithValue("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.anon"),
			},
			{
				Name:   "service_role",
				Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeLegacy),
				ApiKey: nullable.NewNullableWithValue("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.service_role"),
			},
		})
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:     nullable.NewNullableWithValue(uuid.New().String()),
			Name:   "default",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
			ApiKey: nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"name":                "test",
			"type":                "secret",
			"description":         "Service key for test",
			"secret_jwt_template": map[string]any{"role": "service_role"},
		})).
		Reply(http.StatusCreated).
		JSON(api.ApiKeyResponse{
			Id:          nullable.NewNullableWithValue(testApiKeyUUID),
			Name:        "test",
			Type:        nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey:      nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			Description: nullable.NewNullableWithValue("Service key for test"),
		})
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(api.ApiKeyResponse{
			Id:          nullable.NewNullableWithValue(testApiKeyUUID),
			Name:        "test",
			Type:        nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey:      nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			Description: nullable.NewNullableWithValue("Service key for test"),
		})
	mockApiKeyRoles(map[string]string{testApiKeyUUID: "service_role"})
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccApikeyResourceConfigWithDescription,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "id", testApiKeyUUID),
					resource.TestCheckResourceAttr("supabase_apikey.new", "description", "Service key for test"),
				),
			},
		},
	})
}

func TestNullableDescription(t *testing.T) {
	t.Parallel()

	got := nullableDescription(types.StringNull())
	if got.IsSpecified() {
		t.Fatalf("expected unspecified nullable for null description, got %#v", got)
	}

	got = nullableDescription(types.StringValue("Service key for test"))
	if !got.IsSpecified() || got.IsNull() || got.MustGet() != "Service key for test" {
		t.Fatalf("expected nullable value for description, got %#v", got)
	}
}

// An imported key keeps its live description when configuration leaves
// description unset. Before description was Computed, the plan cleared it, the
// PATCH omitted the field, the API kept the old value and apply failed with
// "Provider produced inconsistent result".
func TestAccApiKeyResource_ImportKeepsDescription(t *testing.T) {
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(api.ApiKeyResponse{
			Id:          nullable.NewNullableWithValue(testApiKeyUUID),
			Name:        "test",
			Type:        nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			ApiKey:      nullable.NewNullableWithValue("sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			Description: nullable.NewNullableWithValue("Minted out of band"),
		})
	mockApiKeyRoles(map[string]string{testApiKeyUUID: "app_reader"})
	gock.New(defaultApiEndpoint).
		Delete(apiKeyApiPath).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             testAccApikeyResourceConfigCustomRoleNoDescription,
				ResourceName:       "supabase_apikey.new",
				ImportState:        true,
				ImportStateId:      testProjectRef + "/" + testApiKeyUUID,
				ImportStatePersist: true,
			},
			{
				Config: testAccApikeyResourceConfigCustomRoleNoDescription,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_apikey.new", "description", "Minted out of band"),
					resource.TestCheckResourceAttr("supabase_apikey.new", "secret_jwt_template.role", "app_reader"),
				),
			},
		},
	})
}

const testAccApikeyResourceConfigCustomRoleNoDescription = `
resource "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "test"
  secret_jwt_template = {
    role = "app_reader"
  }
}
`

func TestResolveAPIKeyImportID(t *testing.T) {
	knownID := uuid.New()
	otherID := uuid.New()

	tests := []struct {
		name string
		id   string
		mock func()

		expectProjectRef   string
		expectKeyID        string
		expectErrorSummary string
	}{
		{
			name:             "import by ID",
			id:               testProjectRef + "/" + knownID.String(),
			expectProjectRef: testProjectRef,
			expectKeyID:      knownID.String(),
		},
		{
			name:             "import by upper case ID",
			id:               testProjectRef + "/" + strings.ToUpper(knownID.String()),
			expectProjectRef: testProjectRef,
			expectKeyID:      knownID.String(),
		},
		{
			name: "import by name",
			id:   testProjectRef + "/mykey",
			mock: func() {
				gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).
					JSON([]api.ApiKeyResponse{{Id: nullable.NewNullableWithValue(knownID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret)}})
			},
			expectProjectRef: testProjectRef,
			expectKeyID:      knownID.String(),
		},
		{
			name: "import by name and type",
			id:   testProjectRef + "/mykey/secret",
			mock: func() {
				gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).
					JSON([]api.ApiKeyResponse{
						{Id: nullable.NewNullableWithValue(otherID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable)},
						{Id: nullable.NewNullableWithValue(knownID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret)},
					})
			},
			expectProjectRef: testProjectRef,
			expectKeyID:      knownID.String(),
		},
		{
			name: "import by name (ambiguous)",
			id:   testProjectRef + "/mykey",
			mock: func() {
				gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).
					JSON([]api.ApiKeyResponse{
						{Id: nullable.NewNullableWithValue(knownID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable)},
						{Id: nullable.NewNullableWithValue(otherID.String()), Name: "mykey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret)},
					})
			},
			expectErrorSummary: "Ambiguous Import Identifier",
		},
		{
			name: "key name not found",
			id:   testProjectRef + "/mykey",
			mock: func() {
				gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).
					JSON([]api.ApiKeyResponse{
						{Id: nullable.NewNullableWithValue(knownID.String()), Name: "knownkey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable)},
						{Id: nullable.NewNullableWithValue(otherID.String()), Name: "otherkey", Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret)},
					})
			},
			expectErrorSummary: "Import Error",
		},
		{
			name:               "import by name and bad type",
			id:                 testProjectRef + "/mykey/badtype",
			expectErrorSummary: "Unexpected Import Identifier",
		},
		{
			name:               "invalid import format",
			id:                 testProjectRef,
			expectErrorSummary: "Unexpected Import Identifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gock.InterceptClient(http.DefaultClient)
			defer gock.RestoreClient(http.DefaultClient)
			defer gock.OffAll()
			if tt.mock != nil {
				tt.mock()
			}

			client, err := api.NewClientWithResponses(defaultApiEndpoint)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			actualProjectRef, actualKeyID, diag := resolveAPIKeyImportID(t.Context(), client, tt.id)
			if tt.expectErrorSummary != "" {
				if diag == nil || diag.Summary() != tt.expectErrorSummary {
					t.Errorf("Expected error %q, got: %v", tt.expectErrorSummary, diag)
				}
				return
			}

			if diag != nil {
				t.Fatalf("Expected no error, got: %v", diag)
			}

			if tt.expectProjectRef != actualProjectRef {
				t.Errorf("Expected ref %q, got %q", tt.expectProjectRef, actualProjectRef)
			}
			if tt.expectKeyID != actualKeyID {
				t.Errorf("Expected id %q, got %q", tt.expectKeyID, actualKeyID)
			}
		})
	}
}
