// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestAPIKeyResource_ReadRemovesResourceOnNotFound(t *testing.T) {
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)
	defer gock.OffAll()

	keyID := uuid.New().String()
	gock.New(defaultApiEndpoint).
		Get(apiKeysApiPath + "/" + keyID).
		Reply(http.StatusNotFound)

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	r := &APIKeyResource{client: client}

	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema}
	data := ApiKeyResourceModel{
		ProjectRef:        types.StringValue(testProjectRef),
		Name:              types.StringValue("test"),
		Description:       types.StringNull(),
		Type:              types.StringValue("secret"),
		ApiKey:            types.StringValue("sb_secret_x"),
		SecretJwtTemplate: types.ObjectNull(secretJwtTemplateAttrTypes),
		Id:                types.StringValue(keyID),
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

func TestBranchResource_ReadRemovesResourceOnNotFound(t *testing.T) {
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)
	defer gock.OffAll()

	branchID := uuid.New().String()
	gock.New(defaultApiEndpoint).
		Get("/v1/branches/" + branchID).
		Reply(http.StatusNotFound)

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	r := &BranchResource{client: client}

	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema}
	data := BranchResourceModel{
		GitBranch:        types.StringValue("feature"),
		ParentProjectRef: types.StringValue(testProjectRef),
		Region:           types.StringNull(),
		Persistent:       types.BoolValue(false),
		Database:         types.ObjectNull((BranchDatabaseModel{}).AttributeTypes()),
		Id:               types.StringValue(branchID),
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
