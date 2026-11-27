// Package validators holds Terraform Plugin Framework validators shared across resources and data sources.
package validators

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var _ validator.String = urlValidator{}

// URL returns a validator that accepts an absolute URL with a host and one of the given schemes,
// for example URL("https") or URL("https", "http"). It panics if no scheme is given.
func URL(schemes ...string) validator.String {
	if len(schemes) == 0 {
		panic("validators.URL needs at least one scheme")
	}
	return urlValidator{schemes: schemes}
}

type urlValidator struct {
	schemes []string
}

func (v urlValidator) Description(ctx context.Context) string {
	return v.MarkdownDescription(ctx)
}

func (v urlValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("value must be an absolute URL using %s", strings.Join(v.schemes, " or "))
}

func (v urlValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || !slices.Contains(v.schemes, u.Scheme) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid URL",
			fmt.Sprintf("Expected an absolute URL with a host, starting with %s://, got: %q", strings.Join(v.schemes, ":// or "), value))
	}
}
