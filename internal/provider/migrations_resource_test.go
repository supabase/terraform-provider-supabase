package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestAccMigrationsResource(t *testing.T) {
	// Test basic creation of migrations resource
	defer gock.OffAll()

	// Create temporary migration files for testing
	tmpDir := t.TempDir()
	migration1Path := filepath.Join(tmpDir, "001_initial.sql")
	migration1Content := "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);"
	err := os.WriteFile(migration1Path, []byte(migration1Content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	testConfig := fmt.Sprintf(`
resource "supabase_migrations" "test" {
	project_ref = "%s"
	migrations_dir = "%s"
}
`, testProjectRef, tmpDir)

	// Mock project status check
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		AddMatcher(exactPathMatcher(projectApiPath)).
		Times(3). // Called during wait, create, and read
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})

	// Mock apply migration
	gock.New(defaultApiEndpoint).
		Post(migrationsApiPath).
		Reply(http.StatusOK)

	// Mock migration history read used by resource Read
	gock.New(defaultApiEndpoint).
		Get(migrationsApiPath).
		Reply(http.StatusOK).
		AddHeader("Content-Type", "application/json").
		BodyString(`[{"name":"001_initial.sql","version":"001"}]`)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_migrations.test", "project_ref", testProjectRef),
					testresource.TestCheckResourceAttr("supabase_migrations.test", "migrations.#", "1"),
					testresource.TestCheckResourceAttr("supabase_migrations.test", "migrations.0.name", "001_initial.sql"),
					testresource.TestCheckResourceAttrSet("supabase_migrations.test", "migrations.0.digest"),
				),
			},
		},
	})
}

func TestAccMigrationsResource_AppendUpdate(t *testing.T) {
	// Test appending new migrations (valid update)
	defer gock.OffAll()

	tmpDir := t.TempDir()

	// First migration
	migration1Path := filepath.Join(tmpDir, "001_initial.sql")
	migration1Content := "CREATE TABLE users (id SERIAL PRIMARY KEY);"
	err := os.WriteFile(migration1Path, []byte(migration1Content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Second migration
	migration2Path := filepath.Join(tmpDir, "002_add_column.sql")
	migration2Content := "ALTER TABLE users ADD COLUMN email TEXT;"

	config := fmt.Sprintf(`
resource "supabase_migrations" "test" {
	project_ref = "%s"
	migrations_dir = "%s"
}
`, testProjectRef, tmpDir)

	// Mock project status checks
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		AddMatcher(exactPathMatcher(projectApiPath)).
		Persist().
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})

	// Mock apply migrations
	gock.New(defaultApiEndpoint).
		Post(migrationsApiPath).
		Persist().
		Reply(http.StatusOK)

	// Mock migration history reads:
	// first reads return the initial single migration, subsequent reads return both migrations.
	gock.New(defaultApiEndpoint).
		Get(migrationsApiPath).
		Times(2).
		Reply(http.StatusOK).
		AddHeader("Content-Type", "application/json").
		BodyString(`[{"name":"001_initial.sql","version":"001"}]`)

	gock.New(defaultApiEndpoint).
		Get(migrationsApiPath).
		Persist().
		Reply(http.StatusOK).
		AddHeader("Content-Type", "application/json").
		BodyString(`[{"name":"001_initial.sql","version":"001"},{"name":"002_add_column.sql","version":"002"}]`)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: config,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_migrations.test", "migrations.#", "1"),
				),
			},
			{
				PreConfig: func() {
					err := os.WriteFile(migration2Path, []byte(migration2Content), 0o644)
					if err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_migrations.test", "migrations.#", "2"),
					testresource.TestCheckResourceAttr("supabase_migrations.test", "migrations.1.name", "002_add_column.sql"),
				),
			},
		},
	})
}

func TestAccMigrationsResource_ModifyExisting(t *testing.T) {
	// Test that modifying existing migration content causes an error
	defer gock.OffAll()

	tmpDir := t.TempDir()
	migrationPath := filepath.Join(tmpDir, "001_test.sql")

	// Initial content
	content1 := "CREATE TABLE test1 (id SERIAL);"
	err := os.WriteFile(migrationPath, []byte(content1), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	config1 := fmt.Sprintf(`
resource "supabase_migrations" "test" {
	project_ref = "%s"
	migrations_dir = "%s"
}
`, testProjectRef, tmpDir)

	// Mock project status
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		AddMatcher(exactPathMatcher(projectApiPath)).
		Persist().
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})

	// Mock apply migration
	gock.New(defaultApiEndpoint).
		Post(migrationsApiPath).
		Persist().
		Reply(http.StatusOK)

	// Mock migration history read used by resource Read
	gock.New(defaultApiEndpoint).
		Get(migrationsApiPath).
		Persist().
		Reply(http.StatusOK).
		AddHeader("Content-Type", "application/json").
		BodyString(`[{"name":"001_test.sql","version":"001"}]`)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: config1,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_migrations.test", "migrations.#", "1"),
				),
			},
			{
				PreConfig: func() {
					// Modify the migration file content
					content2 := "CREATE TABLE test2 (id SERIAL);"
					err := os.WriteFile(migrationPath, []byte(content2), 0o644)
					if err != nil {
						t.Fatal(err)
					}
				},
				Config:      config1,
				ExpectError: regexp.MustCompile("Cannot modify existing migration"),
			},
		},
	})
}

func TestAccMigrationsResource_Import(t *testing.T) {
	// Test importing a migrations resource
	defer gock.OffAll()

	// Create a temporary migration file
	tmpDir := t.TempDir()
	migration1Path := filepath.Join(tmpDir, "001_initial.sql")
	migration1Content := "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);"
	err := os.WriteFile(migration1Path, []byte(migration1Content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	testConfig := fmt.Sprintf(`
resource "supabase_migrations" "test" {
	project_ref = "%s"
	migrations_dir = "%s"
}
`, testProjectRef, tmpDir)

	// Mock project status - called multiple times during create, import, and refresh
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		AddMatcher(exactPathMatcher(projectApiPath)).
		Persist().
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})

	// Mock apply migration
	gock.New(defaultApiEndpoint).
		Post(migrationsApiPath).
		Reply(http.StatusOK)

	// Mock migration history reads used by Read and ImportState
	gock.New(defaultApiEndpoint).
		Get(migrationsApiPath).
		Persist().
		Reply(http.StatusOK).
		AddHeader("Content-Type", "application/json").
		BodyString(`[{"name":"001_initial.sql","version":"001"}]`)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			// Create resource first
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_migrations.test", "project_ref", testProjectRef),
					testresource.TestCheckResourceAttr("supabase_migrations.test", "migrations.#", "1"),
				),
			},
			// Import the resource
			{
				ResourceName:            "supabase_migrations.test",
				ImportState:             true,
				ImportStateId:           testProjectRef,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"migrations", "migrations_dir"},
			},
		},
	})
}

func TestAccMigrationsResource_IdempotencyRetry(t *testing.T) {
	// Test that retrying with the same idempotency key prevents duplicate migration execution
	defer gock.OffAll()

	tmpDir := t.TempDir()
	migration1Path := filepath.Join(tmpDir, "001_test.sql")
	migration1Content := "CREATE TABLE test (id SERIAL);"
	err := os.WriteFile(migration1Path, []byte(migration1Content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Compute the expected idempotency key (just the digest)
	digest := computeSHA256(migration1Content)
	expectedIdempotencyKey := digest

	testConfig := fmt.Sprintf(`
resource "supabase_migrations" "test" {
	project_ref = "%s"
	migrations_dir = "%s"
}
`, testProjectRef, tmpDir)

	// Mock project status check
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		AddMatcher(exactPathMatcher(projectApiPath)).
		Persist().
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:     testProjectRef,
			Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})

	// Track how many times the migration API is called with the idempotency key
	migrationCallCount := 0
	receivedIdempotencyKey := ""
	gock.New(defaultApiEndpoint).
		Post(migrationsApiPath).
		AddMatcher(func(req *http.Request, _ *gock.Request) (bool, error) {
			// Capture the idempotency key from query params or headers
			idempotencyKey := req.URL.Query().Get("idempotency_key")
			if idempotencyKey == "" {
				// Try alternate param name formats
				idempotencyKey = req.URL.Query().Get("idempotencyKey")
			}
			if idempotencyKey == "" {
				// Try as a header
				idempotencyKey = req.Header.Get("Idempotency-Key")
			}
			receivedIdempotencyKey = idempotencyKey
			migrationCallCount++
			return true, nil
		}).
		Persist().
		Reply(http.StatusOK)

	// Mock migration history read
	gock.New(defaultApiEndpoint).
		Get(migrationsApiPath).
		Persist().
		Reply(http.StatusOK).
		AddHeader("Content-Type", "application/json").
		BodyString(`[{"name":"001_test.sql","version":"001"}]`)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: testConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr("supabase_migrations.test", "migrations.#", "1"),
					testresource.TestCheckResourceAttr("supabase_migrations.test", "migrations.0.name", "001_test.sql"),
				),
			},
		},
	})

	// Verify that the migration was called exactly once with the correct idempotency key
	if migrationCallCount != 1 {
		t.Errorf("Expected migration to be called once, but was called %d times", migrationCallCount)
	}
	if receivedIdempotencyKey != expectedIdempotencyKey {
		t.Errorf("Expected idempotency key %s, got %s", expectedIdempotencyKey, receivedIdempotencyKey)
	}
}

func TestMigrationDigestComputation(t *testing.T) {
	// Unit test for digest computation
	testCases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "simple migration",
			content:  "CREATE TABLE test (id SERIAL);",
			expected: computeSHA256("CREATE TABLE test (id SERIAL);"),
		},
		{
			name:     "multiline migration",
			content:  "CREATE TABLE users (\n  id SERIAL PRIMARY KEY,\n  name TEXT\n);",
			expected: computeSHA256("CREATE TABLE users (\n  id SERIAL PRIMARY KEY,\n  name TEXT\n);"),
		},
		{
			name:     "empty migration",
			content:  "",
			expected: computeSHA256(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hash := sha256.Sum256([]byte(tc.content))
			digest := hex.EncodeToString(hash[:])

			if digest != tc.expected {
				t.Errorf("Expected digest %s, got %s", tc.expected, digest)
			}
		})
	}
}

// Helper function to compute SHA-256 for test expectations
func computeSHA256(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
