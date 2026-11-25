// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/supabase/cli/pkg/api"
)

var (
	_ resource.Resource                = &CustomHostnameResource{}
	_ resource.ResourceWithImportState = &CustomHostnameResource{}
	_ resource.ResourceWithModifyPlan  = &CustomHostnameResource{}
)

func NewCustomHostnameResource() resource.Resource {
	return &CustomHostnameResource{}
}

type CustomHostnameResource struct {
	client *api.ClientWithResponses
}

type CustomHostnameResourceModel struct {
	Id                    types.String `tfsdk:"id"`
	ProjectRef            types.String `tfsdk:"project_ref"`
	CustomHostname        types.String `tfsdk:"custom_hostname"`
	Activate              types.Bool   `tfsdk:"activate"`
	Status                types.String `tfsdk:"status"`
	HostnameID            types.String `tfsdk:"hostname_id"`
	HostnameStatus        types.String `tfsdk:"hostname_status"`
	CustomOriginServer    types.String `tfsdk:"custom_origin_server"`
	OwnershipVerification types.Object `tfsdk:"ownership_verification"`
	SSL                   types.Object `tfsdk:"ssl"`
	VerificationErrors    types.List   `tfsdk:"verification_errors"`
}

const customHostnameActiveStatus = string(api.N5ServicesReconfigured)

const customHostnameRateLimitDetail = "initialize, reverify, and delete allow 10 requests per minute. " +
	"Each apply sends at most one request to each of those endpoints. " +
	"See https://supabase.com/docs/reference/api/introduction#endpoint-exceptions."

var customHostnamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`)

var ownershipVerificationAttrTypes = map[string]attr.Type{
	"type":  types.StringType,
	"name":  types.StringType,
	"value": types.StringType,
}

var sslValidationRecordAttrTypes = map[string]attr.Type{
	"txt_name":  types.StringType,
	"txt_value": types.StringType,
}

var sslAttrTypes = map[string]attr.Type{
	"status": types.StringType,
	"validation_records": types.ListType{ElemType: types.ObjectType{
		AttrTypes: sslValidationRecordAttrTypes,
	}},
	"validation_errors": types.ListType{ElemType: types.StringType},
}

func (r *CustomHostnameResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_hostname"
}

func (r *CustomHostnameResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Custom hostname for a Supabase project. The Management API marks this feature as beta.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Project reference ID. A project has one custom hostname, so this matches `project_ref`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"custom_hostname": schema.StringAttribute{
				MarkdownDescription: "Hostname to serve the project from, such as `api.example.com`. " +
					"Supabase custom domains support subdomains. Changing it re-initializes the hostname.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 253),
					stringvalidator.RegexMatches(
						customHostnamePattern,
						"custom_hostname must be a DNS hostname such as api.example.com",
					),
				},
			},
			"activate": schema.BoolAttribute{
				MarkdownDescription: "When true, the provider reverifies DNS and activates the hostname. " +
					"Apply again while DNS propagates; the provider keeps planning an update until `status` is `5_services_reconfigured`. " +
					"Setting this to false leaves an active hostname in place. Delete the resource to remove it. " +
					customHostnameRateLimitDetail,
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Supabase custom hostname setup status: `1_not_started`, `2_initiated`, `3_challenge_verified`, `4_origin_setup_completed`, or `5_services_reconfigured`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the custom hostname.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname_status": schema.StringAttribute{
				MarkdownDescription: "Status reported for the hostname record.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"custom_origin_server": schema.StringAttribute{
				MarkdownDescription: "CNAME target for the custom hostname. Point `custom_hostname` at this value.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ownership_verification": schema.SingleNestedAttribute{
				MarkdownDescription: "DNS record that proves ownership of the hostname.",
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						MarkdownDescription: "DNS record type",
						Computed:            true,
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "DNS record name",
						Computed:            true,
					},
					"value": schema.StringAttribute{
						MarkdownDescription: "DNS record value",
						Computed:            true,
					},
				},
			},
			"ssl": schema.SingleNestedAttribute{
				MarkdownDescription: "Certificate validation details for the custom hostname.",
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"status": schema.StringAttribute{
						MarkdownDescription: "Certificate status",
						Computed:            true,
					},
					"validation_records": schema.ListNestedAttribute{
						MarkdownDescription: "DNS TXT records required to issue the certificate.",
						Computed:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"txt_name": schema.StringAttribute{
									MarkdownDescription: "TXT record name",
									Computed:            true,
								},
								"txt_value": schema.StringAttribute{
									MarkdownDescription: "TXT record value",
									Computed:            true,
								},
							},
						},
					},
					"validation_errors": schema.ListAttribute{
						MarkdownDescription: "Certificate validation error messages",
						Computed:            true,
						ElementType:         types.StringType,
					},
				},
			},
			"verification_errors": schema.ListAttribute{
				MarkdownDescription: "Errors from the latest hostname verification.",
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *CustomHostnameResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client, ok := extractClient(req.ProviderData, &resp.Diagnostics); ok {
		r.client = client
	}
}

func (r *CustomHostnameResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CustomHostnameResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(syncCustomHostname(ctx, &data, true, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created custom hostname")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomHostnameResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CustomHostnameResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, diags := readCustomHostname(ctx, &data, r.client)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	tflog.Trace(ctx, "read custom hostname")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomHostnameResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state CustomHostnameResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hostnameChanged := hostnamesDiffer(plan.CustomHostname, state.CustomHostname)
	if !hostnameChanged && (plan.Status.IsNull() || plan.Status.IsUnknown()) {
		plan.Status = state.Status
	}

	resp.Diagnostics.Append(syncCustomHostname(ctx, &plan, hostnameChanged, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated custom hostname")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CustomHostnameResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CustomHostnameResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deleteCustomHostname(ctx, &data, r.client)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "deleted custom hostname")
}

func (r *CustomHostnameResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectRef := strings.TrimSpace(req.ID)
	if projectRef == "" || strings.Contains(projectRef, "/") {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			"Import a custom hostname with the project reference.\nExample: mayuaycdtijbctgqbycg",
		)
		return
	}

	data := CustomHostnameResourceModel{
		ProjectRef: types.StringValue(projectRef),
		Activate:   types.BoolValue(false),
		Id:         types.StringValue(projectRef),
	}

	found, diags := readCustomHostname(ctx, &data, r.client)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found || data.CustomHostname.IsNull() || data.CustomHostname.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Resource Not Found",
			fmt.Sprintf("Project %s does not have a custom hostname", projectRef),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomHostnameResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	var plan, state CustomHostnameResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hostnameChanged := hostnamesDiffer(plan.CustomHostname, state.CustomHostname)
	activating := !plan.Activate.IsNull() && !plan.Activate.IsUnknown() && plan.Activate.ValueBool() && !customHostnameIsActive(state.Status)
	if !hostnameChanged && !activating {
		return
	}

	// These values come from the next initialize, reverify, or activate response.
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("status"), types.StringUnknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("hostname_id"), types.StringUnknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("hostname_status"), types.StringUnknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("custom_origin_server"), types.StringUnknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("ownership_verification"), types.ObjectUnknown(ownershipVerificationAttrTypes))...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("ssl"), types.ObjectUnknown(sslAttrTypes))...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("verification_errors"), types.ListUnknown(types.StringType))...)
}

func syncCustomHostname(ctx context.Context, data *CustomHostnameResourceModel, hostnameChanged bool, client *api.ClientWithResponses) diag.Diagnostics {
	data.Id = types.StringValue(data.ProjectRef.ValueString())

	if hostnameChanged {
		if diags := initializeCustomHostname(ctx, data, client); diags.HasError() {
			return diags
		}
	}

	if data.Activate.IsNull() || data.Activate.IsUnknown() || !data.Activate.ValueBool() {
		return nil
	}
	if customHostnameIsActive(data.Status) {
		return nil
	}

	if diags := reverifyCustomHostname(ctx, data, client); diags.HasError() {
		return diags
	}
	if customHostnameIsActive(data.Status) {
		return nil
	}

	return activateCustomHostname(ctx, data, client)
}

func initializeCustomHostname(ctx context.Context, data *CustomHostnameResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1UpdateHostnameConfigWithResponse(ctx, data.ProjectRef.ValueString(), api.V1UpdateHostnameConfigJSONRequestBody{
		CustomHostname: data.CustomHostname.ValueString(),
	})
	if err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"Client Error",
			fmt.Sprintf("Unable to initialize custom hostname, got error: %s", err),
		)}
	}
	if httpResp.JSON201 == nil {
		return diag.Diagnostics{customHostnameError("initialize", httpResp.StatusCode(), httpResp.Body, true)}
	}

	return setCustomHostnameState(data, *httpResp.JSON201)
}

func reverifyCustomHostname(ctx context.Context, data *CustomHostnameResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1VerifyDnsConfigWithResponse(ctx, data.ProjectRef.ValueString())
	if err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"Client Error",
			fmt.Sprintf("Unable to reverify custom hostname, got error: %s", err),
		)}
	}
	if httpResp.JSON201 == nil {
		return diag.Diagnostics{customHostnameError("reverify", httpResp.StatusCode(), httpResp.Body, true)}
	}

	return setCustomHostnameState(data, *httpResp.JSON201)
}

func activateCustomHostname(ctx context.Context, data *CustomHostnameResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1ActivateCustomHostnameWithResponse(ctx, data.ProjectRef.ValueString())
	if err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"Client Error",
			fmt.Sprintf("Unable to activate custom hostname, got error: %s", err),
		)}
	}
	if httpResp.JSON201 == nil {
		return diag.Diagnostics{customHostnameError("activate", httpResp.StatusCode(), httpResp.Body, false)}
	}

	return setCustomHostnameState(data, *httpResp.JSON201)
}

func readCustomHostname(ctx context.Context, data *CustomHostnameResourceModel, client *api.ClientWithResponses) (bool, diag.Diagnostics) {
	httpResp, err := client.V1GetHostnameConfigWithResponse(ctx, data.ProjectRef.ValueString())
	if err != nil {
		return false, diag.Diagnostics{diag.NewErrorDiagnostic(
			"Client Error",
			fmt.Sprintf("Unable to read custom hostname, got error: %s", err),
		)}
	}
	if httpResp.StatusCode() == http.StatusNotFound {
		return false, nil
	}
	if httpResp.JSON200 == nil {
		return false, diag.Diagnostics{customHostnameError("read", httpResp.StatusCode(), httpResp.Body, false)}
	}

	if diags := setCustomHostnameState(data, *httpResp.JSON200); diags.HasError() {
		return false, diags
	}
	return true, nil
}

func deleteCustomHostname(ctx context.Context, data *CustomHostnameResourceModel, client *api.ClientWithResponses) diag.Diagnostics {
	httpResp, err := client.V1DeleteHostnameConfigWithResponse(ctx, data.ProjectRef.ValueString())
	if err != nil {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"Client Error",
			fmt.Sprintf("Unable to delete custom hostname, got error: %s", err),
		)}
	}
	if httpResp.StatusCode() == http.StatusNotFound || (httpResp.StatusCode() >= 200 && httpResp.StatusCode() < 300) {
		return nil
	}

	return diag.Diagnostics{customHostnameError("delete", httpResp.StatusCode(), httpResp.Body, true)}
}

func hostnamesDiffer(planned, prior types.String) bool {
	if planned.IsNull() || prior.IsNull() || planned.IsUnknown() || prior.IsUnknown() {
		return !planned.Equal(prior)
	}
	return !strings.EqualFold(planned.ValueString(), prior.ValueString())
}

func customHostnameIsActive(status types.String) bool {
	return !status.IsNull() && !status.IsUnknown() && status.ValueString() == customHostnameActiveStatus
}

func customHostnameError(action string, statusCode int, body []byte, strictRateLimit bool) diag.Diagnostic {
	msg := fmt.Sprintf("Unable to %s custom hostname, got status %d: %s", action, statusCode, body)
	if strictRateLimit && statusCode == http.StatusTooManyRequests {
		msg += "\n\n" + customHostnameRateLimitDetail
	}
	return diag.NewErrorDiagnostic("Client Error", msg)
}

func setCustomHostnameState(data *CustomHostnameResourceModel, resp api.UpdateCustomHostnameResponse) diag.Diagnostics {
	var diags diag.Diagnostics

	data.Id = types.StringValue(data.ProjectRef.ValueString())
	data.Status = types.StringValue(string(resp.Status))

	hostname := resp.CustomHostname
	if hostname == "" {
		hostname = resp.Data.Result.Hostname
	}
	if hostname != "" && (data.CustomHostname.IsNull() || data.CustomHostname.IsUnknown() || !strings.EqualFold(data.CustomHostname.ValueString(), hostname)) {
		data.CustomHostname = types.StringValue(hostname)
	}

	data.HostnameID = nullableString(resp.Data.Result.Id)
	data.HostnameStatus = nullableString(resp.Data.Result.Status)
	data.CustomOriginServer = nullableString(resp.Data.Result.CustomOriginServer)

	ownership, ownershipDiags := ownershipVerificationObject(resp.Data.Result.OwnershipVerification)
	diags.Append(ownershipDiags...)
	if diags.HasError() {
		return diags
	}
	data.OwnershipVerification = ownership

	ssl, sslDiags := sslObject(resp.Data.Result.Ssl)
	diags.Append(sslDiags...)
	if diags.HasError() {
		return diags
	}
	data.SSL = ssl

	verificationErrors, listDiags := stringListValue(stringSlice(resp.Data.Result.VerificationErrors))
	diags.Append(listDiags...)
	if diags.HasError() {
		return diags
	}
	data.VerificationErrors = verificationErrors

	return diags
}

func ownershipVerificationObject(verification struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}) (types.Object, diag.Diagnostics) {
	if verification.Name == "" && verification.Type == "" && verification.Value == "" {
		return types.ObjectNull(ownershipVerificationAttrTypes), nil
	}

	return types.ObjectValue(ownershipVerificationAttrTypes, map[string]attr.Value{
		"type":  nullableString(verification.Type),
		"name":  nullableString(verification.Name),
		"value": nullableString(verification.Value),
	})
}

func sslObject(ssl struct {
	Status           string `json:"status"`
	ValidationErrors *[]struct {
		Message string `json:"message"`
	} `json:"validation_errors,omitempty"`
	ValidationRecords []struct {
		TxtName  string `json:"txt_name"`
		TxtValue string `json:"txt_value"`
	} `json:"validation_records"`
}) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	records, recordsDiags := validationRecordsList(ssl.ValidationRecords)
	diags.Append(recordsDiags...)
	errors, errorsDiags := validationErrorsList(ssl.ValidationErrors)
	diags.Append(errorsDiags...)
	if diags.HasError() {
		return types.ObjectNull(sslAttrTypes), diags
	}

	if ssl.Status == "" && len(ssl.ValidationRecords) == 0 && (ssl.ValidationErrors == nil || len(*ssl.ValidationErrors) == 0) {
		return types.ObjectNull(sslAttrTypes), nil
	}

	obj, objDiags := types.ObjectValue(sslAttrTypes, map[string]attr.Value{
		"status":             nullableString(ssl.Status),
		"validation_records": records,
		"validation_errors":  errors,
	})
	diags.Append(objDiags...)
	return obj, diags
}

func validationRecordsList(records []struct {
	TxtName  string `json:"txt_name"`
	TxtValue string `json:"txt_value"`
}) (types.List, diag.Diagnostics) {
	elems := make([]attr.Value, 0, len(records))
	for _, record := range records {
		obj, diags := types.ObjectValue(sslValidationRecordAttrTypes, map[string]attr.Value{
			"txt_name":  types.StringValue(record.TxtName),
			"txt_value": types.StringValue(record.TxtValue),
		})
		if diags.HasError() {
			return types.ListNull(types.ObjectType{AttrTypes: sslValidationRecordAttrTypes}), diags
		}
		elems = append(elems, obj)
	}

	return types.ListValue(types.ObjectType{AttrTypes: sslValidationRecordAttrTypes}, elems)
}

func validationErrorsList(errors *[]struct {
	Message string `json:"message"`
}) (types.List, diag.Diagnostics) {
	var messages []string
	if errors != nil {
		messages = make([]string, 0, len(*errors))
		for _, validationError := range *errors {
			messages = append(messages, validationError.Message)
		}
	}
	return stringListValue(messages)
}

func stringSlice(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}

func stringListValue(values []string) (types.List, diag.Diagnostics) {
	elems := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elems = append(elems, types.StringValue(value))
	}
	return types.ListValue(types.StringType, elems)
}

func nullableString(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}
