// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/supabase/cli/pkg/api"
)

// extractClient extracts the API client from provider data.
// Returns the client and true if successful, nil and false otherwise.
// Adds an error to diagnostics if the provider data is not the expected type.
func extractClient(providerData any, diagnostics *diag.Diagnostics) (*api.ClientWithResponses, bool) {
	if providerData == nil {
		return nil, false
	}

	client, ok := providerData.(*api.ClientWithResponses)
	if !ok {
		diagnostics.AddError(
			"Unexpected Configure Type",
			fmt.Sprintf("Expected *api.ClientWithResponses, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil, false
	}

	return client, true
}
