package provider

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

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

	diags := waitForProjectActive(t.Context(), testProjectRef, client, 5*time.Minute)

	if !diags.HasError() {
		t.Errorf("Expected error for terminal state, got success")
	}
}

func TestWaitForProjectActive_BranchRefFallsBackToBranchesEndpoint(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	// Branch refs are not returned by the projects endpoint.
	gock.New(defaultApiEndpoint).
		Get(branchProjectApiPath).
		Reply(http.StatusNotFound).
		JSON(map[string]string{"message": "Project not found"})

	gock.New(defaultApiEndpoint).
		Get(branchRefApiPath).
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{
			Ref:    testBranchRef,
			Status: api.BranchDetailResponseStatusACTIVEHEALTHY,
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForProjectActive(t.Context(), testBranchRef, client, 5*time.Minute)
	if diags.HasError() {
		t.Errorf("Expected success for active branch ref, got errors: %v", diags)
	}
}

func TestWaitForProjectActive_BranchRefPollsUntilActive(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	for _, status := range []api.BranchDetailResponseStatus{
		api.BranchDetailResponseStatusCOMINGUP,
		api.BranchDetailResponseStatusACTIVEHEALTHY,
	} {
		gock.New(defaultApiEndpoint).
			Get(branchProjectApiPath).
			Reply(http.StatusNotFound).
			JSON(map[string]string{"message": "Project not found"})
		gock.New(defaultApiEndpoint).
			Get(branchRefApiPath).
			Reply(http.StatusOK).
			JSON(api.BranchDetailResponse{
				Ref:    testBranchRef,
				Status: status,
			})
	}

	synctest.Test(t, func(t *testing.T) {
		client, err := api.NewClientWithResponses(defaultApiEndpoint)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		diags := waitForProjectActive(t.Context(), testBranchRef, client, 5*time.Minute)
		if diags.HasError() {
			t.Errorf("Expected success once branch becomes active, got errors: %v", diags)
		}
	})
}

func TestWaitForProjectActive_BranchRefTerminalState(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(branchProjectApiPath).
		Reply(http.StatusNotFound).
		JSON(map[string]string{"message": "Project not found"})

	gock.New(defaultApiEndpoint).
		Get(branchRefApiPath).
		Reply(http.StatusOK).
		JSON(api.BranchDetailResponse{
			Ref:    testBranchRef,
			Status: api.BranchDetailResponseStatusINITFAILED,
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForProjectActive(t.Context(), testBranchRef, client, 5*time.Minute)
	if !diags.HasError() {
		t.Errorf("Expected error for branch in terminal state, got success")
	}
}

func TestWaitForProjectActive_UnknownRefFails(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(branchProjectApiPath).
		Reply(http.StatusNotFound).
		JSON(map[string]string{"message": "Project not found"})

	gock.New(defaultApiEndpoint).
		Get(branchRefApiPath).
		Reply(http.StatusNotFound).
		JSON(map[string]string{"message": "Branch not found"})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForProjectActive(t.Context(), testBranchRef, client, 5*time.Minute)
	if !diags.HasError() {
		t.Errorf("Expected error for ref unknown to both endpoints, got success")
	}
	// The original project lookup failure is surfaced, not the branch 404.
	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "Project not found") {
		t.Errorf("Expected error to surface the project lookup failure, got: %s", detail)
	}
}

func TestWaitForProjectActive_BranchEndpointErrorFails(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(branchProjectApiPath).
		Reply(http.StatusNotFound).
		JSON(map[string]string{"message": "Project not found"})

	gock.New(defaultApiEndpoint).
		Get(branchRefApiPath).
		Reply(http.StatusInternalServerError).
		JSON(map[string]string{"message": "Internal server error"})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForProjectActive(t.Context(), testBranchRef, client, 5*time.Minute)
	if !diags.HasError() {
		t.Errorf("Expected error when branches endpoint fails, got success")
	}
	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "unexpected status 500") {
		t.Errorf("Expected error to surface the branches endpoint failure, got: %s", detail)
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

	diags := waitForServicesActive(t.Context(), testProjectRef, client, allProjectServices, 5*time.Minute)
	if diags.HasError() {
		t.Errorf("Expected success, got errors: %v", diags)
	}
}

func TestWaitForServicesActive_UnhealthyFailsAfterTimeout(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{
			DbSchema:          "public,storage,graphql_public",
			DbExtraSearchPath: "public,extensions",
			MaxRows:           1000,
		})

	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Persist().
		Reply(http.StatusOK).
		JSON([]api.V1ServiceHealthResponse{
			{Name: api.V1ServiceHealthResponseNameDb, Status: api.UNHEALTHY, Healthy: false, Error: Ptr(`{"error": "fatal"}`)},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		})

	synctest.Test(t, func(t *testing.T) {
		client, err := api.NewClientWithResponses(defaultApiEndpoint)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		start := time.Now()
		// The default timeout is long enough to exceed StateChangeConf's 20 "not found" checks.
		diags := waitForServicesActive(t.Context(), testProjectRef, client, allProjectServices, defaultWaitTimeout)
		if !diags.HasError() {
			t.Fatal("Expected error for unhealthy service, got success")
		}
		if elapsed := time.Since(start); elapsed < defaultWaitTimeout {
			t.Errorf("Expected polling until the timeout, gave up after %s", elapsed)
		}
		if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "unhealthy service db: fatal") {
			t.Errorf("Expected the unhealthy reason in the error, got: %s", detail)
		}
	})
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

func TestWaitForServicesActive_ContinuesWhenPostgrestConfigFails(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusInternalServerError)

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

	diags := waitForServicesActive(t.Context(), testProjectRef, client, allProjectServices, 5*time.Minute)
	if diags.HasError() {
		t.Errorf("Expected success when PostgREST config fails but services are healthy, got errors: %v", diags)
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

	diags := waitForServicesActive(t.Context(), testProjectRef, client, allProjectServices, 5*time.Minute)
	if diags.HasError() {
		t.Errorf("Expected success when data API is disabled, got errors: %v", diags)
	}
	if slices.Contains(capturedServices, string(api.V1GetServicesHealthParamsServicesRest)) {
		t.Errorf("Expected 'rest' to be excluded from services query param, got: %v", capturedServices)
	}
}

func TestWaitForAuthServiceActive_ChecksOnlyAuth(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

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
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForAuthServiceActive(t.Context(), testProjectRef, client, 5*time.Minute)
	if diags.HasError() {
		t.Errorf("Expected success, got errors: %v", diags)
	}
	if !slices.Equal(capturedServices, []string{string(api.V1GetServicesHealthParamsServicesAuth)}) {
		t.Errorf("Expected only auth in services query param, got: %v", capturedServices)
	}
	if gock.HasUnmatchedRequest() {
		t.Errorf("Expected no unmatched requests, got: %v", gock.GetUnmatchedRequests())
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
			{Name: api.V1ServiceHealthResponseNameRealtime, Status: api.UNHEALTHY, Healthy: false, Error: Ptr(`{"error": "Service health check failed"}`)},
		},
		{
			{Name: api.V1ServiceHealthResponseNamePooler, Status: api.ACTIVEHEALTHY, Healthy: true},
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		},
	}

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Times(len(responses)).
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

		diags := waitForServicesActive(t.Context(), testProjectRef, client, allProjectServices, 5*time.Minute)
		if diags.HasError() {
			t.Errorf("Expected success, got errors: %v", diags)
		}
	})

	if !gock.IsDone() {
		t.Fatalf("unconsumed mocks: %v", gock.Pending())
	}
}

func TestWaitForServicesActive_HTTPErrorOmitsEarlierUnhealthyReason(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Times(2).
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
			{Name: api.V1ServiceHealthResponseNameRealtime, Status: api.UNHEALTHY, Healthy: false, Error: Ptr(`{"error": "Service health check failed"}`)},
		})
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		Reply(http.StatusForbidden).
		JSON(map[string]string{"message": "forbidden"})

	synctest.Test(t, func(t *testing.T) {
		client, err := api.NewClientWithResponses(defaultApiEndpoint)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		diags := waitForServicesActive(t.Context(), testProjectRef, client, allProjectServices, 5*time.Minute)
		if !diags.HasError() {
			t.Fatal("Expected error for forbidden response, got success")
		}
		detail := diags.Errors()[0].Detail()
		if !strings.Contains(detail, "unexpected status 403") {
			t.Errorf("Expected the HTTP error in the detail, got: %s", detail)
		}
		if strings.Contains(detail, "realtime") {
			t.Errorf("Expected no stale unhealthy reason for a non-timeout error, got: %s", detail)
		}
	})

	if !gock.IsDone() {
		t.Fatalf("unconsumed mocks: %v", gock.Pending())
	}
}

func TestWaitForServicesActive_NoServicesSkipsHealthCheck(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	diags := waitForServicesActive(t.Context(), testProjectRef, client, nil, 5*time.Minute)
	if diags.HasError() {
		t.Errorf("Expected success, got errors: %v", diags)
	}
	if gock.HasUnmatchedRequest() {
		t.Errorf("Expected no requests, got: %v", gock.GetUnmatchedRequests())
	}
}

func TestWaitForServicesActive_ChecksOnlyGivenServices(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	// No PostgREST config mock: without rest in the list, the Data API lookup must be skipped.
	gock.New(defaultApiEndpoint).
		Get(healthApiPath).
		MatchParam("services", "^auth$").
		Reply(http.StatusOK).
		JSON([]api.V1ServiceHealthResponse{
			{Name: api.V1ServiceHealthResponseNameAuth, Status: api.ACTIVEHEALTHY, Healthy: true},
		})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	services := []api.V1GetServicesHealthParamsServices{api.V1GetServicesHealthParamsServicesAuth}
	diags := waitForServicesActive(t.Context(), testProjectRef, client, services, 5*time.Minute)
	if diags.HasError() {
		t.Errorf("Expected success, got errors: %v", diags)
	}
	if gock.HasUnmatchedRequest() {
		t.Errorf("Expected no unmatched requests, got: %v", gock.GetUnmatchedRequests())
	}
	if !gock.IsDone() {
		t.Fatalf("unconsumed mocks: %v", gock.Pending())
	}
}

func TestWaitForServicesActive_OnlyRestWithDataApiDisabledSkipsHealthCheck(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	gock.New(defaultApiEndpoint).
		Get(postgrestApiPath).
		Reply(http.StatusOK).
		JSON(api.V1PostgrestConfigResponse{DbSchema: ""})

	client, err := api.NewClientWithResponses(defaultApiEndpoint)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	services := []api.V1GetServicesHealthParamsServices{api.V1GetServicesHealthParamsServicesRest}
	diags := waitForServicesActive(t.Context(), testProjectRef, client, services, 5*time.Minute)
	if diags.HasError() {
		t.Errorf("Expected success, got errors: %v", diags)
	}
	if gock.HasUnmatchedRequest() {
		t.Errorf("Expected no health request, got: %v", gock.GetUnmatchedRequests())
	}
	if !gock.IsDone() {
		t.Fatalf("unconsumed mocks: %v", gock.Pending())
	}
}

func TestWaitForProjectAndServicesActive_SharesOneDeadline(t *testing.T) {
	defer gock.OffAll()
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)

	const projectReadyAfter = 4 * time.Minute

	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		projectReady := func(*http.Request, *gock.Request) (bool, error) {
			return time.Since(start) >= projectReadyAfter, nil
		}
		projectNotReady := func(*http.Request, *gock.Request) (bool, error) {
			return time.Since(start) < projectReadyAfter, nil
		}

		// Register health first: gock matches paths as regexes, so the project mocks would also match /health.
		gock.New(defaultApiEndpoint).
			Get(healthApiPath).
			Persist().
			Reply(http.StatusOK).
			JSON([]api.V1ServiceHealthResponse{
				{Name: api.V1ServiceHealthResponseNameAuth, Status: api.UNHEALTHY, Healthy: false, Error: Ptr(`{"error": "Service health check failed"}`)},
			})
		gock.New(defaultApiEndpoint).
			Get(projectApiPath).
			AddMatcher(projectNotReady).
			Persist().
			Reply(http.StatusOK).
			JSON(api.V1ProjectWithDatabaseResponse{Id: testProjectRef, Status: api.V1ProjectWithDatabaseResponseStatusCOMINGUP})
		gock.New(defaultApiEndpoint).
			Get(projectApiPath).
			AddMatcher(projectReady).
			Persist().
			Reply(http.StatusOK).
			JSON(api.V1ProjectWithDatabaseResponse{Id: testProjectRef, Status: api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY})

		client, err := api.NewClientWithResponses(defaultApiEndpoint)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		services := []api.V1GetServicesHealthParamsServices{api.V1GetServicesHealthParamsServicesAuth}
		diags := waitForProjectAndServicesActive(t.Context(), testProjectRef, client, services, start.Add(defaultWaitTimeout))
		if !diags.HasError() {
			t.Fatal("Expected error for unhealthy service, got success")
		}
		if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "unhealthy service auth: Service health check failed") {
			t.Errorf("Expected the unhealthy service in the error, got: %s", detail)
		}
		// Allow for one poll interval past the deadline, but not a second full timeout.
		if elapsed := time.Since(start); elapsed > defaultWaitTimeout+time.Minute {
			t.Errorf("Expected the project and services waits to share %s, took %s", defaultWaitTimeout, elapsed)
		}
	})
}
