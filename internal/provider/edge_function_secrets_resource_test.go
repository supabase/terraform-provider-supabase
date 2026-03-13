package provider

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestAccEdgeFunctionSecretsResource(t *testing.T) {
	defer gock.OffAll()

	projectRef := "mayuaycdtijbctgqbycg"

	apiKeyPlain := "secret-api-key-123"
	dbUrlPlain := "postgresql://user:pass@localhost:5432/db"

	// Pre-compute SHA-256 digests matching what the API returns
	apiKeyDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(apiKeyPlain)))
	dbUrlDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(dbUrlPlain)))

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
	project_ref = "%s"
	secrets = [
		{
			name  = "API_KEY"
			value = "%s"
		},
		{
			name  = "DATABASE_URL"
			value = "%s"
		}
	]
}
`, projectRef, apiKeyPlain, dbUrlPlain)

	// Mock create secrets
	gock.New("https://api.supabase.com").
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	// Mock read secrets after create – API returns SHA-256 digests, not plaintext
	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{
				Name:  "API_KEY",
				Value: apiKeyDigest,
			},
			{
				Name:  "DATABASE_URL",
				Value: dbUrlDigest,
			},
		})

	// Mock read secrets for refresh
	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{
				Name:  "API_KEY",
				Value: apiKeyDigest,
			},
			{
				Name:  "DATABASE_URL",
				Value: dbUrlDigest,
			},
		})

	// Mock delete secrets
	gock.New("https://api.supabase.com").
		Delete(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "project_ref", projectRef),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secrets.#", "2"),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.%", "2"),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.API_KEY", apiKeyDigest),
					resource.TestCheckResourceAttr("supabase_edge_function_secrets.test", "secret_digests.DATABASE_URL", dbUrlDigest),
				),
			},
		},
	})
}
