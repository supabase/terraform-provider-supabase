package provider

import (
	"context"
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

const projectActiveTimeout = 5 * time.Minute

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
