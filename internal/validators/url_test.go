package validators

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestURL(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		schemes []string
		value   types.String
		wantErr bool
	}{
		"null":                     {schemes: []string{"https"}, value: types.StringNull()},
		"unknown":                  {schemes: []string{"https"}, value: types.StringUnknown()},
		"https":                    {schemes: []string{"https"}, value: types.StringValue("https://login.example.com")},
		"https with path and port": {schemes: []string{"https"}, value: types.StringValue("https://login.example.com:8443/tenant/")},
		"http not allowed":         {schemes: []string{"https"}, value: types.StringValue("http://login.example.com"), wantErr: true},
		"http allowed":             {schemes: []string{"https", "http"}, value: types.StringValue("http://login.example.com/jwks.json")},
		"other scheme":             {schemes: []string{"https", "http"}, value: types.StringValue("ftp://login.example.com/jwks.json"), wantErr: true},
		"no scheme":                {schemes: []string{"https"}, value: types.StringValue("login.example.com"), wantErr: true},
		"no host":                  {schemes: []string{"https"}, value: types.StringValue("https:///jwks.json"), wantErr: true},
		"port but no host":         {schemes: []string{"https"}, value: types.StringValue("https://:443/jwks.json"), wantErr: true},
		"relative path":            {schemes: []string{"https"}, value: types.StringValue("/.well-known/jwks.json"), wantErr: true},
		"not a URL":                {schemes: []string{"https"}, value: types.StringValue("not a url"), wantErr: true},
		"empty":                    {schemes: []string{"https"}, value: types.StringValue(""), wantErr: true},
		"invalid escape":           {schemes: []string{"https"}, value: types.StringValue("https://login.example.com/%zz"), wantErr: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := validator.StringRequest{Path: path.Root("url"), ConfigValue: tc.value}
			resp := &validator.StringResponse{}
			URL(tc.schemes...).ValidateString(t.Context(), req, resp)

			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
			if tc.wantErr && !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), tc.schemes[0]+"://") {
				t.Fatalf("expected the error to name the allowed scheme, got %q", resp.Diagnostics.Errors()[0].Detail())
			}
		})
	}
}

func TestURLPanicsWithoutSchemes(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("URL() with no schemes should panic")
		}
	}()
	URL()
}

func TestURLDescription(t *testing.T) {
	t.Parallel()
	if got := URL("https", "http").Description(t.Context()); got != "value must be an absolute URL using https or http" {
		t.Fatalf("Description() = %q", got)
	}
}
