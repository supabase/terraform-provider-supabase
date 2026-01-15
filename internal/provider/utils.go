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
func NullableToString[T ~string](n nullable.Nullable[T]) tftypes.String {
	if n.IsSpecified() && !n.IsNull() {
		return tftypes.StringValue(string(n.MustGet()))
	}

	return tftypes.StringNull()
}
