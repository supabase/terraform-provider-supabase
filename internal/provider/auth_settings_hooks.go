package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

// hookProvider describes a single auth hook (enabled + uri + secrets).
type hookProvider struct {
	secretKey string
	// API response readers
	enabled func(*api.AuthConfigResponse) nullable.Nullable[bool]
	uri     func(*api.AuthConfigResponse) nullable.Nullable[string]
	secret  func(*api.AuthConfigResponse) nullable.Nullable[string]
	// TF model field accessors
	getField func(*HooksModel) types.Object
	setField func(*HooksModel, types.Object)
	// API body field pointers
	bEnabled func(*api.UpdateAuthConfigBody) *nullable.Nullable[bool]
	bUri     func(*api.UpdateAuthConfigBody) *nullable.Nullable[string]
	bSecret  func(*api.UpdateAuthConfigBody) *nullable.Nullable[string]
}

var hookProviders = []hookProvider{
	{
		secretKey: "hook_send_email_secrets",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.HookSendEmailEnabled },
		uri:       func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookSendEmailUri },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookSendEmailSecrets },
		getField:  func(h *HooksModel) types.Object { return h.SendEmail },
		setField:  func(h *HooksModel, o types.Object) { h.SendEmail = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.HookSendEmailEnabled },
		bUri:      func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookSendEmailUri },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookSendEmailSecrets },
	},
	{
		secretKey: "hook_send_sms_secrets",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.HookSendSmsEnabled },
		uri:       func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookSendSmsUri },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookSendSmsSecrets },
		getField:  func(h *HooksModel) types.Object { return h.SendSMS },
		setField:  func(h *HooksModel, o types.Object) { h.SendSMS = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.HookSendSmsEnabled },
		bUri:      func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookSendSmsUri },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookSendSmsSecrets },
	},
	{
		secretKey: "hook_custom_access_token_secrets",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.HookCustomAccessTokenEnabled },
		uri:       func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookCustomAccessTokenUri },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookCustomAccessTokenSecrets },
		getField:  func(h *HooksModel) types.Object { return h.CustomAccessToken },
		setField:  func(h *HooksModel, o types.Object) { h.CustomAccessToken = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.HookCustomAccessTokenEnabled },
		bUri:      func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookCustomAccessTokenUri },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookCustomAccessTokenSecrets },
	},
	{
		secretKey: "hook_mfa_verification_attempt_secrets",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.HookMfaVerificationAttemptEnabled },
		uri:       func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookMfaVerificationAttemptUri },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookMfaVerificationAttemptSecrets },
		getField:  func(h *HooksModel) types.Object { return h.MFAVerificationAttempt },
		setField:  func(h *HooksModel, o types.Object) { h.MFAVerificationAttempt = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.HookMfaVerificationAttemptEnabled },
		bUri:      func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookMfaVerificationAttemptUri },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookMfaVerificationAttemptSecrets },
	},
	{
		secretKey: "hook_password_verification_attempt_secrets",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.HookPasswordVerificationAttemptEnabled },
		uri:       func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookPasswordVerificationAttemptUri },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.HookPasswordVerificationAttemptSecrets },
		getField:  func(h *HooksModel) types.Object { return h.PasswordVerificationAttempt },
		setField:  func(h *HooksModel, o types.Object) { h.PasswordVerificationAttempt = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.HookPasswordVerificationAttemptEnabled },
		bUri:      func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookPasswordVerificationAttemptUri },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.HookPasswordVerificationAttemptSecrets },
	},
}

func hooksToModel(ctx context.Context, resp *api.AuthConfigResponse, prior *AuthSettingsResourceModel, resolve secretResolver) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	priorHooks, d := readObj[HooksModel](ctx, prior.Hooks)
	diags.Append(d...)

	var hooks HooksModel
	for _, p := range hookProviders {
		priorObj := p.getField(&priorHooks)
		priorHook, d := readObj[HookModel](ctx, priorObj)
		diags.Append(d...)

		obj, d := buildObj(ctx, HookModel{
			Enabled: NullableToBool(p.enabled(resp)),
			URI:     NullableToString(p.uri(resp)),
			Secrets: resolve(p.secretKey, priorHook.Secrets),
		})
		diags.Append(d...)
		p.setField(&hooks, obj)
	}

	result, d := buildObj(ctx, hooks)
	diags.Append(d...)
	return result, diags
}

func hooksToBody(ctx context.Context, hooksObj types.Object, body *api.UpdateAuthConfigBody) diag.Diagnostics {
	if hooksObj.IsNull() || hooksObj.IsUnknown() {
		return nil
	}
	hooks, diags := readObj[HooksModel](ctx, hooksObj)

	for _, p := range hookProviders {
		obj := p.getField(&hooks)
		if obj.IsNull() || obj.IsUnknown() {
			continue
		}
		h, d := readObj[HookModel](ctx, obj)
		diags.Append(d...)
		*p.bEnabled(body) = setBool(h.Enabled)
		*p.bUri(body) = setStr(h.URI)
		if !h.Secrets.IsNull() && !h.Secrets.IsUnknown() {
			*p.bSecret(body) = nullable.NewNullableWithValue(h.Secrets.ValueString())
		}
	}

	return diags
}
