package planmodifiers

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestRequiresReplaceIfJSONChanged(t *testing.T) {
	t.Parallel()

	const (
		compact = `{"keys":[{"kid":"a","n":12345678901234567890}],"use":"sig"}`
		pretty  = "{\n  \"use\": \"sig\",\n  \"keys\": [ { \"n\": 12345678901234567890, \"kid\": \"a\" } ]\n}"
		other   = `{"keys":[{"kid":"b","n":12345678901234567890}],"use":"sig"}`
		// Differs from compact only past float64 precision.
		bigger = `{"keys":[{"kid":"a","n":12345678901234567891}],"use":"sig"}`
	)

	testCases := map[string]struct {
		state, plan          types.String
		creating, destroying bool
		wantReplace, wantErr bool
	}{
		"unchanged":                     {state: types.StringValue(compact), plan: types.StringValue(compact)},
		"formatting and key order only": {state: types.StringValue(compact), plan: types.StringValue(pretty)},
		"different value":               {state: types.StringValue(compact), plan: types.StringValue(other), wantReplace: true},
		"large numbers aren't rounded":  {state: types.StringValue(compact), plan: types.StringValue(bigger), wantReplace: true},
		"set from null":                 {state: types.StringNull(), plan: types.StringValue(compact), wantReplace: true},
		"set to null":                   {state: types.StringValue(compact), plan: types.StringNull(), wantReplace: true},
		"unknown":                       {state: types.StringValue(compact), plan: types.StringUnknown(), wantReplace: true},
		"invalid JSON":                  {state: types.StringValue(compact), plan: types.StringValue(`{"keys":`), wantErr: true},
		"create":                        {state: types.StringNull(), plan: types.StringValue(compact), creating: true},
		"destroy":                       {state: types.StringValue(compact), plan: types.StringNull(), destroying: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := planmodifier.StringRequest{
				Path:       path.Root("json"),
				StateValue: tc.state,
				PlanValue:  tc.plan,
				State:      tfsdk.State{Raw: testObject(tc.state, tc.creating)},
				Plan:       tfsdk.Plan{Raw: testObject(tc.plan, tc.destroying)},
			}
			resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
			RequiresReplaceIfJSONChanged().PlanModifyString(t.Context(), req, resp)

			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
			if resp.RequiresReplace != tc.wantReplace {
				t.Fatalf("RequiresReplace = %v, want %v", resp.RequiresReplace, tc.wantReplace)
			}
		})
	}
}

// testObject builds the resource object holding value, or a null object when the resource doesn't exist.
func testObject(value types.String, null bool) tftypes.Value {
	objectType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"json": tftypes.String}}
	if null {
		return tftypes.NewValue(objectType, nil)
	}
	var raw tftypes.Value
	switch {
	case value.IsUnknown():
		raw = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	case value.IsNull():
		raw = tftypes.NewValue(tftypes.String, nil)
	default:
		raw = tftypes.NewValue(tftypes.String, value.ValueString())
	}
	return tftypes.NewValue(objectType, map[string]tftypes.Value{"json": raw})
}
