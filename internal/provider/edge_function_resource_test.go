// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestAccEdgeFunctionResource(t *testing.T) {
	// Setup mock api
	defer gock.OffAll()

	functionBody := "export default async () => new Response('Hello World')"
	encodedBody := base64.StdEncoding.EncodeToString([]byte(functionBody))

	// Step 1: create
	gock.New("https://api.supabase.com").
		Post("/v1/projects/test-project/functions").
		Reply(http.StatusCreated).
		JSON(api.FunctionResponse{
			Id:      "test-function-id",
			Slug:    "test-function",
			Name:    "Test Function",
			Version: 1,
			Status:  api.FunctionResponseStatusACTIVE,
		})

	// Step 2: read
	gock.New("https://api.supabase.com").
		Get("/v1/projects/test-project/functions/test-function").
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        "test-function-id",
			Slug:      "test-function",
			Name:      "Test Function",
			Version:   1,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			VerifyJwt: Ptr(true),
		})

	// Step 3: read again
	gock.New("https://api.supabase.com").
		Get("/v1/projects/test-project/functions/test-function").
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        "test-function-id",
			Slug:      "test-function",
			Name:      "Test Function",
			Version:   1,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			VerifyJwt: Ptr(true),
		})

	// Step 4: update
	gock.New("https://api.supabase.com").
		Get("/v1/projects/test-project/functions/test-function").
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        "test-function-id",
			Slug:      "test-function",
			Name:      "Test Function Updated",
			Version:   1,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			VerifyJwt: Ptr(false),
		})

	gock.New("https://api.supabase.com").
		Patch("/v1/projects/test-project/functions/test-function").
		Reply(http.StatusOK).
		JSON(api.FunctionResponse{
			Id:      "test-function-id",
			Slug:    "test-function",
			Name:    "Test Function Updated",
			Version: 2,
			Status:  api.FunctionResponseStatusACTIVE,
		})

	// Step 5: delete
	gock.New("https://api.supabase.com").
		Delete("/v1/projects/test-project/functions/test-function").
		Reply(http.StatusNoContent)

	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccEdgeFunctionResourceConfig(encodedBody),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function.test", "slug", "test-function"),
					resource.TestCheckResourceAttr("supabase_edge_function.test", "name", "Test Function"),
					resource.TestCheckResourceAttr("supabase_edge_function.test", "verify_jwt", "true"),
					resource.TestCheckResourceAttr("supabase_edge_function.test", "version", "1"),
					resource.TestCheckResourceAttr("supabase_edge_function.test", "status", "ACTIVE"),
				),
			},
			// Update and Read testing
			{
				Config: testAccEdgeFunctionResourceConfigUpdated(encodedBody),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function.test", "name", "Test Function Updated"),
					resource.TestCheckResourceAttr("supabase_edge_function.test", "verify_jwt", "false"),
					resource.TestCheckResourceAttr("supabase_edge_function.test", "version", "2"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccEdgeFunctionResourceConfig(encodedBody string) string {
	return `
resource "supabase_edge_function" "test" {
  project_ref = "test-project"
  slug        = "test-function"
  name        = "Test Function"
  body        = "` + encodedBody + `"
  verify_jwt  = true
  import_map  = false
}
`
}

func testAccEdgeFunctionResourceConfigUpdated(encodedBody string) string {
	return `
resource "supabase_edge_function" "test" {
  project_ref = "test-project"
  slug        = "test-function"
  name        = "Test Function Updated"
  body        = "` + encodedBody + `"
  verify_jwt  = false
  import_map  = false
}
`
}
