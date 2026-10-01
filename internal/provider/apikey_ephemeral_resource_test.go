// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

const (
	testAPIKeySecret = "sb_secret_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	// Provider configuration is a valid ephemeral consumer. The key is not
	// assigned to a managed resource, which would persist it in state.
	testAPIKeyEcho = `
provider "echo" {
  data = ephemeral.supabase_apikey.new.api_key
}
`
)

const testAccApiKeyEphemeralInvalidName = `
ephemeral "supabase_apikey" "new" {
  project_ref = "` + testProjectRef + `"
  name        = "Invalid-Name-123"
}
`

func TestAccApiKeyEphemeralResource(t *testing.T) {
	defer gock.OffAll()

	secretKey := revealedAPIKeyResponse("")

	// Plan opens the resource and creates the key. Apply opens it again and
	// reveals the key created during plan. The echo provider configuration is
	// what references the key; the secret is not written to managed state.
	gock.New(defaultApiEndpoint).
		Get(apiKeyApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(secretKey)
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
			Id:     nullable.NewNullableWithValue("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			Name:   "default",
			Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
			ApiKey: nullable.NewNullableWithValue("sb_publishable_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
		})
	gock.New(defaultApiEndpoint).
		Post(apiKeysApiPath).
		Reply(http.StatusCreated).
		JSON(secretKey)
	// Later opens, including the post-apply plan, reuse the key created above.
	gock.New(defaultApiEndpoint).
		Get(apiKeysApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON([]api.ApiKeyResponse{
			{
				Id:   nullable.NewNullableWithValue("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
				Name: "default",
				Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
			},
			secretKey,
		})

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"supabase": testAccProtoV6ProviderFactories["supabase"],
			"echo":     echoprovider.NewProviderServer(),
		},
		Steps: []resource.TestStep{
			{
				Config: examples.ApiKeyEphemeralResourceConfig + testAPIKeyEcho,
			},
		},
	})

	for _, pending := range gock.Pending() {
		if pending.Request().Method == http.MethodPost {
			t.Fatalf("ephemeral open did not create the API key, pending: %+v", gock.Pending())
		}
	}
}

func TestAccApiKeyEphemeralResource_InvalidName(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccApiKeyEphemeralInvalidName,
				ExpectError: regexp.MustCompile(`Name must start with a lowercase letter or an underscore`),
			},
		},
	})
}

func TestOpenAPIKey_CreatesWhenMissing(t *testing.T) {
	client := mockAPIKeyClient(t)
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).JSON([]api.ApiKeyResponse{})
	gock.New(defaultApiEndpoint).Post(apiKeysApiPath).Reply(http.StatusCreated).JSON(api.ApiKeyResponse{
		Id:   nullable.NewNullableWithValue("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Name: "default",
		Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
	})
	gock.New(defaultApiEndpoint).Post(apiKeysApiPath).Reply(http.StatusCreated).JSON(revealedAPIKeyResponse(""))
	gock.New(defaultApiEndpoint).Get(apiKeyApiPath).Reply(http.StatusOK).JSON(revealedAPIKeyResponse("created"))

	data := ApiKeyResourceModel{
		ProjectRef:  types.StringValue(testProjectRef),
		Name:        types.StringValue("test"),
		Description: types.StringValue("created"),
	}
	if diags := openAPIKey(t.Context(), &data, client); diags.HasError() {
		t.Fatalf("open api key: %v", diags)
	}
	if data.Id.ValueString() != testApiKeyUUID {
		t.Errorf("id %q, want %q", data.Id.ValueString(), testApiKeyUUID)
	}
	if data.ApiKey.ValueString() != testAPIKeySecret {
		t.Errorf("api_key %q, want %q", data.ApiKey.ValueString(), testAPIKeySecret)
	}
	if data.Description.ValueString() != "created" {
		t.Errorf("description %q, want created", data.Description.ValueString())
	}
	assertAPIKeyRole(t, data, "service_role")
	if !gock.IsDone() {
		t.Errorf("pending mocks: %+v", gock.Pending())
	}
}

func TestOpenAPIKey_RevealsExisting(t *testing.T) {
	client := mockAPIKeyClient(t)
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).JSON([]api.ApiKeyResponse{
		{
			Id:   nullable.NewNullableWithValue("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			Name: "default",
			Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
		},
		revealedAPIKeyResponse("kept"),
	})
	gock.New(defaultApiEndpoint).Get(apiKeyApiPath).Reply(http.StatusOK).JSON(revealedAPIKeyResponse("kept"))

	data := ApiKeyResourceModel{
		ProjectRef:  types.StringValue(testProjectRef),
		Name:        types.StringValue("test"),
		Description: types.StringNull(),
	}
	if diags := openAPIKey(t.Context(), &data, client); diags.HasError() {
		t.Fatalf("open api key: %v", diags)
	}
	if data.ApiKey.ValueString() != testAPIKeySecret {
		t.Errorf("api_key %q, want %q", data.ApiKey.ValueString(), testAPIKeySecret)
	}
	if data.Description.ValueString() != "kept" {
		t.Errorf("description %q, want kept", data.Description.ValueString())
	}
	if !gock.IsDone() {
		t.Errorf("pending mocks: %+v", gock.Pending())
	}
}

func TestOpenAPIKey_UpdatesDescription(t *testing.T) {
	client := mockAPIKeyClient(t)
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).JSON([]api.ApiKeyResponse{
		{
			Id:   nullable.NewNullableWithValue("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			Name: "default",
			Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
		},
		revealedAPIKeyResponse("old"),
	})
	gock.New(defaultApiEndpoint).
		Patch(apiKeyApiPath).
		JSON(map[string]string{"description": "rotated"}).
		Reply(http.StatusOK).
		JSON(revealedAPIKeyResponse("rotated"))
	gock.New(defaultApiEndpoint).Get(apiKeyApiPath).Reply(http.StatusOK).JSON(revealedAPIKeyResponse("rotated"))

	data := ApiKeyResourceModel{
		ProjectRef:  types.StringValue(testProjectRef),
		Name:        types.StringValue("test"),
		Description: types.StringValue("rotated"),
	}
	if diags := openAPIKey(t.Context(), &data, client); diags.HasError() {
		t.Fatalf("open api key: %v", diags)
	}
	if data.Description.ValueString() != "rotated" {
		t.Errorf("description %q, want rotated", data.Description.ValueString())
	}
	if data.ApiKey.ValueString() != testAPIKeySecret {
		t.Errorf("api_key %q, want %q", data.ApiKey.ValueString(), testAPIKeySecret)
	}
	if !gock.IsDone() {
		t.Errorf("pending mocks: %+v", gock.Pending())
	}
}

func TestOpenAPIKey_ConcurrentOpensCreateOneKey(t *testing.T) {
	client := mockAPIKeyClient(t)
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).JSON([]api.ApiKeyResponse{})
	gock.New(defaultApiEndpoint).Post(apiKeysApiPath).Reply(http.StatusCreated).JSON(api.ApiKeyResponse{
		Id:   nullable.NewNullableWithValue("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Name: "default",
		Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
	})
	gock.New(defaultApiEndpoint).Post(apiKeysApiPath).Reply(http.StatusCreated).JSON(revealedAPIKeyResponse(""))
	gock.New(defaultApiEndpoint).Get(apiKeyApiPath).Reply(http.StatusOK).JSON(revealedAPIKeyResponse("created"))
	gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).JSON([]api.ApiKeyResponse{
		{
			Id:   nullable.NewNullableWithValue("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			Name: "default",
			Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
		},
		revealedAPIKeyResponse("created"),
	})
	gock.New(defaultApiEndpoint).Get(apiKeyApiPath).Reply(http.StatusOK).JSON(revealedAPIKeyResponse("created"))

	var wg sync.WaitGroup
	results := make([]ApiKeyResourceModel, 2)
	errs := make([]diag.Diagnostics, 2)
	start := make(chan struct{})
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			data := ApiKeyResourceModel{
				ProjectRef:  types.StringValue(testProjectRef),
				Name:        types.StringValue("test"),
				Description: types.StringValue("created"),
			}
			errs[i] = openAPIKey(t.Context(), &data, client)
			results[i] = data
		}()
	}
	close(start)
	wg.Wait()

	for i, diags := range errs {
		if diags.HasError() {
			t.Fatalf("open %d: %v", i, diags)
		}
		if results[i].Id.ValueString() != testApiKeyUUID {
			t.Errorf("open %d id %q, want %q", i, results[i].Id.ValueString(), testApiKeyUUID)
		}
	}
	if !gock.IsDone() {
		t.Errorf("pending mocks: %+v", gock.Pending())
	}
}

func TestOpenAPIKey_InvalidID(t *testing.T) {
	client := mockAPIKeyClient(t)
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).JSON([]api.ApiKeyResponse{
		{
			Id:   nullable.NewNullableWithValue("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			Name: "default",
			Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypePublishable),
		},
		{
			Id:          nullable.NewNullableWithValue("not-a-uuid"),
			Name:        "test",
			Type:        nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
			Description: nullable.NewNullableWithValue("old"),
		},
	})

	data := ApiKeyResourceModel{
		ProjectRef:  types.StringValue(testProjectRef),
		Name:        types.StringValue("test"),
		Description: types.StringValue("rotated"),
	}
	diags := openAPIKey(t.Context(), &data, client)
	if !diags.HasError() || diags[0].Detail() != `API key id "not-a-uuid" is not a UUID.` {
		t.Fatalf("expected invalid id diagnostic, got %v", diags)
	}
	if !gock.IsDone() {
		t.Errorf("pending mocks: %+v", gock.Pending())
	}
}

func TestOpenAPIKey_AmbiguousName(t *testing.T) {
	client := mockAPIKeyClient(t)
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).Get(apiKeysApiPath).Reply(http.StatusOK).JSON([]api.ApiKeyResponse{
		revealedAPIKeyResponse(""),
		{
			Id:   nullable.NewNullableWithValue("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Name: "test",
			Type: nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
		},
	})

	data := ApiKeyResourceModel{
		ProjectRef:  types.StringValue(testProjectRef),
		Name:        types.StringValue("test"),
		Description: types.StringNull(),
	}
	diags := openAPIKey(t.Context(), &data, client)
	if !diags.HasError() || diags[0].Summary() != "Ambiguous API Key" {
		t.Fatalf("expected ambiguous API key error, got %v", diags)
	}
}

func mockAPIKeyClient(t *testing.T) *api.ClientWithResponses {
	t.Helper()
	gock.InterceptClient(http.DefaultClient)
	t.Cleanup(func() { gock.RestoreClient(http.DefaultClient) })

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return client
}

func revealedAPIKeyResponse(description string) api.ApiKeyResponse {
	resp := api.ApiKeyResponse{
		Id:     nullable.NewNullableWithValue(testApiKeyUUID),
		Name:   "test",
		Type:   nullable.NewNullableWithValue(api.ApiKeyResponseTypeSecret),
		ApiKey: nullable.NewNullableWithValue(testAPIKeySecret),
		SecretJwtTemplate: nullable.NewNullableWithValue(map[string]interface{}{
			"role": "service_role",
		}),
	}
	if description != "" {
		resp.Description = nullable.NewNullableWithValue(description)
	}
	return resp
}

func assertAPIKeyRole(t *testing.T, data ApiKeyResourceModel, want string) {
	t.Helper()
	role, ok := data.SecretJwtTemplate.Attributes()["role"].(types.String)
	if !ok {
		t.Fatalf("role attribute has type %T", data.SecretJwtTemplate.Attributes()["role"])
	}
	if role.ValueString() != want {
		t.Errorf("role %q, want %q", role.ValueString(), want)
	}
}
