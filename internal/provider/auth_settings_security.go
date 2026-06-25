package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

func securityToModel(ctx context.Context, resp *api.AuthConfigResponse, prior *AuthSettingsResourceModel, resolve secretResolver) (types.Object, diag.Diagnostics) {
	priorSecret := types.StringNull()
	if prior != nil && !prior.Security.IsNull() && !prior.Security.IsUnknown() {
		s, d := readObj[SecurityModel](ctx, prior.Security)
		if !d.HasError() {
			priorSecret = s.CaptchaSecret
		}
	}
	return buildObj(ctx, SecurityModel{
		CaptchaEnabled:       NullableToBool(resp.SecurityCaptchaEnabled),
		CaptchaProvider:      NullableToString(resp.SecurityCaptchaProvider),
		CaptchaSecret:        resolve("security_captcha_secret", priorSecret),
		ManualLinkingEnabled: NullableToBool(resp.SecurityManualLinkingEnabled),
	})
}

func securityToBody(ctx context.Context, secObj types.Object, body *api.UpdateAuthConfigBody) diag.Diagnostics {
	if secObj.IsNull() || secObj.IsUnknown() {
		return nil
	}
	sec, diags := readObj[SecurityModel](ctx, secObj)
	body.SecurityCaptchaEnabled = setBool(sec.CaptchaEnabled)
	if !sec.CaptchaProvider.IsNull() && !sec.CaptchaProvider.IsUnknown() {
		body.SecurityCaptchaProvider = nullable.NewNullableWithValue(api.UpdateAuthConfigBodySecurityCaptchaProvider(sec.CaptchaProvider.ValueString()))
	}
	if !sec.CaptchaSecret.IsNull() && !sec.CaptchaSecret.IsUnknown() {
		body.SecurityCaptchaSecret = nullable.NewNullableWithValue(sec.CaptchaSecret.ValueString())
	}
	body.SecurityManualLinkingEnabled = setBool(sec.ManualLinkingEnabled)
	return diags
}
