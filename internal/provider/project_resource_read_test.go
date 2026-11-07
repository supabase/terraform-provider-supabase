// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestProjectResource_ReadRemovesResourceOnNotFound(t *testing.T) {
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusNotFound)

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	r := &ProjectResource{client: client}

	ctx := t.Context()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema}
	data := ProjectResourceModel{
		OrganizationId:       types.StringValue("test-org"),
		Name:                 types.StringValue("test"),
		DatabasePassword:     types.StringValue("barbaz"),
		Region:               types.StringValue("us-east-1"),
		InstanceSize:         types.StringValue("micro"),
		Id:                   types.StringValue(testProjectRef),
		LegacyApiKeysEnabled: types.BoolValue(false),
		Timeouts: timeouts.Value{Object: types.ObjectNull(map[string]attr.Type{
			"create": types.StringType,
			"update": types.StringType,
		})},
	}
	if diags := state.Set(ctx, &data); diags.HasError() {
		t.Fatalf("failed to build state: %v", diags)
	}

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error on 404, got: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected resource to be removed from state on 404")
	}
}
