package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/oapi-codegen/nullable"
)

// defaultObjOpts allows missing/null/unknown keys when unmarshalling a TF object
// into a Go struct, so partial objects don't produce hard failures.
var defaultObjOpts = basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true}

// attrTyper is satisfied by every model struct that exposes its TF attribute types.
type attrTyper interface {
	AttributeTypes() map[string]attr.Type
}

// buildObj wraps types.ObjectValueFrom, inferring the attribute type map from the
// model struct itself so callers don't repeat T{}.AttributeTypes() at every site.
func buildObj[M attrTyper](ctx context.Context, m M) (types.Object, diag.Diagnostics) {
	return types.ObjectValueFrom(ctx, m.AttributeTypes(), m)
}

// readObj wraps types.Object.As, eliminating the "var m T; obj.As(...)" boilerplate.
// Returns a zero-value M (with null fields) when obj is null or unknown.
func readObj[M any](ctx context.Context, obj types.Object) (M, diag.Diagnostics) {
	var m M
	if obj.IsNull() || obj.IsUnknown() {
		return m, nil
	}
	return m, obj.As(ctx, &m, defaultObjOpts)
}

// setStr converts a TF string to a nullable API string.
// Returns an unspecified Nullable (omitted from JSON) for null/unknown values.
func setStr(s types.String) nullable.Nullable[string] {
	if s.IsNull() || s.IsUnknown() {
		return nullable.Nullable[string]{}
	}
	return nullable.NewNullableWithValue(s.ValueString())
}

// setBool converts a TF bool to a nullable API bool.
// Returns an unspecified Nullable (omitted from JSON) for null/unknown values.
func setBool(b types.Bool) nullable.Nullable[bool] {
	if b.IsNull() || b.IsUnknown() {
		return nullable.Nullable[bool]{}
	}
	return nullable.NewNullableWithValue(b.ValueBool())
}

// setInt64 converts a TF int64 to a nullable int API field.
// Returns an unspecified Nullable (omitted from JSON) for null/unknown values.
func setInt64(i types.Int64) nullable.Nullable[int] {
	if i.IsNull() || i.IsUnknown() {
		return nullable.Nullable[int]{}
	}
	return nullable.NewNullableWithValue(int(i.ValueInt64()))
}

// setFloat32 converts a TF int64 to a nullable float32 (used by sessions fields).
// Returns an unspecified Nullable (omitted from JSON) for null/unknown values.
func setFloat32(i types.Int64) nullable.Nullable[float32] {
	if i.IsNull() || i.IsUnknown() {
		return nullable.Nullable[float32]{}
	}
	return nullable.NewNullableWithValue(float32(i.ValueInt64()))
}
