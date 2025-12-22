// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestWaitForProjectActive_UnknownStatus(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	// First call: return an unrecognized status (simulating a new API status)
	gock.New("https://api.supabase.com").
		Get("/v1/projects/test-project").
		Reply(http.StatusOK).
		JSON(map[string]interface{}{
			"id":              "test-project",
			"name":            "Test Project",
			"organization_id": "test-org",
			"region":          "us-east-1",
			"status":          "SOME_NEW_STATUS",
		})

	// Second call: return ACTIVE_HEALTHY
	gock.New("https://api.supabase.com").
		Get("/v1/projects/test-project").
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             "test-project",
			Name:           "Test Project",
			OrganizationId: "test-org",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY,
		})

	client, err := api.NewClientWithResponses("https://api.supabase.com")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForProjectActive(context.Background(), "test-project", client)

	if diags.HasError() {
		t.Errorf("Expected success, got errors: %v", diags)
	}

	if !gock.IsDone() {
		t.Errorf("Not all expected HTTP requests were made")
	}
}

func TestWaitForProjectActive_TerminalState(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New("https://api.supabase.com").
		Get("/v1/projects/test-project").
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             "test-project",
			Name:           "Test Project",
			OrganizationId: "test-org",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusINITFAILED,
		})

	client, err := api.NewClientWithResponses("https://api.supabase.com")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForProjectActive(context.Background(), "test-project", client)

	if !diags.HasError() {
		t.Errorf("Expected error for terminal state, got success")
	}
}
