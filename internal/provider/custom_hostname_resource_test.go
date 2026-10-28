// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/examples"
	"gopkg.in/h2non/gock.v1"
)

const (
	testCustomHostname  = "api.example.com"
	testCustomHostname2 = "api2.example.com"
	testHostnameID      = "cf-hostname-id"
)

func customHostnameOrigin() string {
	return testProjectRef + ".supabase.co"
}

func customHostnameAPIResponse(hostname, status string) map[string]any {
	sslStatus := "pending_validation"
	hostnameStatus := "pending"
	verificationErrors := []any{"dns not propagated"}
	validationErrors := []any{
		map[string]any{"message": "certificate pending"},
	}
	if status == customHostnameActiveStatus {
		sslStatus = "active"
		hostnameStatus = "active"
		verificationErrors = []any{}
		validationErrors = []any{}
	}

	return map[string]any{
		"status":          status,
		"custom_hostname": hostname,
		"data": map[string]any{
			"success":  true,
			"errors":   []any{},
			"messages": []any{},
			"result": map[string]any{
				"id":                   testHostnameID,
				"hostname":             hostname,
				"custom_origin_server": customHostnameOrigin(),
				"status":               hostnameStatus,
				"ownership_verification": map[string]any{
					"type":  "txt",
					"name":  "_cf-custom-hostname." + hostname,
					"value": "ownership-token",
				},
				"ssl": map[string]any{
					"status": sslStatus,
					"validation_records": []any{
						map[string]any{
							"txt_name":  "_acme-challenge." + hostname,
							"txt_value": "validation-token",
						},
					},
					"validation_errors": validationErrors,
				},
				"verification_errors": verificationErrors,
			},
		},
	}
}

func customHostnameConfig(hostname string, activate *bool) string {
	return customHostnameNamedConfig("test", hostname, activate)
}

func customHostnameNamedConfig(name, hostname string, activate *bool) string {
	activateLine := ""
	if activate != nil {
		activateLine = fmt.Sprintf("\n  activate        = %t", *activate)
	}
	return fmt.Sprintf(`
resource "supabase_custom_hostname" %s {
  project_ref     = %q
  custom_hostname = %q%s
}
`, name, testProjectRef, hostname, activateLine)
}

func mockInitializeCustomHostname(t *testing.T, hostname string, response map[string]any) {
	t.Helper()

	gock.New(defaultApiEndpoint).
		Post(customHostnameInitApiPath).
		AddMatcher(matchJSONBody(t, map[string]any{
			"custom_hostname": hostname,
		})).
		Reply(http.StatusCreated).
		JSON(response)
}

func mockGetCustomHostname(response map[string]any) {
	gock.New(defaultApiEndpoint).
		Get(customHostnameApiPath).
		Reply(http.StatusOK).
		JSON(response)
}

func mockGetCustomHostnameTimes(response map[string]any, times int) {
	gock.New(defaultApiEndpoint).
		Get(customHostnameApiPath).
		Times(times).
		Reply(http.StatusOK).
		JSON(response)
}

func mockReverifyCustomHostname(response map[string]any) {
	gock.New(defaultApiEndpoint).
		Post(customHostnameReverifyPath).
		Reply(http.StatusCreated).
		JSON(response)
}

func mockActivateCustomHostname(response map[string]any) {
	gock.New(defaultApiEndpoint).
		Post(customHostnameActivatePath).
		Reply(http.StatusCreated).
		JSON(response)
}

func mockDeleteCustomHostname(status int) {
	gock.New(defaultApiEndpoint).
		Delete(customHostnameApiPath).
		Reply(status)
}

func requireGockDone(t *testing.T) {
	t.Helper()

	if os.Getenv("TF_ACC") == "" || gock.IsDone() {
		return
	}
	for _, mock := range gock.Pending() {
		req := mock.Request()
		t.Errorf("pending mock: %s %s", req.Method, req.URLStruct.String())
	}
}

func checkCustomHostname(resourceName, hostname, status, activate string) resource.TestCheckFunc {
	sslStatus := "pending_validation"
	hostnameStatus := "pending"
	verificationError := "dns not propagated"
	validationError := "certificate pending"
	if status == customHostnameActiveStatus {
		sslStatus = "active"
		hostnameStatus = "active"
		verificationError = ""
		validationError = ""
	}

	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(resourceName, "id", testProjectRef),
		resource.TestCheckResourceAttr(resourceName, "project_ref", testProjectRef),
		resource.TestCheckResourceAttr(resourceName, "custom_hostname", hostname),
		resource.TestCheckResourceAttr(resourceName, "activate", activate),
		resource.TestCheckResourceAttr(resourceName, "status", status),
		resource.TestCheckResourceAttr(resourceName, "hostname_id", testHostnameID),
		resource.TestCheckResourceAttr(resourceName, "hostname_status", hostnameStatus),
		resource.TestCheckResourceAttr(resourceName, "custom_origin_server", customHostnameOrigin()),
		resource.TestCheckResourceAttr(resourceName, "ownership_verification.type", "txt"),
		resource.TestCheckResourceAttr(resourceName, "ownership_verification.name", "_cf-custom-hostname."+hostname),
		resource.TestCheckResourceAttr(resourceName, "ownership_verification.value", "ownership-token"),
		resource.TestCheckResourceAttr(resourceName, "ssl.status", sslStatus),
		resource.TestCheckResourceAttr(resourceName, "ssl.validation_records.#", "1"),
		resource.TestCheckResourceAttr(resourceName, "ssl.validation_records.0.txt_name", "_acme-challenge."+hostname),
		resource.TestCheckResourceAttr(resourceName, "ssl.validation_records.0.txt_value", "validation-token"),
	}
	if verificationError == "" {
		checks = append(checks, resource.TestCheckResourceAttr(resourceName, "verification_errors.#", "0"))
		checks = append(checks, resource.TestCheckResourceAttr(resourceName, "ssl.validation_errors.#", "0"))
	} else {
		checks = append(checks, resource.TestCheckResourceAttr(resourceName, "verification_errors.0", verificationError))
		checks = append(checks, resource.TestCheckResourceAttr(resourceName, "ssl.validation_errors.0", validationError))
	}
	return resource.ComposeAggregateTestCheckFunc(checks...)
}

func TestAccCustomHostnameResource(t *testing.T) {
	defer gock.OffAll()
	defer requireGockDone(t)

	initial := customHostnameAPIResponse(testCustomHostname, string(api.N2Initiated))
	updated := customHostnameAPIResponse(testCustomHostname2, string(api.N2Initiated))

	mockInitializeCustomHostname(t, testCustomHostname, initial)
	// Refresh after create, import, and the refresh before the hostname update.
	mockGetCustomHostnameTimes(initial, 4)
	mockInitializeCustomHostname(t, testCustomHostname2, updated)
	mockGetCustomHostname(updated)
	mockDeleteCustomHostname(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: examples.CustomHostnameResourceConfig,
				Check:  checkCustomHostname("supabase_custom_hostname.example", testCustomHostname, string(api.N2Initiated), "false"),
			},
			{
				ResourceName:      "supabase_custom_hostname.example",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     testProjectRef,
			},
			{
				Config: customHostnameNamedConfig("example", testCustomHostname2, nil),
				Check:  checkCustomHostname("supabase_custom_hostname.example", testCustomHostname2, string(api.N2Initiated), "false"),
			},
		},
	})
}

func TestAccCustomHostnameResource_Activate(t *testing.T) {
	defer gock.OffAll()
	defer requireGockDone(t)

	initiated := customHostnameAPIResponse(testCustomHostname, string(api.N2Initiated))
	verified := customHostnameAPIResponse(testCustomHostname, string(api.N4OriginSetupCompleted))
	active := customHostnameAPIResponse(testCustomHostname, customHostnameActiveStatus)
	renamed := customHostnameAPIResponse(testCustomHostname2, string(api.N2Initiated))
	renamedActive := customHostnameAPIResponse(testCustomHostname2, customHostnameActiveStatus)
	activate := true

	mockInitializeCustomHostname(t, testCustomHostname, initiated)
	mockReverifyCustomHostname(verified)
	mockActivateCustomHostname(active)
	mockGetCustomHostnameTimes(active, 2)
	mockInitializeCustomHostname(t, testCustomHostname2, renamed)
	mockReverifyCustomHostname(renamedActive)
	mockGetCustomHostnameTimes(renamedActive, 3)
	mockDeleteCustomHostname(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: customHostnameConfig(testCustomHostname, &activate),
				Check:  checkCustomHostname("supabase_custom_hostname.test", testCustomHostname, customHostnameActiveStatus, "true"),
			},
			{
				Config: customHostnameConfig(testCustomHostname2, &activate),
				Check:  checkCustomHostname("supabase_custom_hostname.test", testCustomHostname2, customHostnameActiveStatus, "true"),
			},
			{
				Config: customHostnameConfig(testCustomHostname2, nil),
				Check:  checkCustomHostname("supabase_custom_hostname.test", testCustomHostname2, customHostnameActiveStatus, "false"),
			},
		},
	})
}

func TestAccCustomHostnameResource_PendingActivation(t *testing.T) {
	defer gock.OffAll()
	defer requireGockDone(t)

	initiated := customHostnameAPIResponse(testCustomHostname, string(api.N2Initiated))
	pending := customHostnameAPIResponse(testCustomHostname, string(api.N4OriginSetupCompleted))
	active := customHostnameAPIResponse(testCustomHostname, customHostnameActiveStatus)
	activate := true

	mockInitializeCustomHostname(t, testCustomHostname, initiated)
	mockReverifyCustomHostname(pending)
	mockActivateCustomHostname(pending)
	mockGetCustomHostnameTimes(pending, 3)
	mockReverifyCustomHostname(active)
	mockGetCustomHostname(active)
	mockDeleteCustomHostname(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             customHostnameConfig(testCustomHostname, &activate),
				Check:              checkCustomHostname("supabase_custom_hostname.test", testCustomHostname, string(api.N4OriginSetupCompleted), "true"),
				ExpectNonEmptyPlan: true,
			},
			{
				Config:             customHostnameConfig(testCustomHostname, &activate),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: customHostnameConfig(testCustomHostname, &activate),
				Check:  checkCustomHostname("supabase_custom_hostname.test", testCustomHostname, customHostnameActiveStatus, "true"),
			},
		},
	})
}

func TestAccCustomHostnameResource_SkipsReverifyWhenActive(t *testing.T) {
	defer gock.OffAll()
	defer requireGockDone(t)

	initiated := customHostnameAPIResponse(testCustomHostname, string(api.N2Initiated))
	active := customHostnameAPIResponse(testCustomHostname, customHostnameActiveStatus)
	activate := true

	mockInitializeCustomHostname(t, testCustomHostname, initiated)
	mockGetCustomHostname(initiated)
	mockGetCustomHostname(active)
	mockGetCustomHostname(active)
	mockDeleteCustomHostname(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: customHostnameConfig(testCustomHostname, nil),
				Check:  checkCustomHostname("supabase_custom_hostname.test", testCustomHostname, string(api.N2Initiated), "false"),
			},
			{
				Config: customHostnameConfig(testCustomHostname, &activate),
				Check:  checkCustomHostname("supabase_custom_hostname.test", testCustomHostname, customHostnameActiveStatus, "true"),
			},
		},
	})
}

func TestAccCustomHostnameResource_ReadRemovesMissing(t *testing.T) {
	defer gock.OffAll()
	defer requireGockDone(t)

	initial := customHostnameAPIResponse(testCustomHostname, string(api.N2Initiated))

	mockInitializeCustomHostname(t, testCustomHostname, initial)
	mockGetCustomHostname(initial)
	gock.New(defaultApiEndpoint).
		Get(customHostnameApiPath).
		Reply(http.StatusNotFound).
		JSON(map[string]any{"message": "not found"})
	mockDeleteCustomHostname(http.StatusNotFound)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: customHostnameConfig(testCustomHostname, nil),
				Check:  resource.TestCheckResourceAttr("supabase_custom_hostname.test", "id", testProjectRef),
			},
			{
				Config:             customHostnameConfig(testCustomHostname, nil),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccCustomHostnameResource_DeleteNotFound(t *testing.T) {
	defer gock.OffAll()
	defer requireGockDone(t)

	initial := customHostnameAPIResponse(testCustomHostname, string(api.N2Initiated))

	mockInitializeCustomHostname(t, testCustomHostname, initial)
	mockGetCustomHostname(initial)
	mockDeleteCustomHostname(http.StatusNotFound)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: customHostnameConfig(testCustomHostname, nil),
				Check:  resource.TestCheckResourceAttr("supabase_custom_hostname.test", "custom_hostname", testCustomHostname),
			},
		},
	})
}

func TestAccCustomHostnameResource_LowercaseAndInvalid(t *testing.T) {
	defer gock.OffAll()
	defer requireGockDone(t)

	initial := customHostnameAPIResponse(testCustomHostname, string(api.N2Initiated))
	mockInitializeCustomHostname(t, "API.Example.COM", initial)
	mockGetCustomHostnameTimes(initial, 3)
	mockDeleteCustomHostname(http.StatusOK)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      customHostnameConfig("https://api.example.com", nil),
				ExpectError: regexp.MustCompile("custom_hostname must be a DNS hostname"),
			},
			{
				Config: customHostnameConfig("API.Example.COM", nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supabase_custom_hostname.test", "custom_hostname", "API.Example.COM"),
					resource.TestCheckResourceAttr("supabase_custom_hostname.test", "ownership_verification.name", "_cf-custom-hostname."+testCustomHostname),
				),
			},
			{
				Config: customHostnameConfig(testCustomHostname, nil),
				Check:  resource.TestCheckResourceAttr("supabase_custom_hostname.test", "custom_hostname", testCustomHostname),
			},
		},
	})
}

func TestAccCustomHostnameResource_InitializeRateLimit(t *testing.T) {
	defer gock.OffAll()
	defer requireGockDone(t)

	gock.New(defaultApiEndpoint).
		Post(customHostnameInitApiPath).
		Reply(http.StatusTooManyRequests).
		JSON(map[string]any{"message": "rate limit exceeded"})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      customHostnameConfig(testCustomHostname, nil),
				ExpectError: regexp.MustCompile("10 requests per minute"),
			},
		},
	})
}

func TestSetCustomHostnameState(t *testing.T) {
	data := &CustomHostnameResourceModel{
		ProjectRef:     types.StringValue(testProjectRef),
		CustomHostname: types.StringValue(testCustomHostname),
		Activate:       types.BoolValue(false),
	}

	resp := api.UpdateCustomHostnameResponse{
		CustomHostname: testCustomHostname,
		Status:         api.N2Initiated,
	}
	resp.Data.Result.Id = testHostnameID
	resp.Data.Result.Hostname = testCustomHostname
	resp.Data.Result.CustomOriginServer = customHostnameOrigin()
	resp.Data.Result.Status = "pending"
	resp.Data.Result.OwnershipVerification.Type = "txt"
	resp.Data.Result.OwnershipVerification.Name = "_cf-custom-hostname." + testCustomHostname
	resp.Data.Result.OwnershipVerification.Value = "ownership-token"
	resp.Data.Result.Ssl.Status = "pending_validation"
	resp.Data.Result.Ssl.ValidationRecords = append(resp.Data.Result.Ssl.ValidationRecords, struct {
		TxtName  string `json:"txt_name"`
		TxtValue string `json:"txt_value"`
	}{
		TxtName:  "_acme-challenge." + testCustomHostname,
		TxtValue: "validation-token",
	})
	validationErrors := []struct {
		Message string `json:"message"`
	}{{Message: "certificate pending"}}
	resp.Data.Result.Ssl.ValidationErrors = &validationErrors
	verificationErrors := []string{"dns not propagated"}
	resp.Data.Result.VerificationErrors = &verificationErrors

	diags := setCustomHostnameState(data, resp)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if data.Id.ValueString() != testProjectRef {
		t.Fatalf("id = %s", data.Id.ValueString())
	}
	if data.OwnershipVerification.IsNull() {
		t.Fatal("expected ownership verification")
	}
	if data.SSL.IsNull() {
		t.Fatal("expected ssl")
	}
	if data.VerificationErrors.IsNull() {
		t.Fatal("expected verification errors")
	}

	empty := &CustomHostnameResourceModel{
		ProjectRef:     types.StringValue(testProjectRef),
		CustomHostname: types.StringValue("kept.example.com"),
	}
	emptyResp := api.UpdateCustomHostnameResponse{Status: api.N1NotStarted}
	emptyResp.Data.Result.Hostname = "kept.example.com"
	diags = setCustomHostnameState(empty, emptyResp)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if empty.CustomHostname.ValueString() != "kept.example.com" {
		t.Fatalf("custom_hostname = %s", empty.CustomHostname.ValueString())
	}
	if !empty.OwnershipVerification.IsNull() {
		t.Fatal("expected null ownership verification")
	}
	if !empty.SSL.IsNull() {
		t.Fatal("expected null ssl")
	}
	if empty.VerificationErrors.IsNull() {
		t.Fatal("expected empty verification error list")
	}
}

func TestCustomHostnameIsActive(t *testing.T) {
	if customHostnameIsActive(types.StringNull()) {
		t.Fatal("null status is not active")
	}
	if customHostnameIsActive(types.StringUnknown()) {
		t.Fatal("unknown status is not active")
	}
	if customHostnameIsActive(types.StringValue(string(api.N2Initiated))) {
		t.Fatal("initiated status is not active")
	}
	if !customHostnameIsActive(types.StringValue(customHostnameActiveStatus)) {
		t.Fatal("expected active status")
	}
}

func TestHostnamesDiffer(t *testing.T) {
	t.Parallel()

	original := types.StringValue(testCustomHostname)
	if hostnamesDiffer(original, types.StringValue("API.Example.COM")) {
		t.Fatal("hostname comparison should ignore case")
	}
	if !hostnamesDiffer(original, types.StringValue(testCustomHostname2)) {
		t.Fatal("expected different hostnames to differ")
	}
	if !hostnamesDiffer(original, types.StringNull()) {
		t.Fatal("expected null hostname to differ")
	}
}
