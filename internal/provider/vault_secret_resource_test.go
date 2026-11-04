// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

func TestParseVaultSecretImportID(t *testing.T) {
	t.Parallel()

	projectRef, secretRef, d := parseVaultSecretImportID(" mayuaycdtijbctgqbycg / stripe_secret_key ")
	if d != nil {
		t.Fatalf("unexpected diagnostic: %s", d.Detail())
	}
	if projectRef != testProjectRef || secretRef != "stripe_secret_key" {
		t.Fatalf("got %s / %s", projectRef, secretRef)
	}

	_, secretRef, d = parseVaultSecretImportID(testProjectRef + "/" + testVaultSecretUUID)
	if d != nil {
		t.Fatalf("unexpected diagnostic: %s", d.Detail())
	}
	if secretRef != testVaultSecretUUID {
		t.Fatalf("got secret ref %s", secretRef)
	}

	_, secretRef, d = parseVaultSecretImportID(testProjectRef + "/nested/secret")
	if d != nil {
		t.Fatalf("unexpected diagnostic: %s", d.Detail())
	}
	if secretRef != "nested/secret" {
		t.Fatalf("got secret ref %s", secretRef)
	}

	for _, id := range []string{"", "project-only", "/secret", "project/"} {
		if _, _, d := parseVaultSecretImportID(id); d == nil {
			t.Fatalf("expected diagnostic for %q", id)
		}
	}
}

func TestApplyVaultSecretRow(t *testing.T) {
	t.Parallel()

	var data VaultSecretResourceModel
	diags := applyVaultSecretRow(&data, map[string]any{
		"id":               testVaultSecretUUID,
		"name":             "stripe_secret_key",
		"description":      "Stripe API secret",
		"decrypted_secret": "sk_test_vault_value",
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if data.Id.ValueString() != testVaultSecretUUID || data.Name.ValueString() != "stripe_secret_key" || data.Value.ValueString() != "sk_test_vault_value" || data.Description.ValueString() != "Stripe API secret" {
		t.Fatalf("unexpected model: %#v", data)
	}

	diags = applyVaultSecretRow(&data, map[string]any{
		"id":               testVaultSecretUUID,
		"name":             "stripe_secret_key",
		"description":      "",
		"decrypted_secret": "",
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !data.Description.IsNull() {
		t.Fatal("expected blank description to be null")
	}
	if data.Value.ValueString() != "" {
		t.Fatalf("expected empty secret, got %q", data.Value.ValueString())
	}

	diags = applyVaultSecretRow(&data, map[string]any{
		"id":   testVaultSecretUUID,
		"name": "stripe_secret_key",
	})
	if !diags.HasError() {
		t.Fatal("expected missing decrypted_secret to fail")
	}

	for _, row := range []map[string]any{
		{"id": testVaultSecretUUID, "decrypted_secret": "sk_test_vault_value"},
		{"id": testVaultSecretUUID, "name": nil, "decrypted_secret": "sk_test_vault_value"},
		{"id": testVaultSecretUUID, "name": "", "decrypted_secret": "sk_test_vault_value"},
	} {
		if diags := applyVaultSecretRow(&data, row); !diags.HasError() {
			t.Fatalf("expected unnamed secret to fail: %#v", row)
		}
	}

	id, diags := vaultCreateSecretID([]map[string]any{{"create_secret": testVaultSecretUUID}})
	if diags.HasError() || id != testVaultSecretUUID {
		t.Fatalf("create_secret id = %s, diags = %v", id, diags)
	}
}

func TestVaultSecretMissingProject(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	client, err := api.NewClientWithResponses(server.URL, api.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}

	rows, projectNotFound, diags := runDatabaseQuery(context.Background(), client, testProjectRef, readVaultSecretByIDSQL, []any{testVaultSecretUUID})
	if diags.HasError() || !projectNotFound || rows != nil {
		t.Fatalf("rows=%v projectNotFound=%v diags=%v", rows, projectNotFound, diags)
	}
	if projectNotFoundError(testProjectRef, false).HasError() {
		t.Fatal("expected no diagnostic when the project exists")
	}
	if !projectNotFoundError(testProjectRef, true).HasError() {
		t.Fatal("expected a diagnostic when the project is missing")
	}

	data := VaultSecretResourceModel{
		ProjectRef: types.StringValue(testProjectRef),
		Id:         types.StringValue(testVaultSecretUUID),
	}
	found, diags := readVaultSecretByID(context.Background(), client, &data)
	if found || diags.HasError() {
		t.Fatalf("found=%v diags=%v", found, diags)
	}
}

func TestAccVaultSecretResource(t *testing.T) {
	gock.OffAll()

	store := newVaultSecretStore()
	server := httptest.NewServer(vaultQueryHandler(store))
	defer server.Close()

	updatedConfig := fmt.Sprintf(`
resource "supabase_vault_secret" "example" {
  project_ref = "%s"
  name        = "stripe_secret_key_v2"
  value       = "sk_test_vault_value_v2"
}
`, testProjectRef)

	replacedConfig := fmt.Sprintf(`
resource "supabase_vault_secret" "example" {
  project_ref = "%s"
  name        = "stripe_secret_key_v2"
  value       = "sk_test_vault_value_v2"
}
`, testBranchRef)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Setenv("SUPABASE_API_ENDPOINT", server.URL)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: examples.VaultSecretResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "id", testVaultSecretUUID),
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "project_ref", testProjectRef),
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "name", "stripe_secret_key"),
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "value", "sk_test_vault_value"),
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "description", "Stripe API secret"),
				),
			},
			{
				ResourceName:      "supabase_vault_secret.example",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     testProjectRef + "/stripe_secret_key",
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "id", testVaultSecretUUID),
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "name", "stripe_secret_key_v2"),
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "value", "sk_test_vault_value_v2"),
					resource.TestCheckNoResourceAttr("supabase_vault_secret.example", "description"),
				),
			},
			{
				ResourceName:      "supabase_vault_secret.example",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["supabase_vault_secret.example"]
					if !ok {
						return "", fmt.Errorf("supabase_vault_secret.example not found in state")
					}
					return rs.Primary.Attributes["project_ref"] + "/" + rs.Primary.Attributes["id"], nil
				},
			},
			{
				Config: replacedConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("supabase_vault_secret.example", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "project_ref", testBranchRef),
					resource.TestCheckResourceAttr("supabase_vault_secret.example", "name", "stripe_secret_key_v2"),
					func(*terraform.State) error {
						store.mu.Lock()
						defer store.mu.Unlock()
						if _, ok := store.secrets[testProjectRef]; ok {
							return fmt.Errorf("secret still present on %s after replace", testProjectRef)
						}
						return nil
					},
				),
			},
		},
	})

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.secrets) != 0 {
		t.Fatalf("expected destroy to delete vault secrets, still have %#v", store.secrets)
	}
}

type vaultSecretRecord struct {
	name        string
	value       string
	description string
}

type vaultSecretStore struct {
	mu      sync.Mutex
	seq     int
	secrets map[string]map[string]*vaultSecretRecord
}

func newVaultSecretStore() *vaultSecretStore {
	return &vaultSecretStore{
		secrets: map[string]map[string]*vaultSecretRecord{},
	}
}

func vaultQueryHandler(store *vaultSecretStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test" {
			http.Error(w, "missing authorization", http.StatusUnauthorized)
			return
		}

		projectRef, ok := parseProjectQueryPath(r.URL.Path)
		if !ok || (projectRef == testProjectRef && r.URL.Path != dbQueryApiPath) {
			http.Error(w, "unexpected query path "+r.URL.Path, http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var payload struct {
			Query      string `json:"query"`
			Parameters []any  `json:"parameters"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if queryContainsParam(payload.Query, payload.Parameters) {
			http.Error(w, "query interpolated a parameter", http.StatusBadRequest)
			return
		}

		switch {
		case strings.Contains(payload.Query, "vault.create_secret"):
			handleCreateVaultSecret(w, store, projectRef, payload.Parameters)
		case strings.Contains(payload.Query, "vault.update_secret"):
			handleUpdateVaultSecret(w, store, projectRef, payload.Parameters)
		case strings.Contains(payload.Query, "DELETE FROM vault.secrets"):
			handleDeleteVaultSecret(w, store, projectRef, payload.Parameters)
		case strings.Contains(payload.Query, "vault.decrypted_secrets") && strings.Contains(payload.Query, "WHERE id"):
			handleReadVaultSecretByID(w, store, projectRef, payload.Parameters)
		case strings.Contains(payload.Query, "vault.decrypted_secrets") && strings.Contains(payload.Query, "WHERE name"):
			handleReadVaultSecretByName(w, store, projectRef, payload.Parameters)
		default:
			http.Error(w, "unexpected query "+payload.Query, http.StatusBadRequest)
		}
	})
}

func handleCreateVaultSecret(w http.ResponseWriter, store *vaultSecretStore, projectRef string, params []any) {
	values, ok := paramStrings(params, 3)
	if !ok {
		http.Error(w, fmt.Sprintf("create expected 3 string parameters, got %#v", params), http.StatusBadRequest)
		return
	}
	value, name, description := values[0], values[1], values[2]

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.nameTaken(projectRef, name, "") {
		http.Error(w, "duplicate secret name", http.StatusBadRequest)
		return
	}
	store.seq++
	id := uuid.NewString()
	if store.seq == 1 {
		id = testVaultSecretUUID
	}
	if store.secrets[projectRef] == nil {
		store.secrets[projectRef] = map[string]*vaultSecretRecord{}
	}
	store.secrets[projectRef][id] = &vaultSecretRecord{name: name, value: value, description: description}
	writeJSON(w, []map[string]any{{"id": id}})
}

func handleUpdateVaultSecret(w http.ResponseWriter, store *vaultSecretStore, projectRef string, params []any) {
	values, ok := paramStrings(params, 4)
	if !ok {
		http.Error(w, fmt.Sprintf("update expected 4 string parameters, got %#v", params), http.StatusBadRequest)
		return
	}
	id, value, name, description := values[0], values[1], values[2], values[3]

	store.mu.Lock()
	defer store.mu.Unlock()
	rec := store.secrets[projectRef][id]
	if rec == nil {
		writeJSON(w, []map[string]any{})
		return
	}
	if store.nameTaken(projectRef, name, id) {
		http.Error(w, "duplicate secret name", http.StatusBadRequest)
		return
	}
	rec.name = name
	rec.value = value
	rec.description = description
	writeJSON(w, []map[string]any{{"update_secret": ""}})
}

func handleDeleteVaultSecret(w http.ResponseWriter, store *vaultSecretStore, projectRef string, params []any) {
	values, ok := paramStrings(params, 1)
	if !ok {
		http.Error(w, fmt.Sprintf("delete expected 1 string parameter, got %#v", params), http.StatusBadRequest)
		return
	}
	id := values[0]

	store.mu.Lock()
	defer store.mu.Unlock()
	_, existed := store.secrets[projectRef][id]
	delete(store.secrets[projectRef], id)
	if len(store.secrets[projectRef]) == 0 {
		delete(store.secrets, projectRef)
	}
	if !existed {
		writeJSON(w, []map[string]any{})
		return
	}
	writeJSON(w, []map[string]any{{"id": id}})
}

func handleReadVaultSecretByID(w http.ResponseWriter, store *vaultSecretStore, projectRef string, params []any) {
	values, ok := paramStrings(params, 1)
	if !ok {
		http.Error(w, fmt.Sprintf("read by id expected 1 string parameter, got %#v", params), http.StatusBadRequest)
		return
	}
	id := values[0]

	store.mu.Lock()
	defer store.mu.Unlock()
	rec := store.secrets[projectRef][id]
	if rec == nil {
		writeJSON(w, []map[string]any{})
		return
	}
	writeJSON(w, []map[string]any{vaultSecretJSON(id, rec)})
}

func handleReadVaultSecretByName(w http.ResponseWriter, store *vaultSecretStore, projectRef string, params []any) {
	values, ok := paramStrings(params, 1)
	if !ok {
		http.Error(w, fmt.Sprintf("read by name expected 1 string parameter, got %#v", params), http.StatusBadRequest)
		return
	}
	name := values[0]

	store.mu.Lock()
	defer store.mu.Unlock()
	var rows []map[string]any
	for id, rec := range store.secrets[projectRef] {
		if rec.name == name {
			rows = append(rows, vaultSecretJSON(id, rec))
		}
	}
	writeJSON(w, rows)
}

func (s *vaultSecretStore) nameTaken(projectRef, name, exceptID string) bool {
	for id, rec := range s.secrets[projectRef] {
		if id != exceptID && rec.name == name {
			return true
		}
	}
	return false
}

func vaultSecretJSON(id string, rec *vaultSecretRecord) map[string]any {
	description := any(rec.description)
	if rec.description == "" {
		description = nil
	}
	return map[string]any{
		"id":               id,
		"name":             rec.name,
		"description":      description,
		"decrypted_secret": rec.value,
	}
}

func parseProjectQueryPath(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, "/v1/projects/")
	if !ok {
		return "", false
	}
	projectRef, ok := strings.CutSuffix(rest, "/database/query")
	if !ok || projectRef == "" || strings.Contains(projectRef, "/") {
		return "", false
	}
	return projectRef, true
}

func paramStrings(params []any, n int) ([]string, bool) {
	if len(params) != n {
		return nil, false
	}
	values := make([]string, n)
	for i, param := range params {
		text, ok := param.(string)
		if !ok {
			return nil, false
		}
		values[i] = text
	}
	return values, true
}

func queryContainsParam(query string, params []any) bool {
	for _, param := range params {
		text, ok := param.(string)
		if ok && text != "" && strings.Contains(query, text) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write(body); err != nil {
		return
	}
}
