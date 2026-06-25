package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	openapiTypes "github.com/oapi-codegen/runtime/types"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

func smtpToModel(ctx context.Context, resp *api.AuthConfigResponse, prior *AuthSettingsResourceModel, resolve secretResolver) (types.Object, diag.Diagnostics) {
	priorPass := types.StringNull()
	if prior != nil && !prior.SMTP.IsNull() && !prior.SMTP.IsUnknown() {
		s, d := readObj[SMTPModel](ctx, prior.SMTP)
		if !d.HasError() {
			priorPass = s.Pass
		}
	}
	return buildObj(ctx, SMTPModel{
		AdminEmail:   NullableToString(resp.SmtpAdminEmail),
		Host:         NullableToString(resp.SmtpHost),
		Port:         NullableToString(resp.SmtpPort),
		User:         NullableToString(resp.SmtpUser),
		Pass:         resolve("smtp_pass", priorPass),
		SenderName:   NullableToString(resp.SmtpSenderName),
		MaxFrequency: NullableToInt64(resp.SmtpMaxFrequency),
	})
}

func smtpToBody(ctx context.Context, smtpObj types.Object, body *api.UpdateAuthConfigBody) diag.Diagnostics {
	if smtpObj.IsNull() || smtpObj.IsUnknown() {
		return nil
	}
	smtp, diags := readObj[SMTPModel](ctx, smtpObj)
	if !smtp.AdminEmail.IsNull() && !smtp.AdminEmail.IsUnknown() {
		body.SmtpAdminEmail = nullable.NewNullableWithValue(openapiTypes.Email(smtp.AdminEmail.ValueString()))
	}
	body.SmtpHost = setStr(smtp.Host)
	body.SmtpPort = setStr(smtp.Port)
	body.SmtpUser = setStr(smtp.User)
	if !smtp.Pass.IsNull() && !smtp.Pass.IsUnknown() {
		body.SmtpPass = nullable.NewNullableWithValue(smtp.Pass.ValueString())
	}
	body.SmtpSenderName = setStr(smtp.SenderName)
	body.SmtpMaxFrequency = setInt64(smtp.MaxFrequency)
	return diags
}
