// Package planmodifiers holds Terraform Plugin Framework plan modifiers shared across resources.
package planmodifiers

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/supabase/terraform-provider-supabase/internal/jsonvalue"
)

const requiresReplaceIfJSONChangedDescription = "Requires replacement when the JSON value changes, ignoring formatting."

// RequiresReplaceIfJSONChanged returns a plan modifier for a JSON string attribute that replaces the resource
// when the value changes meaning. Formatting-only changes, such as whitespace or the order of object keys,
// don't replace it. The built-in RequiresReplace compares raw strings, so it would.
func RequiresReplaceIfJSONChanged() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		jsonChanged,
		requiresReplaceIfJSONChangedDescription,
		requiresReplaceIfJSONChangedDescription,
	)
}

// jsonChanged is only called when the planned and current values differ, and not on create or destroy.
func jsonChanged(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	// A null or unknown value can't be compared as JSON, so it counts as a change.
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() || req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		resp.RequiresReplace = true
		return
	}

	same, err := jsonvalue.Equal(req.PlanValue.ValueString(), req.StateValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON", fmt.Sprintf("Unable to compare the planned and current JSON values: %s", err))
		return
	}
	resp.RequiresReplace = !same
}
