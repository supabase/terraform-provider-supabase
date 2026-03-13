package provider

import (
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

	testConfig := fmt.Sprintf(`
resource "supabase_edge_function_secrets" "test" {
  project_ref = "%s"
  secrets = [
    {
      name  = "API_KEY"
      value = "secret-api-key-123"
    },
    {
      name  = "DATABASE_URL"
      value = "postgresql://user:pass@localhost:5432/db"
    }
  ]
}
`, projectRef)

	// Mock create secrets
	gock.New("https://api.supabase.com").
		Post(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK)

	// Mock read secrets after create
	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{
				Name:  "API_KEY",
				Value: "secret-api-key-123",
			},
			{
				Name:  "DATABASE_URL",
				Value: "postgresql://user:pass@localhost:5432/db",
			},
		})

	// Mock read secrets for refresh
	gock.New("https://api.supabase.com").
		Get(fmt.Sprintf("/v1/projects/%s/secrets", projectRef)).
		Reply(http.StatusOK).
		JSON([]api.SecretResponse{
			{
				Name:  "API_KEY",
				Value: "secret-api-key-123",
			},
			{
				Name:  "DATABASE_URL",
				Value: "postgresql://user:pass@localhost:5432/db",
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
				),
			},
		},
	})
}
