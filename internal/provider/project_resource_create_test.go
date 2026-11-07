// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestProjectResource_CreateReportsWaitFailure(t *testing.T) {
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)
	defer gock.OffAll()

	gock.New(defaultApiEndpoint).
		Post(projectsApiPath).
		Reply(http.StatusCreated).
		JSON(api.V1ProjectResponse{
			Id:             testProjectRef,
			Ref:            testProjectRef,
			Name:           "test",
			OrganizationId: "test-org",
			Region:         "us-east-1",
		})
	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "test",
			OrganizationId: "test-org",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusINITFAILED,
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	r := &ProjectResource{client: client}

	ctx := t.Context()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	data := ProjectResourceModel{
		OrganizationId:       types.StringValue("test-org"),
		Name:                 types.StringValue("test"),
		DatabasePassword:     types.StringValue("barbaz"),
		Region:               types.StringValue("us-east-1"),
		InstanceSize:         types.StringUnknown(),
		Id:                   types.StringUnknown(),
		LegacyApiKeysEnabled: types.BoolUnknown(),
		Timeouts: timeouts.Value{Object: types.ObjectNull(map[string]attr.Type{
			"create": types.StringType,
			"update": types.StringType,
		})},
	}
	if diags := plan.Set(ctx, &data); diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: plan.Raw.Copy()}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the project ends in a terminal state, got none")
	}
	if !strings.Contains(resp.Diagnostics[0].Detail(), "INIT_FAILED") {
		t.Fatalf("expected the terminal state in the error, got: %v", resp.Diagnostics)
	}
}
