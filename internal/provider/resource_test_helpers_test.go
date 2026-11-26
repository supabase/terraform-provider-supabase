package provider

import (
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// testResourceSchema returns the schema of r, for unit tests that call resource methods directly.
func testResourceSchema(t *testing.T, r fwresource.Resource) schema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	r.Schema(t.Context(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// testObjectValue builds a value for schema s with the given string attributes set and every other attribute null.
// Use it to build the Raw value of a tfsdk.Config, tfsdk.Plan or tfsdk.State in unit tests.
func testObjectValue(t *testing.T, s schema.Schema, attrs map[string]string) tftypes.Value {
	t.Helper()
	objType, ok := s.Type().TerraformType(t.Context()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}
	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, typ := range objType.AttributeTypes {
		if v, ok := attrs[name]; ok {
			values[name] = tftypes.NewValue(typ, v)
		} else {
			values[name] = tftypes.NewValue(typ, nil)
		}
	}
	return tftypes.NewValue(objType, values)
}
