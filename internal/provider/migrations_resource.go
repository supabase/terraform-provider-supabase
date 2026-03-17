package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/supabase/cli/pkg/api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &MigrationsResource{}
	_ resource.ResourceWithImportState = &MigrationsResource{}
)

// migrationDigestsPlanModifier computes SHA-256 digests from migration file contents during plan phase
// to make digests known before apply.
type migrationDigestsPlanModifier struct{}

func (m migrationDigestsPlanModifier) Description(_ context.Context) string {
	return "Computes SHA-256 digests from migration file contents during plan to detect changes."
}

func (m migrationDigestsPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m migrationDigestsPlanModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	// If the entire plan is null (resource being destroyed), nothing to do
	if req.Plan.Raw.IsNull() {
		return
	}

	var migrationsDir types.String
	diags := req.Plan.GetAttribute(ctx, path.Root("migrations_dir"), &migrationsDir)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if migrationsDir.IsUnknown() || migrationsDir.IsNull() {
		return
	}

	dirPath := migrationsDir.ValueString()
	if strings.TrimSpace(dirPath) == "" {
		resp.Diagnostics.AddError(
			"Invalid migrations directory",
			"`migrations_dir` must be a non-empty path to a directory containing .sql files.",
		)
		return
	}

	// Get state migrations to check which ones already exist
	var stateMigrations types.List
	diags = req.State.GetAttribute(ctx, path.Root("migrations"), &stateMigrations)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var stateMigrationModels []MigrationModel
	if !stateMigrations.IsNull() && !stateMigrations.IsUnknown() {
		diags = stateMigrations.ElementsAs(ctx, &stateMigrationModels, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read migrations directory",
			fmt.Sprintf("Could not read migrations directory '%s': %s", dirPath, err.Error()),
		)
		return
	}

	var migrationFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if strings.EqualFold(filepath.Ext(entry.Name()), ".sql") {
			migrationFiles = append(migrationFiles, entry.Name())
		}
	}

	if len(migrationFiles) == 0 {
		resp.Diagnostics.AddError(
			"No migration files found",
			fmt.Sprintf("No .sql files were found in migrations directory '%s'.", dirPath),
		)
		return
	}

	sort.Strings(migrationFiles)

	var planMigrationModels []MigrationModel
	for _, migrationFile := range migrationFiles {
		planMigrationModels = append(planMigrationModels, MigrationModel{
			FilePath: types.StringValue(filepath.Join(dirPath, migrationFile)),
			Name:     types.StringValue(migrationFile),
		})
	}

	// Compute digests and update the plan
	var updated []MigrationModel
	for i, planMigration := range planMigrationModels {
		// For migrations that already exist in state (same index with computed fields),
		// preserve the state values instead of recomputing
		// This prevents sensitive attribute inconsistency errors
		if i < len(stateMigrationModels) {
			stateMigration := stateMigrationModels[i]
			// If state migration has computed fields populated, preserve them
			if !stateMigration.Content.IsNull() && !stateMigration.Digest.IsNull() {
				// Still verify that file hasn't changed by recomputing digest
				filePath := planMigration.FilePath.ValueString()
				if !planMigration.FilePath.IsUnknown() && !planMigration.FilePath.IsNull() {
					content, err := os.ReadFile(filePath)
					if err != nil {
						resp.Diagnostics.AddError(
							"Failed to read migration file",
							fmt.Sprintf("Could not read migration file '%s': %s", filePath, err.Error()),
						)
						return
					}

					// Check if content has changed
					hash := sha256.Sum256(content)
					newDigest := hex.EncodeToString(hash[:])
					stateDigest := stateMigration.Digest.ValueString()

					if newDigest != stateDigest {
						// File changed - this will be caught by Update validation
						// Compute new values for comparison
						planMigration.Content = types.StringValue(string(content))
						planMigration.Digest = types.StringValue(newDigest)
					} else {
						// File hasn't changed - preserve state values
						planMigration.Content = stateMigration.Content
						planMigration.Digest = stateMigration.Digest
						planMigration.Name = stateMigration.Name
					}
				} else {
					// Can't read file, preserve state
					planMigration.Content = stateMigration.Content
					planMigration.Digest = stateMigration.Digest
					planMigration.Name = stateMigration.Name
				}
				updated = append(updated, planMigration)
				continue
			}
		}

		// This is a new migration or doesn't have computed state yet

		filePath := planMigration.FilePath.ValueString()

		// Read file and compute digest at plan time
		content, err := os.ReadFile(filePath)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to read migration file",
				fmt.Sprintf("Could not read migration file '%s': %s", filePath, err.Error()),
			)
			return
		}

		// Store plaintext content (marked sensitive)
		planMigration.Content = types.StringValue(string(content))

		// Compute SHA-256 digest
		hash := sha256.Sum256(content)
		digest := hex.EncodeToString(hash[:])
		planMigration.Digest = types.StringValue(digest)

		// Extract name from file path (base name without extension)
		name := filepath.Base(filePath)
		if planMigration.Name.IsNull() || planMigration.Name.ValueString() == "" {
			planMigration.Name = types.StringValue(name)
		}

		updated = append(updated, planMigration)
	}

	// Build the updated list value
	migrationElemType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"file_path": types.StringType,
			"name":      types.StringType,
			"content":   types.StringType,
			"digest":    types.StringType,
		},
	}

	var values []attr.Value
	for _, mig := range updated {
		obj, objDiags := types.ObjectValue(
			migrationElemType.AttrTypes,
			map[string]attr.Value{
				"file_path": mig.FilePath,
				"name":      mig.Name,
				"content":   mig.Content,
				"digest":    mig.Digest,
			},
		)
		resp.Diagnostics.Append(objDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		values = append(values, obj)
	}

	updatedList, listDiags := types.ListValue(migrationElemType, values)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.PlanValue = updatedList
}

func NewMigrationsResource() resource.Resource {
	return &MigrationsResource{}
}

type MigrationsResource struct {
	client *api.ClientWithResponses
}

type MigrationsResourceModel struct {
	ProjectRef    types.String `tfsdk:"project_ref"`
	MigrationsDir types.String `tfsdk:"migrations_dir"`
	Migrations    types.List   `tfsdk:"migrations"` // Computed list of MigrationModel
	Id            types.String `tfsdk:"id"`
}

type MigrationModel struct {
	FilePath types.String `tfsdk:"file_path"` // Required: path to migration file
	Name     types.String `tfsdk:"name"`      // Computed: migration name (from file or API)
	Content  types.String `tfsdk:"content"`   // Computed: plaintext SQL (sensitive)
	Digest   types.String `tfsdk:"digest"`    // Computed: SHA-256 of content
}

func (r *MigrationsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_migrations"
}

func (r *MigrationsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Applies a list of imperative SQL migration files to a Supabase project database. " +
			"Migrations are applied sequentially in order. Once applied, migrations cannot be rolled back. " +
			"Updates only allow appending new migrations to the list. Modifying or reordering existing migrations will cause an error. " +
			"**Warning**: Deleting this resource removes it from Terraform state but does not roll back applied migrations.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Internal identifier for this resource (same as project_ref).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project ref identifier for the Supabase project.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"migrations_dir": schema.StringAttribute{
				MarkdownDescription: "Path to a directory containing ordered migration SQL files (.sql). Files are sorted lexicographically and applied in that order.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"migrations": schema.ListNestedAttribute{
				MarkdownDescription: "Computed ordered list of migration files discovered in `migrations_dir`. Existing applied migrations are immutable.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"file_path": schema.StringAttribute{
							MarkdownDescription: "Path to the migration SQL file (under `migrations_dir`).",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the migration (same as file name).",
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"content": schema.StringAttribute{
							MarkdownDescription: "SQL content of the migration file computed at plan time.",
							Computed:            true,
							Sensitive:           true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"digest": schema.StringAttribute{
							MarkdownDescription: "SHA-256 digest of the migration content computed at plan time.",
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
					},
				},
				PlanModifiers: []planmodifier.List{
					migrationDigestsPlanModifier{},
					listplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *MigrationsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		r.client = client
	}
}

func (r *MigrationsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MigrationsResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set ID to project_ref
	plan.Id = plan.ProjectRef

	// Extract migration models from the plan
	var planMigrations []MigrationModel
	resp.Diagnostics.Append(plan.Migrations.ElementsAs(ctx, &planMigrations, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Wait for project to be active before applying migrations
	waitDiags := waitForProjectActive(ctx, plan.ProjectRef.ValueString(), r.client)
	resp.Diagnostics.Append(waitDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Apply migrations sequentially
	var appliedMigrations []MigrationModel
	for i, migration := range planMigrations {
		tflog.Debug(ctx, "Applying migration", map[string]interface{}{
			"index": i,
			"name":  migration.Name.ValueString(),
			"path":  migration.FilePath.ValueString(),
		})

		// Apply the migration
		applied, applyDiags := r.applyMigration(ctx, plan.ProjectRef.ValueString(), migration)
		resp.Diagnostics.Append(applyDiags...)

		if resp.Diagnostics.HasError() {
			// On error, preserve state for successfully applied migrations
			if len(appliedMigrations) > 0 {
				plan.Migrations = r.buildMigrationsList(appliedMigrations, &resp.Diagnostics)
				resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			}
			return
		}

		appliedMigrations = append(appliedMigrations, applied)
	}

	// Update plan with applied migrations
	plan.Migrations = r.buildMigrationsList(appliedMigrations, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *MigrationsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MigrationsResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if project still exists
	projectResp, err := r.client.V1GetProjectWithResponse(ctx, state.ProjectRef.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read project, got error: %s", err),
		)
		return
	}

	if projectResp.StatusCode() == 404 {
		// Project no longer exists, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	if projectResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Unexpected API Response",
			fmt.Sprintf("Expected JSON200 response, got status %d: %s", projectResp.StatusCode(), string(projectResp.Body)),
		)
		return
	}

	// TODO: If API provides a way to list applied migrations, fetch and reconcile state here
	// For now, we trust the state as the source of truth for applied migrations
	// This is acceptable because migrations are append-only and immutable once applied

	tflog.Debug(ctx, "Read migrations resource", map[string]interface{}{
		"project_ref": state.ProjectRef.ValueString(),
	})

	// Save updated state
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *MigrationsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state MigrationsResourceModel

	// Read Terraform plan and state
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Extract migrations from plan and state
	var planMigrations, stateMigrations []MigrationModel
	resp.Diagnostics.Append(plan.Migrations.ElementsAs(ctx, &planMigrations, false)...)
	resp.Diagnostics.Append(state.Migrations.ElementsAs(ctx, &stateMigrations, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate update: only appending is allowed
	if len(planMigrations) < len(stateMigrations) {
		resp.Diagnostics.AddError(
			"Invalid Migration Update",
			"Cannot remove migrations from the list. Migrations are immutable once applied.",
		)
		return
	}

	// Verify existing migrations haven't changed
	for i := 0; i < len(stateMigrations); i++ {
		stateDigest := stateMigrations[i].Digest.ValueString()
		planDigest := planMigrations[i].Digest.ValueString()

		if stateDigest != planDigest {
			resp.Diagnostics.AddError(
				"Invalid Migration Update",
				fmt.Sprintf("Cannot modify existing migration at index %d (name: %s). "+
					"Existing digest: %s, new digest: %s. "+
					"Migrations are immutable once applied. To change migrations, create a new resource.",
					i,
					stateMigrations[i].Name.ValueString(),
					stateDigest,
					planDigest,
				),
			)
			return
		}
	}

	// Wait for project to be active
	waitDiags := waitForProjectActive(ctx, plan.ProjectRef.ValueString(), r.client)
	resp.Diagnostics.Append(waitDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Apply only new migrations (from len(stateMigrations) onwards)
	for i := len(stateMigrations); i < len(planMigrations); i++ {
		migration := planMigrations[i]

		tflog.Debug(ctx, "Applying new migration", map[string]interface{}{
			"index": i,
			"name":  migration.Name.ValueString(),
			"path":  migration.FilePath.ValueString(),
		})

		// Apply the migration
		_, applyDiags := r.applyMigration(ctx, plan.ProjectRef.ValueString(), migration)
		resp.Diagnostics.Append(applyDiags...)

		if resp.Diagnostics.HasError() {
			// On error, we would need to save partial state, but for now just return error
			return
		}
	}

	// Save plan directly to state - the plan already has all the correct values
	// from the plan modifier (including content, digest, etc.)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *MigrationsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MigrationsResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Migrations cannot be rolled back - this is a state-only operation
	tflog.Warn(ctx, "Deleting migrations resource from state only - applied migrations cannot be rolled back", map[string]interface{}{
		"project_ref": state.ProjectRef.ValueString(),
	})

	// Resource is removed from state automatically after Delete returns without error
}

func (r *MigrationsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import using project_ref as the ID
	projectRef := req.ID

	// Verify project exists
	projectResp, err := r.client.V1GetProjectWithResponse(ctx, projectRef)
	if err != nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read project during import: %s", err),
		)
		return
	}

	if projectResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Project Not Found",
			fmt.Sprintf("Project with ref '%s' not found (status: %d)", projectRef, projectResp.StatusCode()),
		)
		return
	}

	// Set basic attributes
	state := MigrationsResourceModel{
		Id:            types.StringValue(projectRef),
		ProjectRef:    types.StringValue(projectRef),
		MigrationsDir: types.StringNull(),
	}

	// TODO: If API provides endpoint to list applied migrations, fetch them here
	// For now, import creates an empty migrations list with a warning
	state.Migrations = types.ListNull(types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"file_path": types.StringType,
			"name":      types.StringType,
			"content":   types.StringType,
			"digest":    types.StringType,
		},
	})

	resp.Diagnostics.AddWarning(
		"Import Limitation",
		"Imported migrations resource with empty migration list. "+
			"The Supabase API does not currently expose a list of applied migrations. "+
			"Set `migrations_dir` in configuration to a directory that matches already applied migrations, then apply.",
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// applyMigration applies a single migration with retry logic
func (r *MigrationsResource) applyMigration(ctx context.Context, projectRef string, migration MigrationModel) (MigrationModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	content := migration.Content.ValueString()
	name := migration.Name.ValueString()

	// Apply migration with retry logic
	var httpResp *api.V1ApplyAMigrationResponse
	var err error

	retryErr := retry.RetryContext(ctx, 2*time.Minute, func() *retry.RetryError {
		// Build migration request body
		body := api.V1ApplyAMigrationJSONRequestBody{
			Name:  Ptr(name),
			Query: content,
		}

		// Call the Supabase API to apply the migration
		httpResp, err = r.client.V1ApplyAMigrationWithResponse(ctx, projectRef, nil, body)
		if err != nil {
			return retry.RetryableError(err)
		}

		// Retry on server errors (5xx)
		if httpResp.StatusCode() >= 500 {
			return retry.RetryableError(fmt.Errorf("server error: %d - %s", httpResp.StatusCode(), string(httpResp.Body)))
		}

		// Don't retry on client errors (4xx)
		if httpResp.StatusCode() >= 400 {
			return retry.NonRetryableError(fmt.Errorf("client error: %d - %s", httpResp.StatusCode(), string(httpResp.Body)))
		}

		// Success (2xx)
		return nil
	})

	if retryErr != nil {
		diags.AddError(
			"Failed to Apply Migration",
			fmt.Sprintf("Could not apply migration '%s': %s", name, retryErr.Error()),
		)
		return migration, diags
	}

	// Update migration with response data
	result := migration

	tflog.Info(ctx, "Successfully applied migration", map[string]interface{}{
		"name":    name,
		"project": projectRef,
		"status":  httpResp.StatusCode(),
	})

	return result, diags
}

// buildMigrationsList constructs a types.List from a slice of MigrationModel
func (r *MigrationsResource) buildMigrationsList(migrations []MigrationModel, diags *diag.Diagnostics) types.List {
	migrationElemType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"file_path": types.StringType,
			"name":      types.StringType,
			"content":   types.StringType,
			"digest":    types.StringType,
		},
	}

	var values []attr.Value
	for _, mig := range migrations {
		obj, objDiags := types.ObjectValue(
			migrationElemType.AttrTypes,
			map[string]attr.Value{
				"file_path": mig.FilePath,
				"name":      mig.Name,
				"content":   mig.Content,
				"digest":    mig.Digest,
			},
		)
		diags.Append(objDiags...)
		if diags.HasError() {
			return types.ListNull(migrationElemType)
		}
		values = append(values, obj)
	}

	list, listDiags := types.ListValue(migrationElemType, values)
	diags.Append(listDiags...)
	return list
}
