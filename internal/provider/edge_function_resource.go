// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

var (
	_ resource.Resource                = &EdgeFunctionResource{}
	_ resource.ResourceWithImportState = &EdgeFunctionResource{}
)

// computes checksum from local files during plan phase
// to detect file content changes even when file paths haven't changed.
type localChecksumPlanModifier struct{}

func (m localChecksumPlanModifier) Description(_ context.Context) string {
	return "Computes SHA256 checksum from local source files to detect content changes."
}

func (m localChecksumPlanModifier) MarkdownDescription(_ context.Context) string {
	return "Computes SHA256 checksum from local source files to detect content changes."
}

func (m localChecksumPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var entrypoint types.String
	diags := req.Plan.GetAttribute(ctx, path.Root("entrypoint"), &entrypoint)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if entrypoint.IsUnknown() || entrypoint.IsNull() {
		return
	}

	var importMap types.String
	diags = req.Plan.GetAttribute(ctx, path.Root("import_map"), &importMap)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var staticFiles types.List
	diags = req.Plan.GetAttribute(ctx, path.Root("static_files"), &staticFiles)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data := &EdgeFunctionResourceModel{
		Entrypoint:  entrypoint,
		ImportMap:   importMap,
		StaticFiles: staticFiles,
	}

	checksum, checksumDiags := computeLocalChecksum(ctx, data)
	if checksumDiags.HasError() {
		resp.Diagnostics.Append(checksumDiags...)
		return
	}

	resp.PlanValue = types.StringValue(checksum)
}

func NewEdgeFunctionResource() resource.Resource {
	return &EdgeFunctionResource{}
}

type EdgeFunctionResource struct {
	client *api.ClientWithResponses
}

type EdgeFunctionResourceModel struct {
	ProjectRef  types.String `tfsdk:"project_ref"`
	Slug        types.String `tfsdk:"slug"`
	Name        types.String `tfsdk:"name"`
	Entrypoint  types.String `tfsdk:"entrypoint"`
	ImportMap   types.String `tfsdk:"import_map"`
	StaticFiles types.List   `tfsdk:"static_files"`

	Id            types.String `tfsdk:"id"`
	Version       types.Int64  `tfsdk:"version"`
	Status        types.String `tfsdk:"status"`
	Checksum      types.String `tfsdk:"checksum"`
	LocalChecksum types.String `tfsdk:"local_checksum"`
	CreatedAt     types.Int64  `tfsdk:"created_at"`
	UpdatedAt     types.Int64  `tfsdk:"updated_at"`
}

type functionMetadata struct {
	EntrypointPath string   `json:"entrypoint_path"`
	Name           string   `json:"name"`
	ImportMapPath  *string  `json:"import_map_path,omitempty"`
	StaticPatterns []string `json:"static_patterns,omitempty"`
}

func (r *EdgeFunctionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_edge_function"
}

func (r *EdgeFunctionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Edge Function resource",

		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project ref",
				Required:            true,
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: "URL-friendly identifier for the function",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the function (defaults to slug if not specified)",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"entrypoint": schema.StringAttribute{
				MarkdownDescription: "Path to the function entrypoint file",
				Required:            true,
			},
			"import_map": schema.StringAttribute{
				MarkdownDescription: "Path to the import map file",
				Optional:            true,
			},
			"static_files": schema.ListAttribute{
				MarkdownDescription: "List of glob patterns for static files to include",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Function identifier",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: "Currently deployed function version",
				Computed:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Function deployment status",
				Computed:            true,
			},
			"checksum": schema.StringAttribute{
				MarkdownDescription: "SHA256 checksum of the deployed function bundle (remote)",
				Computed:            true,
			},
			"local_checksum": schema.StringAttribute{
				MarkdownDescription: "SHA256 checksum of local source files before upload",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					localChecksumPlanModifier{},
				},
			},
			"created_at": schema.Int64Attribute{
				MarkdownDescription: "Timestamp when the function was created",
				Computed:            true,
			},
			"updated_at": schema.Int64Attribute{
				MarkdownDescription: "Timestamp when the function was last updated",
				Computed:            true,
			},
		},
	}
}

func (r *EdgeFunctionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *api.ClientWithResponses, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *EdgeFunctionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EdgeFunctionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deployEdgeFunction(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "create edge function")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EdgeFunctionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read edge function")

	found, diags := readEdgeFunction(ctx, &data, r.client)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If resource was deleted out-of-band, remove from state
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EdgeFunctionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deployEdgeFunction(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "update edge function")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeFunctionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EdgeFunctionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deleteEdgeFunction(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "delete edge function")
}

func (r *EdgeFunctionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import ID in format 'project_ref/slug', got: %s", req.ID),
		)
		return
	}

	projectRef := parts[0]
	slug := parts[1]

	data := EdgeFunctionResourceModel{
		ProjectRef: types.StringValue(projectRef),
		Slug:       types.StringValue(slug),
	}

	found, diags := readEdgeFunction(ctx, &data, r.client)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !found {
		resp.Diagnostics.AddError(
			"Resource Not Found",
			fmt.Sprintf("Edge function %s/%s does not exist", projectRef, slug),
		)
		return
	}

	data.Entrypoint = types.StringNull()
	data.ImportMap = types.StringNull()
	data.StaticFiles = types.ListNull(types.StringType)
	data.LocalChecksum = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func deployEdgeFunction(ctx context.Context, data *EdgeFunctionResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	projectRef := data.ProjectRef.ValueString()
	slug := data.Slug.ValueString()
	entrypoint := data.Entrypoint.ValueString()

	name := data.Name.ValueString()
	if name == "" {
		name = slug
	}

	// Compute local checksum before deployment
	localChecksum, diags := computeLocalChecksum(ctx, data)
	if diags.HasError() {
		return diags
	}
	data.LocalChecksum = types.StringValue(localChecksum)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	entrypointContent, err := os.ReadFile(entrypoint)
	if err != nil {
		msg := fmt.Sprintf("Unable to read entrypoint file %s, got error: %s", entrypoint, err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	entrypointPart, err := writer.CreateFormFile("file", filepath.Base(entrypoint))
	if err != nil {
		msg := fmt.Sprintf("Unable to create form file for entrypoint, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}
	if _, err := entrypointPart.Write(entrypointContent); err != nil {
		msg := fmt.Sprintf("Unable to write entrypoint content, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	var importMapPath *string
	if !data.ImportMap.IsNull() && !data.ImportMap.IsUnknown() {
		importMapFile := data.ImportMap.ValueString()
		importMapContent, err := os.ReadFile(importMapFile)
		if err != nil {
			msg := fmt.Sprintf("Unable to read import map file %s, got error: %s", importMapFile, err)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}

		importMapPart, err := writer.CreateFormFile("file", filepath.Base(importMapFile))
		if err != nil {
			msg := fmt.Sprintf("Unable to create form file for import map, got error: %s", err)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}
		if _, err := importMapPart.Write(importMapContent); err != nil {
			msg := fmt.Sprintf("Unable to write import map content, got error: %s", err)
			return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}
		importMapPath = &importMapFile
	}

	var staticPatterns []string
	if !data.StaticFiles.IsNull() && !data.StaticFiles.IsUnknown() {
		var patterns []string
		diags := data.StaticFiles.ElementsAs(ctx, &patterns, false)
		if diags.HasError() {
			return diags
		}

		for _, pattern := range patterns {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				msg := fmt.Sprintf("Invalid glob pattern %s, got error: %s", pattern, err)
				return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
			}

			for _, match := range matches {
				info, err := os.Stat(match)
				if err != nil || info.IsDir() {
					continue
				}

				content, err := os.ReadFile(match)
				if err != nil {
					msg := fmt.Sprintf("Unable to read static file %s, got error: %s", match, err)
					return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
				}

				part, err := writer.CreateFormFile("file", match)
				if err != nil {
					msg := fmt.Sprintf("Unable to create form file for static file %s, got error: %s", match, err)
					return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
				}
				if _, err := part.Write(content); err != nil {
					msg := fmt.Sprintf("Unable to write static file content %s, got error: %s", match, err)
					return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
				}
			}
		}
		staticPatterns = patterns
	}

	meta := functionMetadata{
		EntrypointPath: filepath.Base(entrypoint),
		Name:           name,
	}
	if importMapPath != nil {
		basePath := filepath.Base(*importMapPath)
		meta.ImportMapPath = &basePath
	}
	if len(staticPatterns) > 0 {
		meta.StaticPatterns = staticPatterns
	}

	metadataBytes, err := json.Marshal(meta)
	if err != nil {
		msg := fmt.Sprintf("Unable to marshal metadata, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if err := writer.WriteField("metadata", string(metadataBytes)); err != nil {
		msg := fmt.Sprintf("Unable to write metadata, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if err := writer.Close(); err != nil {
		msg := fmt.Sprintf("Unable to close multipart writer, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	params := &api.V1DeployAFunctionParams{
		Slug: &slug,
	}

	httpResp, err := client.V1DeployAFunctionWithBodyWithResponse(
		ctx,
		projectRef,
		params,
		writer.FormDataContentType(),
		body,
	)
	if err != nil {
		msg := fmt.Sprintf("Unable to deploy edge function, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.JSON201 == nil {
		msg := fmt.Sprintf("Unable to deploy edge function, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	result := httpResp.JSON201
	data.Id = types.StringValue(result.Id)
	data.Name = types.StringValue(result.Name)
	data.Version = types.Int64Value(int64(result.Version))
	data.Status = types.StringValue(string(result.Status))
	if result.EzbrSha256 != nil {
		data.Checksum = types.StringValue(*result.EzbrSha256)
	} else {
		data.Checksum = types.StringNull()
	}
	if result.CreatedAt != nil {
		data.CreatedAt = types.Int64Value(*result.CreatedAt)
	}
	if result.UpdatedAt != nil {
		data.UpdatedAt = types.Int64Value(*result.UpdatedAt)
	}

	return nil
}

// Returns (true, nil) if found, (false, nil) if not found (404), or (false, diags) on error.
func readEdgeFunction(ctx context.Context, data *EdgeFunctionResourceModel, client *api.ClientWithResponses) (bool, diag.Diagnostics) {
	projectRef := data.ProjectRef.ValueString()
	slug := data.Slug.ValueString()

	httpResp, err := client.V1GetAFunctionWithResponse(ctx, projectRef, slug)
	if err != nil {
		msg := fmt.Sprintf("Unable to read edge function, got error: %s", err)
		return false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() == http.StatusNotFound {
		tflog.Trace(ctx, fmt.Sprintf("edge function not found: %s/%s", projectRef, slug))
		return false, nil
	}

	if httpResp.JSON200 == nil {
		msg := fmt.Sprintf("Unable to read edge function, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	result := httpResp.JSON200
	data.Id = types.StringValue(result.Id)
	data.Name = types.StringValue(result.Name)
	data.Version = types.Int64Value(int64(result.Version))
	data.Status = types.StringValue(string(result.Status))
	if result.EzbrSha256 != nil {
		data.Checksum = types.StringValue(*result.EzbrSha256)
	} else {
		data.Checksum = types.StringNull()
	}
	data.CreatedAt = types.Int64Value(result.CreatedAt)
	data.UpdatedAt = types.Int64Value(result.UpdatedAt)

	return true, nil
}

func deleteEdgeFunction(ctx context.Context, data *EdgeFunctionResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	projectRef := data.ProjectRef.ValueString()
	slug := data.Slug.ValueString()

	httpResp, err := client.V1DeleteAFunctionWithResponse(ctx, projectRef, slug)
	if err != nil {
		msg := fmt.Sprintf("Unable to delete edge function, got error: %s", err)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if httpResp.StatusCode() == http.StatusNotFound {
		tflog.Trace(ctx, fmt.Sprintf("edge function already deleted: %s/%s", projectRef, slug))
		return nil
	}

	if httpResp.StatusCode() != http.StatusOK {
		msg := fmt.Sprintf("Unable to delete edge function, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
		return diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	return nil
}

// Computes a SHA256 checksum of all local source files.
func computeLocalChecksum(ctx context.Context, data *EdgeFunctionResourceModel) (string, diag.Diagnostics) {
	hasher := sha256.New()

	addFile := func(path string) error {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hasher.Write([]byte(path))
		hasher.Write([]byte{0})
		hasher.Write(content)
		hasher.Write([]byte{0})
		return nil
	}

	entrypoint := data.Entrypoint.ValueString()
	if err := addFile(entrypoint); err != nil {
		msg := fmt.Sprintf("Unable to read entrypoint file for checksum, got error: %s", err)
		return "", diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
	}

	if !data.ImportMap.IsNull() && !data.ImportMap.IsUnknown() {
		importMapFile := data.ImportMap.ValueString()
		if err := addFile(importMapFile); err != nil {
			msg := fmt.Sprintf("Unable to read import map file for checksum, got error: %s", err)
			return "", diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
		}
	}

	if !data.StaticFiles.IsNull() && !data.StaticFiles.IsUnknown() {
		var patterns []string
		diags := data.StaticFiles.ElementsAs(ctx, &patterns, false)
		if diags.HasError() {
			return "", diags
		}

		var allFiles []string
		for _, pattern := range patterns {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				msg := fmt.Sprintf("Invalid glob pattern %s, got error: %s", pattern, err)
				return "", diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
			}
			for _, match := range matches {
				info, err := os.Stat(match)
				if err != nil || info.IsDir() {
					continue
				}
				allFiles = append(allFiles, match)
			}
		}

		sort.Strings(allFiles)

		for _, file := range allFiles {
			if err := addFile(file); err != nil {
				msg := fmt.Sprintf("Unable to read static file for checksum, got error: %s", err)
				return "", diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
			}
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
