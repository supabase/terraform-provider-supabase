// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdkresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
)

const (
	// sweepProjectNamePrefix limits the project sweeper to projects created by
	// acceptance tests, so that it never deletes real projects in the account.
	sweepProjectNamePrefix = "tf-acc-test"

	projectSweeperName = "supabase_project"
)

func init() {
	sdkresource.AddTestSweepers(projectSweeperName, &sdkresource.Sweeper{
		Name: projectSweeperName,
		F:    sweepProjects,
	})
}

// sweepProjects deletes every project whose name starts with
// sweepProjectNamePrefix. The Management API is global, so the region passed by
// the sweeper runner is ignored. It keeps going after a failed delete and
// returns all failures together.
func sweepProjects(_ string) error {
	ctx := context.Background()

	client, err := sweeperClient()
	if err != nil {
		return err
	}

	listResp, err := client.V1ListAllProjectsWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("unable to list projects, got error: %w", err)
	}
	if listResp.JSON200 == nil {
		return fmt.Errorf("unable to list projects, got status %d: %s", listResp.StatusCode(), listResp.Body)
	}

	var errList []error
	for _, project := range *listResp.JSON200 {
		if !strings.HasPrefix(project.Name, sweepProjectNamePrefix) ||
			project.Status == api.V1ProjectWithDatabaseResponseStatusREMOVED {
			continue
		}

		if err := deleteWithResource(ctx, client, NewProjectResource, map[string]any{"id": project.Ref}); err != nil {
			errList = append(errList, fmt.Errorf("project %q (%s): %w", project.Name, project.Ref, err))
		}
	}

	return errors.Join(errList...)
}
