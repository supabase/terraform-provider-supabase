// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"testing"
	"testing/synctest"

	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

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

	diags := waitForProjectActive(t.Context(), "test-project", client)

	if !diags.HasError() {
		t.Errorf("Expected error for terminal state, got success")
	}
}

func TestWaitForServicesActive_AllHealthy(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New("https://api.supabase.com").
		Get("/v1/projects/test-project/health").
		Reply(http.StatusOK).
		JSON([]api.V1ServiceHealthResponse{
			{Name: api.V1ServiceHealthResponseNameDb, Status: api.ACTIVEHEALTHY, Healthy: true},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		})

	client, err := api.NewClientWithResponses("https://api.supabase.com")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForServicesActive(t.Context(), "test-project", client)
	if diags.HasError() {
		t.Errorf("Expected success, got errors: %v", diags)
	}
}

func TestWaitForServicesActive_UnhealthyFails(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New("https://api.supabase.com").
		Get("/v1/projects/test-project/health").
		Reply(http.StatusOK).
		JSON([]api.V1ServiceHealthResponse{
			{Name: api.V1ServiceHealthResponseNameDb, Status: api.UNHEALTHY, Healthy: false, Error: Ptr(`{"error": "fatal"}`)},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		})

	client, err := api.NewClientWithResponses("https://api.supabase.com")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForServicesActive(t.Context(), "test-project", client)
	if !diags.HasError() {
		t.Error("Expected error for unhealthy service, got success")
	}
}

func TestWaitForServicesActive_TransientErrorsKeepsPolling(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	responses := [][]api.V1ServiceHealthResponse{
		{
			{Name: api.V1ServiceHealthResponseNamePooler, Status: api.UNHEALTHY, Healthy: false, Error: Ptr(`{"error": "not found"}`)},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.COMINGUP},
		},
		{
			{Name: api.V1ServiceHealthResponseNamePooler, Status: api.COMINGUP},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.COMINGUP},
		},
		{
			{Name: api.V1ServiceHealthResponseNamePooler, Status: api.ACTIVEHEALTHY, Healthy: true},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.UNHEALTHY, Healthy: false, Error: Ptr(`{"error": "Failed to retrieve project's auth service health"}`)},
		},
		{
			{Name: api.V1ServiceHealthResponseNamePooler, Status: api.ACTIVEHEALTHY, Healthy: true},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		},
	}

	for _, resp := range responses {
		gock.New("https://api.supabase.com").
			Get("/v1/projects/test-project/health").
			Reply(http.StatusOK).
			JSON(resp)
	}

	synctest.Test(t, func(t *testing.T) {
		client, err := api.NewClientWithResponses("https://api.supabase.com")
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		diags := waitForServicesActive(t.Context(), "test-project", client)
		if diags.HasError() {
			t.Errorf("Expected success, got errors: %v", diags)
		}
	})
}
