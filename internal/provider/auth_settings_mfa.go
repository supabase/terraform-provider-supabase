package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

func mfaToModel(ctx context.Context, resp *api.AuthConfigResponse) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	totp, d := buildObj(ctx, MFATotpModel{
		EnrollEnabled: NullableToBool(resp.MfaTotpEnrollEnabled),
		VerifyEnabled: NullableToBool(resp.MfaTotpVerifyEnabled),
	})
	diags.Append(d...)

	phone, d := buildObj(ctx, MFAPhoneModel{
		EnrollEnabled: NullableToBool(resp.MfaPhoneEnrollEnabled),
		VerifyEnabled: NullableToBool(resp.MfaPhoneVerifyEnabled),
		OtpLength:     types.Int64Value(int64(resp.MfaPhoneOtpLength)),
		Template:      NullableToString(resp.MfaPhoneTemplate),
		MaxFrequency:  NullableToInt64(resp.MfaPhoneMaxFrequency),
	})
	diags.Append(d...)

	webAuthn, d := buildObj(ctx, MFAWebAuthnModel{
		EnrollEnabled: NullableToBool(resp.MfaWebAuthnEnrollEnabled),
		VerifyEnabled: NullableToBool(resp.MfaWebAuthnVerifyEnabled),
	})
	diags.Append(d...)

	mfa, d := buildObj(ctx, MFAModel{
		MaxEnrolledFactors: NullableToInt64(resp.MfaMaxEnrolledFactors),
		TOTP:               totp,
		Phone:              phone,
		WebAuthn:           webAuthn,
	})
	diags.Append(d...)
	return mfa, diags
}

func mfaToBody(ctx context.Context, mfaObj types.Object, body *api.UpdateAuthConfigBody) diag.Diagnostics {
	if mfaObj.IsNull() || mfaObj.IsUnknown() {
		return nil
	}
	mfa, diags := readObj[MFAModel](ctx, mfaObj)

	if !mfa.MaxEnrolledFactors.IsNull() && !mfa.MaxEnrolledFactors.IsUnknown() {
		body.MfaMaxEnrolledFactors = nullable.NewNullableWithValue(int(mfa.MaxEnrolledFactors.ValueInt64()))
	}

	if !mfa.TOTP.IsNull() && !mfa.TOTP.IsUnknown() {
		totp, d := readObj[MFATotpModel](ctx, mfa.TOTP)
		diags.Append(d...)
		body.MfaTotpEnrollEnabled = setBool(totp.EnrollEnabled)
		body.MfaTotpVerifyEnabled = setBool(totp.VerifyEnabled)
	}

	if !mfa.Phone.IsNull() && !mfa.Phone.IsUnknown() {
		phone, d := readObj[MFAPhoneModel](ctx, mfa.Phone)
		diags.Append(d...)
		body.MfaPhoneEnrollEnabled = setBool(phone.EnrollEnabled)
		body.MfaPhoneVerifyEnabled = setBool(phone.VerifyEnabled)
		if !phone.OtpLength.IsNull() && !phone.OtpLength.IsUnknown() {
			body.MfaPhoneOtpLength = nullable.NewNullableWithValue(int(phone.OtpLength.ValueInt64()))
		}
		body.MfaPhoneTemplate = setStr(phone.Template)
		body.MfaPhoneMaxFrequency = setInt64(phone.MaxFrequency)
	}

	if !mfa.WebAuthn.IsNull() && !mfa.WebAuthn.IsUnknown() {
		webAuthn, d := readObj[MFAWebAuthnModel](ctx, mfa.WebAuthn)
		diags.Append(d...)
		body.MfaWebAuthnEnrollEnabled = setBool(webAuthn.EnrollEnabled)
		body.MfaWebAuthnVerifyEnabled = setBool(webAuthn.VerifyEnabled)
	}

	return diags
}
