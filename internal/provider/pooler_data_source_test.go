// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

func TestAccPoolerDataSource(t *testing.T) {
	poolerUrl := "postgres://user:pass@db.supabase.co:5432/postgres"
	// Setup mock api
	defer gock.OffAll()
	gock.New("https://api.supabase.com").
		Get("/v1/projects/mayuaycdtijbctgqbycg/config/database/pgbouncer").
		Times(3).
		Reply(http.StatusOK).
		JSON(api.V1PgbouncerConfigResponse{
			ConnectionString:        &poolerUrl,
			DefaultPoolSize:         Ptr(float32(15)),
			IgnoreStartupParameters: Ptr(""),
			MaxClientConn:           Ptr(float32(200)),
			PoolMode:                Ptr(api.Transaction),
		})
	// Run test
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: examples.PoolerDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.supabase_pooler.production", "url.transaction", poolerUrl),
				),
			},
		},
	})
}
