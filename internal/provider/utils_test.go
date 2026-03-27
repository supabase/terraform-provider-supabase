// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"net/http"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/supabase/cli/pkg/api"
	"gopkg.in/h2non/gock.v1"
)

func TestWaitForProjectActive_TerminalState(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(projectApiPath).
		Reply(http.StatusOK).
		JSON(api.V1ProjectWithDatabaseResponse{
			Id:             testProjectRef,
			Name:           "Test Project",
			OrganizationId: "test-org",
			Region:         "us-east-1",
			Status:         api.V1ProjectWithDatabaseResponseStatusINITFAILED,
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForProjectActive(t.Context(), testProjectRef, client)

	if !diags.HasError() {
		t.Errorf("Expected error for terminal state, got success")
	}
}

func TestWaitForServicesActive_AllHealthy(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbSchema:          "public,storage,graphql_public",
			DbExtraSearchPath: "public,extensions",
			MaxRows:           1000,
		})

	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON([]api.V1ServiceHealthResponse{
			{Name: api.V1ServiceHealthResponseNameDb, Status: api.ACTIVEHEALTHY, Healthy: true},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForServicesActive(t.Context(), testProjectRef, client)
	if diags.HasError() {
		t.Errorf("Expected success, got errors: %v", diags)
	}
}

func TestWaitForServicesActive_UnhealthyFails(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbSchema:          "public,storage,graphql_public",
			DbExtraSearchPath: "public,extensions",
			MaxRows:           1000,
		})

	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusOK).
		JSON([]api.V1ServiceHealthResponse{
			{Name: api.V1ServiceHealthResponseNameDb, Status: api.UNHEALTHY, Healthy: false, Error: Ptr(`{"error": "fatal"}`)},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForServicesActive(t.Context(), testProjectRef, client)
	if !diags.HasError() {
		t.Error("Expected error for unhealthy service, got success")
	}
}

func TestIsDataApiDisabled_EmptySchema(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbSchema:          "",
			DbExtraSearchPath: "public,extensions",
			MaxRows:           1000,
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	disabled, diags := isDataApiDisabled(t.Context(), testProjectRef, client)
	if diags.HasError() {
		t.Fatalf("Unexpected error: %v", diags)
	}
	if !disabled {
		t.Error("Expected Data API to be disabled when db_schema is empty")
	}
}

func TestIsDataApiDisabled_NonEmptySchema(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbSchema:          "public,storage,graphql_public",
			DbExtraSearchPath: "public,extensions",
			MaxRows:           1000,
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	disabled, diags := isDataApiDisabled(t.Context(), testProjectRef, client)
	if diags.HasError() {
		t.Fatalf("Unexpected error: %v", diags)
	}
	if disabled {
		t.Error("Expected Data API to be enabled when db_schema is non-empty")
	}
}

func TestIsDataApiDisabled_ApiError(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusInternalServerError)

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, diags := isDataApiDisabled(t.Context(), testProjectRef, client)
	if !diags.HasError() {
		t.Error("Expected error when API returns 500")
	}
}

func TestWaitForServicesActive_ErrorsWhenPostgrestConfigFails(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusInternalServerError)

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForServicesActive(t.Context(), testProjectRef, client)
	if !diags.HasError() {
		t.Error("Expected error when PostgREST config fetch fails")
	}
}

func TestWaitForServicesActive_SkipsRestWhenDataApiDisabled(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	// PostgREST config returns empty db_schema → Data API disabled → rest excluded
	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbSchema:          "",
			DbExtraSearchPath: "public,extensions",
			MaxRows:           1000,
		})

	// Capture the health request to verify "rest" was excluded from query params.
	var capturedServices []string
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		SetMatcher(gock.NewBasicMatcher()).
		AddMatcher(func(req *http.Request, ereq *gock.Request) (bool, error) {
			capturedServices = req.URL.Query()["services"]
			return true, nil
		}).
		Reply(http.StatusOK).
		JSON([]api.V1ServiceHealthResponse{
			{Name: api.V1ServiceHealthResponseNameDb, Status: api.ACTIVEHEALTHY, Healthy: true},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForServicesActive(t.Context(), testProjectRef, client)
	if diags.HasError() {
		t.Errorf("Expected success when data API is disabled, got errors: %v", diags)
	}
	if slices.Contains(capturedServices, string(api.V1GetServicesHealthParamsServicesRest)) {
		t.Errorf("Expected 'rest' to be excluded from services query param, got: %v", capturedServices)
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

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbSchema:          "public,storage,graphql_public",
			DbExtraSearchPath: "public,extensions",
			MaxRows:           1000,
		})

	for _, resp := range responses {
		gock.New(defaultApiEndpoint).
			Get(healthApiPath).
			Reply(http.StatusOK).
			JSON(resp)
	}

	synctest.Test(t, func(t *testing.T) {
		client, err := api.NewClientWithResponses(defaultApiEndpoint)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		diags := waitForServicesActive(t.Context(), testProjectRef, client)
		if diags.HasError() {
			t.Errorf("Expected success, got errors: %v", diags)
		}
	})
}
