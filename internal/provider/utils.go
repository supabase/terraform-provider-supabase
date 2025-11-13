package provider

import (
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

func Ptr[T any](v T) *T {
	return &v
}

// NullableToString converts an oapi-codegen [nullable.Nullable] to an appropriate
// terraform string type.
func NullableToString(n nullable.Nullable[string]) tftypes.String {
	if n.IsSpecified() && !n.IsNull() {
		// MustGet is safe when the value is specified and not null
		return tftypes.StringValue(n.MustGet())
	}

	return tftypes.StringNull()
}
