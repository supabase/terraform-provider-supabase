// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
)

// expectSecretKeyMatchesResource verifies that the data source's secret_keys
// list contains an entry with the given name whose api_key equals the
// api_key of the managed supabase_apikey resource.
type expectSecretKeyMatchesResource struct {
	dataSource string
	resource   string
	keyName    string
}

func (e expectSecretKeyMatchesResource) CheckState(ctx context.Context, req statecheck.CheckStateRequest, resp *statecheck.CheckStateResponse) {
	var resourceKey, dataSourceKey any
	var entries []any
	for _, r := range req.State.Values.RootModule.Resources {
		switch r.Address {
		case e.resource:
			resourceKey = r.AttributeValues["api_key"]
		case e.dataSource:
			entries, _ = r.AttributeValues["secret_keys"].([]any)
		}
	}
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if ok && m["name"] == e.keyName {
			dataSourceKey = m["api_key"]
		}
	}
	if resourceKey == nil || dataSourceKey == nil {
		resp.Error = fmt.Errorf("secret key %q not found (resource=%v, data source=%v)", e.keyName, resourceKey != nil, dataSourceKey != nil)
		return
	}
	if resourceKey != dataSourceKey {
		resp.Error = fmt.Errorf("api_key mismatch between %s and %s secret_keys entry %q", e.resource, e.dataSource, e.keyName)
	}
}

func TestAccApiKeysDataSource(t *testing.T) {
	organizationID := os.Getenv("ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("Organization ID not set")
	}

	projectResourceName := "supabase_project.test"
	resourceName := "supabase_apikey.test"
	dataSourceName := "data.supabase_apikeys.test"
	keyName := "test_key"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesActual,
		Steps: []resource.TestStep{
			// Read testing
			{
				ConfigDirectory: config.StaticDirectory("test-fixtures/ApiKeys/basic/"),
				ConfigVariables: testAccApiKeyLiveVariables(organizationID, acctest.RandomName(), keyName, nil),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(
						dataSourceName, tfjsonpath.New("project_ref"),
						projectResourceName, tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					statecheck.ExpectKnownValue(dataSourceName, tfjsonpath.New("anon_key"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(dataSourceName, tfjsonpath.New("service_role_key"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(dataSourceName, tfjsonpath.New("publishable_key"), knownvalue.NotNull()),
					expectSecretKeyMatchesResource{dataSource: dataSourceName, resource: resourceName, keyName: keyName},
				},
			},
		},
	})
}
