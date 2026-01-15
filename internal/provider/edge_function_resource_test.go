// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestAccEdgeFunctionResource(t *testing.T) {
	defer gock.OffAll()

	testFunctionID := uuid.New().String()
	projectRef := "mayuaycdtijbctgqbycg"
	functionSlug := "foo"

	// Create a temp directory with the entrypoint file
	tmpDir := t.TempDir()
	entrypointPath := filepath.Join(tmpDir, "index.ts")
	if err := os.WriteFile(entrypointPath, []byte(`Deno.serve((req) => new Response("Hello"));`), 0o600); err != nil {
		t.Fatalf("Failed to write entrypoint file: %v", err)
	}

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function" "foo" {
  project_ref = "%s"
  slug        = "%s"
  entrypoint  = "%s"
}
`, projectRef, functionSlug, entrypointPath)

	// Step 1: Create - Deploy function
	createdAt := int64(1234567890)
	updatedAt := int64(1234567890)
	gock.New("https://api.supabase.com").
		Post(fmt.Sprintf("/v1/projects/%s/functions/deploy", projectRef)).
		Reply(http.StatusCreated).
		JSON(api.DeployFunctionResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.DeployFunctionResponseStatusACTIVE,
			Version:   1,
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		})

	// Step 1: Read after create
	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   1,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567890,
		})

	// Step 2: Read for refresh
	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   1,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567890,
		})

	// Step 3: Delete
	gock.New("https://api.supabase.com").
		Delete(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "id", testFunctionID),
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "slug", functionSlug),
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "name", functionSlug),
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "status", "ACTIVE"),
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "version", "1"),
					resource.TestCheckResourceAttrSet("supabase_edge_function.foo", "local_checksum"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccEdgeFunctionResource_Update(t *testing.T) {
	defer gock.OffAll()

	testFunctionID := uuid.New().String()
	projectRef := "mayuaycdtijbctgqbycg"
	functionSlug := "foo"

	tmpDir := t.TempDir()
	entrypointPath := filepath.Join(tmpDir, "index.ts")
	if err := os.WriteFile(entrypointPath, []byte(`Deno.serve((req) => new Response("Hello"));`), 0o600); err != nil {
		t.Fatalf("Failed to write entrypoint file: %v", err)
	}

	entrypointPath2 := filepath.Join(tmpDir, "index2.ts")
	if err := os.WriteFile(entrypointPath2, []byte(`Deno.serve((req) => new Response("Updated"));`), 0o600); err != nil {
		t.Fatalf("Failed to write entrypoint file 2: %v", err)
	}

	testConfig1 := fmt.Sprintf(`
resource "supabase_edge_function" "foo" {
  project_ref = "%s"
  slug        = "%s"
  entrypoint  = "%s"
}
`, projectRef, functionSlug, entrypointPath)

	testConfig2 := fmt.Sprintf(`
resource "supabase_edge_function" "foo" {
  project_ref = "%s"
  slug        = "%s"
  entrypoint  = "%s"
}
`, projectRef, functionSlug, entrypointPath2)

	createdAt := int64(1234567890)
	updatedAt := int64(1234567890)
	updatedAt2 := int64(1234567999)

	gock.New("https://api.supabase.com").
		Post(fmt.Sprintf("/v1/projects/%s/functions/deploy", projectRef)).
		Reply(http.StatusCreated).
		JSON(api.DeployFunctionResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.DeployFunctionResponseStatusACTIVE,
			Version:   1,
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   1,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567890,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   1,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567890,
		})

	gock.New("https://api.supabase.com").
		Post(fmt.Sprintf("/v1/projects/%s/functions/deploy", projectRef)).
		Reply(http.StatusCreated).
		JSON(api.DeployFunctionResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.DeployFunctionResponseStatusACTIVE,
			Version:   2,
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt2,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   2,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567999,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   2,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567999,
		})

	gock.New("https://api.supabase.com").
		Delete(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testConfig1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "id", testFunctionID),
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "version", "1"),
				),
			},
			{
				Config: testConfig2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "id", testFunctionID),
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "version", "2"),
				),
			},
		},
	})
}

func TestAccEdgeFunctionResource_Import(t *testing.T) {
	defer gock.OffAll()

	testFunctionID := uuid.New().String()
	projectRef := "mayuaycdtijbctgqbycg"
	functionSlug := "foo"

	tmpDir := t.TempDir()
	entrypointPath := filepath.Join(tmpDir, "index.ts")
	if err := os.WriteFile(entrypointPath, []byte(`Deno.serve((req) => new Response("Hello"));`), 0o600); err != nil {
		t.Fatalf("Failed to write entrypoint file: %v", err)
	}

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function" "foo" {
  project_ref = "%s"
  slug        = "%s"
  entrypoint  = "%s"
}
`, projectRef, functionSlug, entrypointPath)

	createdAt := int64(1234567890)
	updatedAt := int64(1234567890)

	gock.New("https://api.supabase.com").
		Post(fmt.Sprintf("/v1/projects/%s/functions/deploy", projectRef)).
		Reply(http.StatusCreated).
		JSON(api.DeployFunctionResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.DeployFunctionResponseStatusACTIVE,
			Version:   1,
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   1,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567890,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   1,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567890,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   1,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567890,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:        testFunctionID,
			Slug:      functionSlug,
			Name:      functionSlug,
			Status:    api.FunctionSlugResponseStatusACTIVE,
			Version:   1,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567890,
		})

	gock.New("https://api.supabase.com").
		Delete(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function.foo", "id", testFunctionID),
				),
			},
			{
				ResourceName:            "supabase_edge_function.foo",
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("%s/%s", projectRef, functionSlug),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"entrypoint", "import_map", "static_files", "local_checksum"},
			},
		},
	})
}
