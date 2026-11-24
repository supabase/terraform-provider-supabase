package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	sdkresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/supabase/cli/pkg/api"
)

// TestMain enables the -sweep flags so registered sweepers can be run.
func TestMain(m *testing.M) {
	sdkresource.TestMain(m)
}

// sweeperClient builds an API client from the same environment variables the
// provider reads.
func sweeperClient() (*api.ClientWithResponses, error) {
	token := strings.TrimSpace(os.Getenv("SUPABASE_ACCESS_TOKEN"))
	if token == "" {
		return nil, errors.New("SUPABASE_ACCESS_TOKEN must be set to run sweepers")
	}

	endpoint := strings.TrimSpace(os.Getenv("SUPABASE_API_ENDPOINT"))
	if endpoint == "" {
		endpoint = defaultApiEndpoint
	}

	client, err := api.NewClientWithResponses(
		endpoint,
		api.WithHTTPClient(newRetryableClient(nil)),
		api.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		}),
		api.WithRequestEditorFn(stashRequestMethod),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create API client: %w", err)
	}

	return client, nil
}

// deleteWithResource runs the Delete method of the resource built by newResource
// against a state holding attrs, so sweepers exercise the same code path as
// terraform destroy. attrs maps top-level attribute names to the values Delete
// needs to identify the object, such as "id" or "project_ref".
func deleteWithResource(ctx context.Context, client *api.ClientWithResponses, newResource func() resource.Resource, attrs map[string]any) error {
	r := newResource()

	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		return fmt.Errorf("%T does not implement resource.ResourceWithConfigure", r)
	}
	var configResp resource.ConfigureResponse
	configurable.Configure(ctx, resource.ConfigureRequest{ProviderData: client}, &configResp)
	if configResp.Diagnostics.HasError() {
		return fmt.Errorf("unable to configure resource: %v", configResp.Diagnostics)
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}
	for name, value := range attrs {
		if diags := state.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			return fmt.Errorf("unable to set %q in state: %v", name, diags)
		}
	}

	resp := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		return fmt.Errorf("unable to delete: %v", resp.Diagnostics)
	}

	return nil
}
