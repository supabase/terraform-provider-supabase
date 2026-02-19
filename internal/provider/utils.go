package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

func Ptr[T any](v T) *T {
	return &v
}

// NullableToString converts an oapi-codegen [nullable.Nullable] to an appropriate
// terraform string type.
func NullableToString[T ~string](n nullable.Nullable[T]) tftypes.String {
	if n.IsSpecified() && !n.IsNull() {
		return tftypes.StringValue(string(n.MustGet()))
	}

	return tftypes.StringNull()
}

const defaultWaitTimeout = 5 * time.Minute

var terminalProjectStatuses = []api.V1ProjectWithDatabaseResponseStatus{
	api.V1ProjectWithDatabaseResponseStatusGOINGDOWN,
	api.V1ProjectWithDatabaseResponseStatusINITFAILED,
	api.V1ProjectWithDatabaseResponseStatusREMOVED,
	api.V1ProjectWithDatabaseResponseStatusINACTIVE,
	api.V1ProjectWithDatabaseResponseStatusPAUSEFAILED,
	api.V1ProjectWithDatabaseResponseStatusRESTOREFAILED,
}

// fails fast on terminal states (GOING_DOWN, INIT_FAILED, REMOVED, etc.) and
// keeps polling on transient states (COMING_UP, RESTORING, ACTIVE_UNHEALTHY, etc.).
func waitForProjectActive(ctx context.Context, projectRef string, client *api.ClientWithResponses, timeout time.Duration) diag.Diagnostics {
	stateConf := &retry.StateChangeConf{
		Pending: []string{
			string(api.V1ProjectWithDatabaseResponseStatusACTIVEUNHEALTHY),
			string(api.V1ProjectWithDatabaseResponseStatusRESTORING),
			string(api.V1ProjectWithDatabaseResponseStatusCOMINGUP),
			string(api.V1ProjectWithDatabaseResponseStatusUPGRADING),
			string(api.V1ProjectWithDatabaseResponseStatusPAUSING),
			string(api.V1ProjectWithDatabaseResponseStatusRESIZING),
			string(api.V1ProjectWithDatabaseResponseStatusRESTARTING),
			string(api.V1ProjectWithDatabaseResponseStatusUNKNOWN),
		},
		Target: []string{
			string(api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY),
		},
		Refresh: func() (any, string, error) {
			httpResp, err := client.V1GetProjectWithResponse(ctx, projectRef)
			if err != nil {
				return nil, "", fmt.Errorf("failed to get project status: %w", err)
			}
			if httpResp.JSON200 == nil {
				return nil, "", fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode(), httpResp.Body)
			}

			status := string(httpResp.JSON200.Status)
			tflog.Debug(ctx, "Waiting for project to become active", map[string]interface{}{
				"project_ref": projectRef,
				"status":      status,
			})

			if slices.Contains(terminalProjectStatuses, httpResp.JSON200.Status) {
				return nil, "", fmt.Errorf("project %s in terminal state: %s", projectRef, status)
			}

			return httpResp.JSON200, status, nil
		},
		Timeout: timeout,
	}

	_, err := stateConf.WaitForStateContext(ctx)
	if err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"Project Not Ready",
			fmt.Sprintf("Project %s did not become active within timeout: %s", projectRef, err),
		)}
	}
	return nil
}

const (
	waitForServicesStatusDone    = "WAIT_FOR_SERVICES_STATUS_DONE"
	waitForServicesStatusPending = "WAIT_FOR_SERVICES_STATUS_PENDING"
)

var allProjectServices = []api.V1GetServicesHealthParamsServices{
	api.V1GetServicesHealthParamsServicesAuth, api.V1GetServicesHealthParamsServicesDb, api.V1GetServicesHealthParamsServicesDbPostgresUser,
	api.V1GetServicesHealthParamsServicesPgBouncer, api.V1GetServicesHealthParamsServicesPooler, api.V1GetServicesHealthParamsServicesRealtime,
	api.V1GetServicesHealthParamsServicesRest, api.V1GetServicesHealthParamsServicesStorage,
}

func waitForServicesActive(ctx context.Context, projectRef string, client *api.ClientWithResponses, timeout time.Duration) diag.Diagnostics {
	stateConf := &retry.StateChangeConf{
		Timeout: timeout,
		Pending: []string{waitForServicesStatusPending},
		Target:  []string{waitForServicesStatusDone},
		Refresh: func() (any, string, error) {
			resp, err := client.V1GetServicesHealthWithResponse(ctx, projectRef, &api.V1GetServicesHealthParams{Services: allProjectServices})
			if err != nil {
				return nil, "", fmt.Errorf("failed to get health information for project services: %w", err)
			}
			if resp.JSON200 == nil {
				return nil, "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode(), resp.Body)
			}
			tflog.Debug(ctx, "Waiting for project services to become active", map[string]any{
				"project_ref": projectRef,
				"status":      resp.JSON200,
			})

			comingupCount := 0
			var errs []error

			for _, v := range *resp.JSON200 {
				switch v.Status {
				case api.UNHEALTHY:
					err := errorFromServiceErrorDescription(v.Name, v.Error)
					// errFailedToRetrieveHealth is always transient; poll again to get more information
					if errors.Is(err, errFailedToRetrieveHealth) {
						tflog.Debug(ctx, "Retrying a recoverable error", map[string]any{
							"error":  err,
							"status": v,
						})
						comingupCount++
						continue
					}
					// pooler reports UNHEALTHY: "not found" for the first couple of seconds of provisioning,
					// so we treat it as COMINGUP
					if v.Name == api.V1ServiceHealthResponseNamePooler && errors.Is(err, errServiceNotFound) {
						tflog.Debug(ctx, "Retrying a recoverable error", map[string]any{
							"error":  err,
							"status": v,
						})
						comingupCount++
						continue
					}
					errs = append(errs, err)
				case api.COMINGUP:
					comingupCount++
				}
			}

			if err := errors.Join(errs...); err != nil {
				return nil, "", err
			}

			if comingupCount > 0 {
				return nil, waitForServicesStatusPending, nil
			}

			return resp.JSON200, waitForServicesStatusDone, nil
		},
	}

	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"Project Services Unhealthy",
			fmt.Sprintf("Project %s services did not become active within timeout: %s", projectRef, err),
		)}
	}
	return nil
}

var (
	errServiceNotFound        = errors.New("not found")
	errFailedToRetrieveHealth = errors.New("failed to retrieve health information for the service")
)

func errorFromServiceErrorDescription(service api.V1ServiceHealthResponseName, desc *string) error {
	if desc == nil {
		return fmt.Errorf("unhealthy service %s: unknown reason", service)
	}

	v := struct {
		Error string `json:"error"`
	}{}
	if err := json.Unmarshal([]byte(*desc), &v); err != nil {
		return fmt.Errorf("unhealthy service %s: %s", service, *desc)
	}

	if v.Error == "not found" {
		return fmt.Errorf("unhealthy service %s: %w", service, errServiceNotFound)
	}

	if v.Error == fmt.Sprintf("Failed to retrieve project's %s service health", service) {
		return fmt.Errorf("unhealthy service %s: %w", service, errFailedToRetrieveHealth)
	}

	return fmt.Errorf("unhealthy service %s: %s", service, v.Error)
}
