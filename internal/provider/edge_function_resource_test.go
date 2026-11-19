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
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
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

	gock.New(defaultApiEndpoint).
		Get(fmt.Sprintf("/v1/projects/%s/functions/%s/body", projectRef, slug)).
		MatchHeader("Accept", "multipart/form-data").
		Reply(http.StatusOK).
		SetHeader("Content-Type", writer.FormDataContentType()).
		Body(&buf)
}

const testAccEdgeFunctionSlug = "test-function"

func testAccEdgeFunctionLiveVariables(organizationID, projectName, entrypoint string, name *string) config.Variables {
	vars := config.Variables{
		"organization_id": config.StringVariable(organizationID),
		"project_name":    config.StringVariable(projectName),
		"slug":            config.StringVariable(testAccEdgeFunctionSlug),
		"entrypoint":      config.StringVariable(entrypoint),
	}
	if name != nil {
		vars["name"] = config.StringVariable(*name)
	}
	return vars
}

func writeTestEntrypoint(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.ts")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("Failed to write entrypoint file: %v", err)
	}
	return path
}

func TestAccEdgeFunctionResource_basic(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_edge_function.test"
	projectName := acctest.RandomName()
	slug := testAccEdgeFunctionSlug
	entrypoint := writeTestEntrypoint(t, `Deno.serve((req) => new Response("Hello"));`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunction/basic/"),
				ConfigVariables: testAccEdgeFunctionLiveVariables(organizationID, projectName, entrypoint, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.CompareValuePairs(
						resourceName, tfjsonpath.New("project_ref"),
						projectResourceName, tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("slug"), knownvalue.StringExact(slug)),
					// name defaults to the slug when not specified
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(slug)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("entrypoint"), knownvalue.StringExact(entrypoint)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("import_map"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("static_files"), knownvalue.Null()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("status"), knownvalue.StringExact("ACTIVE")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("version"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("checksum"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("local_checksum"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("created_at"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("updated_at"), knownvalue.NotNull()),
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccEdgeFunctionResource_update(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	resourceName := "supabase_edge_function.test"
	projectName := acctest.RandomName()
	slug := testAccEdgeFunctionSlug
	entrypoint := writeTestEntrypoint(t, `Deno.serve((req) => new Response("Hello"));`)
	updatedEntrypoint := writeTestEntrypoint(t, `Deno.serve((req) => new Response("Updated"));`)
	updatedName := "Renamed function"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Create
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunction/basic/"),
				ConfigVariables: testAccEdgeFunctionLiveVariables(organizationID, projectName, entrypoint, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("version"), knownvalue.Int64Exact(1)),
				},
			},
			// Update entrypoint (new source deploys a new version)
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunction/basic/"),
				ConfigVariables: testAccEdgeFunctionLiveVariables(organizationID, projectName, updatedEntrypoint, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("entrypoint"), knownvalue.StringExact(updatedEntrypoint)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("version"), knownvalue.Int64Exact(2)),
				},
			},
			// Update name
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/EdgeFunction/basic/"),
				ConfigVariables: testAccEdgeFunctionLiveVariables(organizationID, projectName, updatedEntrypoint, &updatedName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("slug"), knownvalue.StringExact(slug)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(updatedName)),
				},
			},
		},
	})
}

// testAccEdgeFunctionFixtureDir returns the absolute path of the basic fixture so
// tests can t.Chdir (import downloads source relative to the working directory)
// without breaking the relative ConfigDirectory lookup.
func testAccEdgeFunctionFixtureDir(t *testing.T) config.TestStepConfigFunc {
	t.Helper()
	dir, err := filepath.Abs("test-fixtures/EdgeFunction/basic")
	if err != nil {
		t.Fatalf("Failed to resolve fixture dir: %v", err)
	}
	return config.StaticDirectory(dir)
}

func testAccEdgeFunctionImportStateID(projectResource, functionResource string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		project, ok := s.RootModule().Resources[projectResource]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", projectResource)
		}
		fn, ok := s.RootModule().Resources[functionResource]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", functionResource)
		}
		return fmt.Sprintf("%s/%s", project.Primary.ID, fn.Primary.Attributes["slug"]), nil
	}
}

func TestAccEdgeFunctionResource_import(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_edge_function.test"
	slug := testAccEdgeFunctionSlug
	entrypointContent := `Deno.serve((req) => new Response("Hello from import"));`
	entrypoint := writeTestEntrypoint(t, entrypointContent)
	fixture := testAccEdgeFunctionFixtureDir(t)
	vars := testAccEdgeFunctionLiveVariables(organizationID, acctest.RandomName(), entrypoint, nil)

	// chdir to a temp directory so downloaded files don't pollute the repo
	t.Chdir(t.TempDir())
	expectedEntrypoint := filepath.Join(".", "supabase", "functions", slug, "index.ts")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			{
				ConfigDirectory: fixture,
				ConfigVariables: vars,
			},
			// ImportState testing (downloads the deployed source)
			{
				ConfigDirectory:         fixture,
				ConfigVariables:         vars,
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       testAccEdgeFunctionImportStateID(projectResourceName, resourceName),
				ImportStateVerifyIgnore: []string{"entrypoint", "import_map", "static_files", "local_checksum"},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					if got := states[0].Attributes["entrypoint"]; got != expectedEntrypoint {
						return fmt.Errorf("entrypoint: got %q, want %q", got, expectedEntrypoint)
					}
					if _, ok := states[0].Attributes["import_map"]; ok {
						return fmt.Errorf("expected import_map to be unset")
					}
					content, err := os.ReadFile(expectedEntrypoint)
					if err != nil {
						return fmt.Errorf("expected entrypoint file at %s: %w", expectedEntrypoint, err)
					}
					if string(content) != entrypointContent {
						return fmt.Errorf("entrypoint content mismatch: got %q, want %q", content, entrypointContent)
					}
					return nil
				},
			},
		},
	})
}

func TestAccEdgeFunctionResource_importWithExistingFiles(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_edge_function.test"
	slug := testAccEdgeFunctionSlug
	originalContent := "// original content - must not be overwritten"
	entrypoint := writeTestEntrypoint(t, `Deno.serve((req) => new Response("Downloaded"));`)
	fixture := testAccEdgeFunctionFixtureDir(t)
	vars := testAccEdgeFunctionLiveVariables(organizationID, acctest.RandomName(), entrypoint, nil)

	t.Chdir(t.TempDir())

	// Pre-create the output file to trigger collision detection
	existingFile := filepath.Join(".", "supabase", "functions", slug, "index.ts")
	if err := os.MkdirAll(filepath.Dir(existingFile), 0o755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}
	if err := os.WriteFile(existingFile, []byte(originalContent), 0o600); err != nil {
		t.Fatalf("Failed to write existing file: %v", err)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			{
				ConfigDirectory: fixture,
				ConfigVariables: vars,
			},
			{
				ConfigDirectory:         fixture,
				ConfigVariables:         vars,
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       testAccEdgeFunctionImportStateID(projectResourceName, resourceName),
				ImportStateVerifyIgnore: []string{"entrypoint", "import_map", "static_files", "local_checksum"},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					if _, ok := states[0].Attributes["entrypoint"]; ok {
						return fmt.Errorf("expected entrypoint to be unset after a collision")
					}
					content, err := os.ReadFile(existingFile)
					if err != nil {
						return fmt.Errorf("existing file should still exist at %s: %w", existingFile, err)
					}
					if string(content) != originalContent {
						return fmt.Errorf("existing file was modified: got %q, want %q", content, originalContent)
					}
					matches, err := filepath.Glob(filepath.Join(".", "supabase", "functions", ".tf-import-*"))
					if err != nil {
						return fmt.Errorf("glob error: %w", err)
					}
					if len(matches) > 0 {
						return fmt.Errorf("temp directory residue found: %v", matches)
					}
					return nil
				},
			},
		},
	})
}

func TestAccEdgeFunctionResource_ImportFallback(t *testing.T) {
	defer gock.OffAll()

	testFunctionID := uuid.New().String()
	functionSlug := "fallback-func"
	fallbackFuncFunctionApiPath := functionsApiPath + "/" + functionSlug
	fallbackFuncBodyApiPath := fallbackFuncFunctionApiPath + "/body"

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
`, testProjectRef, functionSlug, entrypointPath)

	createdAt := int64(1234567890)
	updatedAt := int64(1234567890)

	gock.New(defaultApiEndpoint).
		Post(deployFunctionApiPath).
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

	gock.New(defaultApiEndpoint).
		Get(fallbackFuncFunctionApiPath).
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

	gock.New(defaultApiEndpoint).
		Get(fallbackFuncFunctionApiPath).
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

	gock.New(defaultApiEndpoint).
		Get(fallbackFuncFunctionApiPath).
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

	gock.New(defaultApiEndpoint).
		Get(fallbackFuncBodyApiPath).
		Reply(http.StatusInternalServerError).
		BodyString("Internal Server Error")

	gock.New(defaultApiEndpoint).
		Get(fallbackFuncFunctionApiPath).
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

	gock.New(defaultApiEndpoint).
		Delete(fallbackFuncFunctionApiPath).
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
				ImportStateId:           fmt.Sprintf("%s/%s", testProjectRef, functionSlug),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"entrypoint", "import_map", "static_files", "local_checksum"},
			},
		},
	})
}

func TestAccEdgeFunctionResource_ImportSlugDotDot(t *testing.T) {
	// Slug ".." should be rejected by validation before any API call.
	// No gock mocks needed since it fails at the slug validation stage.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "supabase_edge_function" "test" {
  project_ref = "proj"
  slug        = "x"
  entrypoint  = "x.ts"
}
`,
				ResourceName:      "supabase_edge_function.test",
				ImportState:       true,
				ImportStateId:     "proj/..",
				ExpectError:       regexp.MustCompile(`path separators or traversal segments`),
				ImportStateVerify: false,
			},
		},
	})
}

func TestAccEdgeFunctionResource_ImportSlugDot(t *testing.T) {
	// Slug "." should be rejected by validation before any API call.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "supabase_edge_function" "test" {
  project_ref = "proj"
  slug        = "x"
  entrypoint  = "x.ts"
}
`,
				ResourceName:      "supabase_edge_function.test",
				ImportState:       true,
				ImportStateId:     "proj/.",
				ExpectError:       regexp.MustCompile(`path separators or traversal segments`),
				ImportStateVerify: false,
			},
		},
	})
}

func TestAccEdgeFunctionResource_ImportDotDotEntrypoint(t *testing.T) {
	// When the bundle metadata returns entrypointPath="..", downloadFunctionSource
	// should reject it with the "invalid entrypoint path" error.
	// ImportState gracefully degrades to metadata-only import with a warning.
	defer gock.OffAll()

	testFunctionID := uuid.New().String()
	functionSlug := "dotdot-ep"
	dotDotEpFunctionApiPath := functionsApiPath + "/" + functionSlug

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
resource "supabase_edge_function" "dotdot" {
  project_ref = "%s"
  slug        = "%s"
  entrypoint  = "%s"
}
`, testProjectRef, functionSlug, entrypointPath)

	createdAt := int64(1234567890)
	updatedAt := int64(1234567890)

	gock.New(defaultApiEndpoint).
		Post(deployFunctionApiPath).
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

	gock.New(defaultApiEndpoint).
		Get(dotDotEpFunctionApiPath).
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

	gock.New(defaultApiEndpoint).
		Get(dotDotEpFunctionApiPath).
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

	gock.New(defaultApiEndpoint).
		Get(dotDotEpFunctionApiPath).
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

	mockMultipartBodyResponse(t, testProjectRef, functionSlug, "..", map[string]string{
		"..": `Deno.serve((req) => new Response("Hello"));`,
	})

	gock.New(defaultApiEndpoint).
		Get(dotDotEpFunctionApiPath).
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

	gock.New(defaultApiEndpoint).
		Delete(dotDotEpFunctionApiPath).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function.dotdot", "id", testFunctionID),
				),
			},
			{
				ResourceName:            "supabase_edge_function.dotdot",
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("%s/%s", testProjectRef, functionSlug),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"entrypoint", "import_map", "static_files", "local_checksum"},
			},
		},
	})
}
