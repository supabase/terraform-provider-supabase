package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/supabase/cli/pkg/api"
)

func TestUserAgentValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, provider, terraform, want string
	}{
		{"with terraform version", "1.11.0", "1.7.5", "TFProvider/1.11.0 Terraform/1.7.5"},
		{"without terraform version", "1.11.0", "", "TFProvider/1.11.0"},
		{"dev build", "dev", "1.12.0", "TFProvider/dev Terraform/1.12.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := userAgentValue(tt.provider, tt.terraform); got != tt.want {
				t.Errorf("userAgentValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientInfoValue(t *testing.T) {
	t.Parallel()

	got := clientInfoValue("1.11.0", "1.7.5")

	parts := strings.Split(got, "; ")
	if parts[0] != "terraform-provider-supabase/1.11.0" {
		t.Errorf("first token = %q, want terraform-provider-supabase/1.11.0", parts[0])
	}

	params := map[string]string{}
	for _, p := range parts[1:] {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			t.Fatalf("parameter %q is not key=value in %q", p, got)
		}
		params[k] = v
	}
	want := map[string]string{
		"runtime":               "terraform",
		"runtime-version":       "1.7.5",
		"platform":              platformName(runtime.GOOS),
		"platform-architecture": runtime.GOARCH,
	}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("parameter %s = %q, want %q (header %q)", k, params[k], v, got)
		}
	}
	if len(params) != len(want) {
		t.Errorf("got %d parameters, want %d: %q", len(params), len(want), got)
	}
}

func TestClientInfoValue_OmitsUnknownTerraformVersion(t *testing.T) {
	t.Parallel()

	got := clientInfoValue("dev", "")
	if !strings.Contains(got, "; runtime=terraform;") {
		t.Errorf("expected runtime=terraform to be kept, got %q", got)
	}
	if strings.Contains(got, "runtime-version") {
		t.Errorf("expected no runtime-version parameter, got %q", got)
	}
}

func TestPlatformName(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"darwin":  "macOS",
		"linux":   "Linux",
		"windows": "Windows",
		"wasip1":  "WASIp1",
		"plan9":   "plan9", // no mapping: emitted verbatim
	}
	for goos, want := range tests {
		if got := platformName(goos); got != want {
			t.Errorf("platformName(%q) = %q, want %q", goos, got, want)
		}
	}
}

// TestConfigure_SendsClientHeaders drives the real Configure and checks the
// headers that reach the API for a request made with the configured client.
func TestConfigure_SendsClientHeaders(t *testing.T) {
	t.Parallel()

	var gotUserAgent, gotClientInfo string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		gotClientInfo = r.Header.Get("X-Client-Info")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	ctx := context.Background()
	p := &SupabaseProvider{version: "1.2.3"}

	var schemaResp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(
			tftypes.Object{AttributeTypes: map[string]tftypes.Type{
				"endpoint":     tftypes.String,
				"access_token": tftypes.String,
			}},
			map[string]tftypes.Value{
				"endpoint":     tftypes.NewValue(tftypes.String, server.URL),
				"access_token": tftypes.NewValue(tftypes.String, "test"),
			},
		),
	}

	var resp provider.ConfigureResponse
	p.Configure(ctx, provider.ConfigureRequest{Config: config, TerraformVersion: "1.7.5"}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure returned errors: %v", resp.Diagnostics)
	}

	client, ok := resp.ResourceData.(*api.ClientWithResponses)
	if !ok {
		t.Fatalf("ResourceData is %T, want *api.ClientWithResponses", resp.ResourceData)
	}
	if _, err := client.V1ListAllProjectsWithResponse(ctx); err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if want := "TFProvider/1.2.3 Terraform/1.7.5"; gotUserAgent != want {
		t.Errorf("User-Agent = %q, want %q", gotUserAgent, want)
	}
	if want := "terraform-provider-supabase/1.2.3; "; !strings.HasPrefix(gotClientInfo, want) {
		t.Errorf("X-Client-Info = %q, want prefix %q", gotClientInfo, want)
	}
	if want := "; runtime=terraform; runtime-version=1.7.5; "; !strings.Contains(gotClientInfo, want) {
		t.Errorf("X-Client-Info = %q, want it to contain %q", gotClientInfo, want)
	}
}
