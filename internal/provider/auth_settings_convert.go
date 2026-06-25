package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

// authConfigResponseToModel converts an API response into a TF model.
//
// When driftCheck is false (Create/Update) secret fields are carried forward from
// plan so the user's plaintext is preserved in state.
//
// When driftCheck is true (Read) each hash returned by the API is compared against
// the stored hash; a mismatch signals an out-of-band change and the plaintext is
// set to null so the next plan shows a diff against the HCL config.
func authConfigResponseToModel(ctx context.Context, resp *api.AuthConfigResponse, prior *AuthSettingsResourceModel, driftCheck bool) (*AuthSettingsResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	newHashes := extractAuthSecretHashes(resp)
	oldHashes := priorSecretHashes(ctx, prior)

	resolve := func(apiKey string, priorVal types.String) types.String {
		if !driftCheck {
			return priorVal
		}
		if priorVal.IsNull() || priorVal.IsUnknown() {
			return priorVal
		}
		oldHash := oldHashes[apiKey]
		if oldHash != "" && oldHash != newHashes[apiKey] {
			return types.StringNull()
		}
		return priorVal
	}

	m := &AuthSettingsResourceModel{
		ProjectRef: prior.ProjectRef,
		Id:         prior.Id,
		Timeouts:   prior.Timeouts,
	}
	m.SiteUrl = NullableToString(resp.SiteUrl)
	m.JwtExp = NullableToInt64(resp.JwtExp)
	m.DisableSignup = NullableToBool(resp.DisableSignup)
	m.ExternalEmailEnabled = NullableToBool(resp.ExternalEmailEnabled)
	m.ExternalPhoneEnabled = NullableToBool(resp.ExternalPhoneEnabled)
	m.PasswordMinLength = NullableToInt64(resp.PasswordMinLength)

	var d diag.Diagnostics

	m.SMTP, d = smtpToModel(ctx, resp, prior, resolve)
	diags.Append(d...)

	m.External, d = externalToModel(ctx, resp, prior, resolve)
	diags.Append(d...)

	m.Hooks, d = hooksToModel(ctx, resp, prior, resolve)
	diags.Append(d...)

	m.MFA, d = mfaToModel(ctx, resp)
	diags.Append(d...)

	m.Security, d = securityToModel(ctx, resp, prior, resolve)
	diags.Append(d...)

	m.Sessions, d = sessionsToModel(ctx, resp)
	diags.Append(d...)

	m.SecretHashes, d = hashMapValue(newHashes)
	diags.Append(d...)

	return m, diags
}

func modelToAuthConfigBody(ctx context.Context, m *AuthSettingsResourceModel) (api.UpdateAuthConfigBody, diag.Diagnostics) {
	var diags diag.Diagnostics
	body := api.UpdateAuthConfigBody{}

	if !m.SiteUrl.IsNull() && !m.SiteUrl.IsUnknown() {
		body.SiteUrl = nullable.NewNullableWithValue(m.SiteUrl.ValueString())
	}
	if !m.JwtExp.IsNull() && !m.JwtExp.IsUnknown() {
		body.JwtExp = nullable.NewNullableWithValue(int(m.JwtExp.ValueInt64()))
	}
	if !m.DisableSignup.IsNull() && !m.DisableSignup.IsUnknown() {
		body.DisableSignup = nullable.NewNullableWithValue(m.DisableSignup.ValueBool())
	}
	if !m.ExternalEmailEnabled.IsNull() && !m.ExternalEmailEnabled.IsUnknown() {
		body.ExternalEmailEnabled = nullable.NewNullableWithValue(m.ExternalEmailEnabled.ValueBool())
	}
	if !m.ExternalPhoneEnabled.IsNull() && !m.ExternalPhoneEnabled.IsUnknown() {
		body.ExternalPhoneEnabled = nullable.NewNullableWithValue(m.ExternalPhoneEnabled.ValueBool())
	}
	if !m.PasswordMinLength.IsNull() && !m.PasswordMinLength.IsUnknown() {
		body.PasswordMinLength = nullable.NewNullableWithValue(int(m.PasswordMinLength.ValueInt64()))
	}

	d := smtpToBody(ctx, m.SMTP, &body)
	diags.Append(d...)

	d = externalToBody(ctx, m.External, &body)
	diags.Append(d...)

	d = hooksToBody(ctx, m.Hooks, &body)
	diags.Append(d...)

	d = mfaToBody(ctx, m.MFA, &body)
	diags.Append(d...)

	d = securityToBody(ctx, m.Security, &body)
	diags.Append(d...)

	d = sessionsToBody(ctx, m.Sessions, &body)
	diags.Append(d...)

	return body, diags
}
