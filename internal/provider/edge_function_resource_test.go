// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func mockMultipartBodyResponse(t *testing.T, projectRef, slug, entrypointPath string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	headers := textproto.MIMEHeader{}
	headers.Set("Content-Disposition", `form-data; name="metadata"`)
	headers.Set("Content-Type", "application/json")
	pw, err := writer.CreatePart(headers)
	if err != nil {
		t.Fatalf("Failed to create metadata part: %v", err)
	}
	meta := bundleMetadata{EntrypointPath: entrypointPath}
	if err := json.NewEncoder(pw).Encode(meta); err != nil {
		t.Fatalf("Failed to encode metadata: %v", err)
	}

	for filename, content := range files {
		fh := textproto.MIMEHeader{}
		fh.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
		pw, err := writer.CreatePart(fh)
		if err != nil {
			t.Fatalf("Failed to create file part: %v", err)
		}
		if _, err := pw.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close multipart writer: %v", err)
	}

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s/body", projectRef, slug)).
		MatchHeader("Accept", "multipart/form-data").
		Reply(http.StatusOK).
		SetHeader("Content-Type", writer.FormDataContentType()).
		Body(&buf)
}

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

func TestAccEdgeFunctionResource_ImportWithDownload(t *testing.T) {
	defer gock.OffAll()

	testFunctionID := uuid.New().String()
	projectRef := "mayuaycdtijbctgqbycg"
	functionSlug := "hello-world"
	entrypointContent := `Deno.serve((req) => new Response("Hello from import"));`

	// chdir to temp directory so downloaded files don't pollute the repo
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	entrypointPath := filepath.Join(tmpDir, "deploy-src", "index.ts")
	if err := os.MkdirAll(filepath.Dir(entrypointPath), 0o755); err != nil {
		t.Fatalf("Failed to create deploy-src dir: %v", err)
	}
	if err := os.WriteFile(entrypointPath, []byte(entrypointContent), 0o600); err != nil {
		t.Fatalf("Failed to write entrypoint file: %v", err)
	}

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function" "test" {
  project_ref = "%s"
  slug        = "%s"
  entrypoint  = "%s"
}
`, projectRef, functionSlug, entrypointPath)

	createdAt := int64(1234567890)
	updatedAt := int64(1234567890)
	apiEntrypoint := "file:///src/index.ts"

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
			Id:             testFunctionID,
			Slug:           functionSlug,
			Name:           functionSlug,
			Status:         api.FunctionSlugResponseStatusACTIVE,
			Version:        1,
			CreatedAt:      1234567890,
			UpdatedAt:      1234567890,
			EntrypointPath: &apiEntrypoint,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:             testFunctionID,
			Slug:           functionSlug,
			Name:           functionSlug,
			Status:         api.FunctionSlugResponseStatusACTIVE,
			Version:        1,
			CreatedAt:      1234567890,
			UpdatedAt:      1234567890,
			EntrypointPath: &apiEntrypoint,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:             testFunctionID,
			Slug:           functionSlug,
			Name:           functionSlug,
			Status:         api.FunctionSlugResponseStatusACTIVE,
			Version:        1,
			CreatedAt:      1234567890,
			UpdatedAt:      1234567890,
			EntrypointPath: &apiEntrypoint,
		})

	mockMultipartBodyResponse(t, projectRef, functionSlug, "/src/index.ts", map[string]string{
		"/src/index.ts": entrypointContent,
	})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:             testFunctionID,
			Slug:           functionSlug,
			Name:           functionSlug,
			Status:         api.FunctionSlugResponseStatusACTIVE,
			Version:        1,
			CreatedAt:      1234567890,
			UpdatedAt:      1234567890,
			EntrypointPath: &apiEntrypoint,
		})

	gock.New("https://api.supabase.com").
		Delete(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK)

	expectedEntrypoint := filepath.Join(".", "supabase", "functions", functionSlug, "index.ts")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function.test", "id", testFunctionID),
				),
			},
			{
				ResourceName:  "supabase_edge_function.test",
				ImportState:   true,
				ImportStateId: fmt.Sprintf("%s/%s", projectRef, functionSlug),
				// Can't use ImportStateVerify because entrypoint path differs between
				// deployed (absolute tmp path) and imported (relative ./supabase/functions/...)
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"entrypoint", "import_map", "static_files", "local_checksum"},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function.test", "entrypoint", expectedEntrypoint),
					resource.TestCheckNoResourceAttr("supabase_edge_function.test", "import_map"),
					// Verify file was actually written to disk
					func(s *terraform.State) error {
						content, err := os.ReadFile(expectedEntrypoint)
						if err != nil {
							return fmt.Errorf("expected entrypoint file at %s: %v", expectedEntrypoint, err)
						}
						if string(content) != entrypointContent {
							return fmt.Errorf("entrypoint content mismatch: got %q, want %q", string(content), entrypointContent)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccEdgeFunctionResource_ImportFallback(t *testing.T) {
	defer gock.OffAll()

	testFunctionID := uuid.New().String()
	projectRef := "mayuaycdtijbctgqbycg"
	functionSlug := "fallback-func"

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	entrypointPath := filepath.Join(tmpDir, "deploy-src", "index.ts")
	if err := os.MkdirAll(filepath.Dir(entrypointPath), 0o755); err != nil {
		t.Fatalf("Failed to create deploy-src dir: %v", err)
	}
	if err := os.WriteFile(entrypointPath, []byte(`Deno.serve((req) => new Response("Hello"));`), 0o600); err != nil {
		t.Fatalf("Failed to write entrypoint file: %v", err)
	}

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function" "fallback" {
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
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s/body", projectRef, functionSlug)).
		Reply(http.StatusInternalServerError).
		BodyString("Internal Server Error")

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
					resource.TestCheckResourceAttr("supabase_edge_function.fallback", "id", testFunctionID),
				),
			},
			{
				ResourceName:            "supabase_edge_function.fallback",
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("%s/%s", projectRef, functionSlug),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"entrypoint", "import_map", "static_files", "local_checksum"},
			},
		},
	})
}

func TestAccEdgeFunctionResource_ImportWithExistingFiles(t *testing.T) {
	defer gock.OffAll()

	testFunctionID := uuid.New().String()
	projectRef := "mayuaycdtijbctgqbycg"
	functionSlug := "collision-func"
	originalContent := "// original content - must not be overwritten"
	downloadedContent := `Deno.serve((req) => new Response("Downloaded"));`

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Pre-create the output file to trigger collision detection
	existingFile := filepath.Join(".", "supabase", "functions", functionSlug, "index.ts")
	if err := os.MkdirAll(filepath.Dir(existingFile), 0o755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}
	if err := os.WriteFile(existingFile, []byte(originalContent), 0o600); err != nil {
		t.Fatalf("Failed to write existing file: %v", err)
	}

	entrypointPath := filepath.Join(tmpDir, "deploy-src", "index.ts")
	if err := os.MkdirAll(filepath.Dir(entrypointPath), 0o755); err != nil {
		t.Fatalf("Failed to create deploy-src dir: %v", err)
	}
	if err := os.WriteFile(entrypointPath, []byte(downloadedContent), 0o600); err != nil {
		t.Fatalf("Failed to write entrypoint file: %v", err)
	}

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function" "collision" {
  project_ref = "%s"
  slug        = "%s"
  entrypoint  = "%s"
}
`, projectRef, functionSlug, entrypointPath)

	createdAt := int64(1234567890)
	updatedAt := int64(1234567890)
	apiEntrypoint := "file:///src/index.ts"

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
			Id:             testFunctionID,
			Slug:           functionSlug,
			Name:           functionSlug,
			Status:         api.FunctionSlugResponseStatusACTIVE,
			Version:        1,
			CreatedAt:      1234567890,
			UpdatedAt:      1234567890,
			EntrypointPath: &apiEntrypoint,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:             testFunctionID,
			Slug:           functionSlug,
			Name:           functionSlug,
			Status:         api.FunctionSlugResponseStatusACTIVE,
			Version:        1,
			CreatedAt:      1234567890,
			UpdatedAt:      1234567890,
			EntrypointPath: &apiEntrypoint,
		})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:             testFunctionID,
			Slug:           functionSlug,
			Name:           functionSlug,
			Status:         api.FunctionSlugResponseStatusACTIVE,
			Version:        1,
			CreatedAt:      1234567890,
			UpdatedAt:      1234567890,
			EntrypointPath: &apiEntrypoint,
		})

	mockMultipartBodyResponse(t, projectRef, functionSlug, "/src/index.ts", map[string]string{
		"/src/index.ts": downloadedContent,
	})

	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s", projectRef, functionSlug)).
		Reply(http.StatusOK).
		JSON(api.FunctionSlugResponse{
			Id:             testFunctionID,
			Slug:           functionSlug,
			Name:           functionSlug,
			Status:         api.FunctionSlugResponseStatusACTIVE,
			Version:        1,
			CreatedAt:      1234567890,
			UpdatedAt:      1234567890,
			EntrypointPath: &apiEntrypoint,
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
					resource.TestCheckResourceAttr("supabase_edge_function.collision", "id", testFunctionID),
				),
			},
			{
				ResourceName:            "supabase_edge_function.collision",
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("%s/%s", projectRef, functionSlug),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"entrypoint", "import_map", "static_files", "local_checksum"},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("supabase_edge_function.collision", "entrypoint"),
					func(s *terraform.State) error {
						content, err := os.ReadFile(existingFile)
						if err != nil {
							return fmt.Errorf("existing file should still exist at %s: %v", existingFile, err)
						}
						if string(content) != originalContent {
							return fmt.Errorf("existing file was modified: got %q, want %q", string(content), originalContent)
						}
						return nil
					},
					func(s *terraform.State) error {
						parentDir := filepath.Join(".", "supabase", "functions")
						matches, err := filepath.Glob(filepath.Join(parentDir, ".tf-import-*"))
						if err != nil {
							return fmt.Errorf("glob error: %v", err)
						}
						if len(matches) > 0 {
							return fmt.Errorf("temp directory residue found: %v", matches)
						}
						return nil
					},
				),
			},
		},
	})
}
