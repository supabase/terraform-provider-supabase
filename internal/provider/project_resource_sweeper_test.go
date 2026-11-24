// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	sdkresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
	"github.com/supabase/terraform-provider-supabase/internal/acctest"
)

const (
	projectSweeperName = "supabase_project"
)

func init() {
	sdkresource.AddTestSweepers(projectSweeperName, &sdkresource.Sweeper{
		Name: projectSweeperName,
		F:    sweepProjects,
	})
}

// sweepProjects deletes every project whose name starts with
// acctest.NamePrefix. The Management API is global, so the region passed by
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
		if !strings.HasPrefix(project.Name, acctest.NamePrefix) ||
			project.Status == api.V1ProjectWithDatabaseResponseStatusREMOVED {
			continue
		}

		log.Printf("[INFO] Sweeping %s: deleting project %q (id: %s)", projectSweeperName, project.Name, project.Ref)
		if err := deleteWithResource(ctx, client, NewProjectResource, map[string]any{"id": project.Ref}); err != nil {
			log.Printf("[ERROR] Sweeping %s: failed to delete project %q (id: %s): %s", projectSweeperName, project.Name, project.Ref, err)
			errList = append(errList, fmt.Errorf("project %q (%s): %w", project.Name, project.Ref, err))
			continue
		}
		log.Printf("[INFO] Sweeping %s: deleted project %q (id: %s)", projectSweeperName, project.Name, project.Ref)
	}

	return errors.Join(errList...)
}
