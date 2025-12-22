package provider

import (
	"context"
	"fmt"
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

const projectActiveTimeout = 5 * time.Minute

const statusUnknownTransient = "UNKNOWN_TRANSIENT"

var knownProjectStatuses = map[api.V1ProjectWithDatabaseResponseStatus]bool{
	// Target
	api.V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY: true,
	// Pending
	api.V1ProjectWithDatabaseResponseStatusACTIVEUNHEALTHY: true,
	api.V1ProjectWithDatabaseResponseStatusRESTORING:       true,
	api.V1ProjectWithDatabaseResponseStatusCOMINGUP:        true,
	api.V1ProjectWithDatabaseResponseStatusUPGRADING:       true,
	api.V1ProjectWithDatabaseResponseStatusPAUSING:         true,
	api.V1ProjectWithDatabaseResponseStatusRESIZING:        true,
	api.V1ProjectWithDatabaseResponseStatusRESTARTING:      true,
	api.V1ProjectWithDatabaseResponseStatusUNKNOWN:         true,
	// Terminal (handled separately, but included for completeness)
	api.V1ProjectWithDatabaseResponseStatusGOINGDOWN:     true,
	api.V1ProjectWithDatabaseResponseStatusINITFAILED:    true,
	api.V1ProjectWithDatabaseResponseStatusREMOVED:       true,
	api.V1ProjectWithDatabaseResponseStatusINACTIVE:      true,
	api.V1ProjectWithDatabaseResponseStatusPAUSEFAILED:   true,
	api.V1ProjectWithDatabaseResponseStatusRESTOREFAILED: true,
}

// fails fast on terminal states (GOING_DOWN, INIT_FAILED, REMOVED, etc.) and
// keeps polling on transient states (COMING_UP, RESTORING, ACTIVE_UNHEALTHY, etc.).
func waitForProjectActive(ctx context.Context, projectRef string, client *api.ClientWithResponses) diag.Diagnostics {
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
			statusUnknownTransient,
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

			switch httpResp.JSON200.Status {
			case api.V1ProjectWithDatabaseResponseStatusGOINGDOWN,
				api.V1ProjectWithDatabaseResponseStatusINITFAILED,
				api.V1ProjectWithDatabaseResponseStatusREMOVED,
				api.V1ProjectWithDatabaseResponseStatusINACTIVE,
				api.V1ProjectWithDatabaseResponseStatusPAUSEFAILED,
				api.V1ProjectWithDatabaseResponseStatusRESTOREFAILED:
				return nil, "", fmt.Errorf("project %s in terminal state: %s", projectRef, status)
			}

			if !knownProjectStatuses[httpResp.JSON200.Status] {
				tflog.Warn(ctx, "Unrecognized project status, treating as transient", map[string]interface{}{
					"project_ref": projectRef,
					"status":      status,
				})
				return httpResp.JSON200, statusUnknownTransient, nil
			}

			return httpResp.JSON200, status, nil
		},
		Timeout: projectActiveTimeout,
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
