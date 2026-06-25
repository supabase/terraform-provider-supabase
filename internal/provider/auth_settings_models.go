package provider

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type AuthSettingsResourceModel struct {
	ProjectRef           types.String   `tfsdk:"project_ref"`
	Id                   types.String   `tfsdk:"id"`
	SiteUrl              types.String   `tfsdk:"site_url"`
	JwtExp               types.Int64    `tfsdk:"jwt_exp"`
	DisableSignup        types.Bool     `tfsdk:"disable_signup"`
	ExternalEmailEnabled types.Bool     `tfsdk:"external_email_enabled"`
	ExternalPhoneEnabled types.Bool     `tfsdk:"external_phone_enabled"`
	PasswordMinLength    types.Int64    `tfsdk:"password_min_length"`
	SMTP                 types.Object   `tfsdk:"smtp"`
	External             types.Object   `tfsdk:"external"`
	Hooks                types.Object   `tfsdk:"hooks"`
	MFA                  types.Object   `tfsdk:"mfa"`
	Security             types.Object   `tfsdk:"security"`
	Sessions             types.Object   `tfsdk:"sessions"`
	SecretHashes         types.Map      `tfsdk:"secret_hashes"`
	Timeouts             timeouts.Value `tfsdk:"timeouts"`
}

type SMTPModel struct {
	AdminEmail   types.String `tfsdk:"admin_email"`
	Host         types.String `tfsdk:"host"`
	Port         types.String `tfsdk:"port"`
	User         types.String `tfsdk:"user"`
	Pass         types.String `tfsdk:"pass"`
	SenderName   types.String `tfsdk:"sender_name"`
	MaxFrequency types.Int64  `tfsdk:"max_frequency"`
}

func (m SMTPModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"admin_email":   types.StringType,
		"host":          types.StringType,
		"port":          types.StringType,
		"user":          types.StringType,
		"pass":          types.StringType,
		"sender_name":   types.StringType,
		"max_frequency": types.Int64Type,
	}
}

type OAuthProviderModel struct {
	Enabled  types.Bool   `tfsdk:"enabled"`
	ClientId types.String `tfsdk:"client_id"`
	Secret   types.String `tfsdk:"secret"`
}

func (m OAuthProviderModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enabled":   types.BoolType,
		"client_id": types.StringType,
		"secret":    types.StringType,
	}
}

type OAuthProviderWithURLModel struct {
	Enabled  types.Bool   `tfsdk:"enabled"`
	ClientId types.String `tfsdk:"client_id"`
	Secret   types.String `tfsdk:"secret"`
	URL      types.String `tfsdk:"url"`
}

func (m OAuthProviderWithURLModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enabled":   types.BoolType,
		"client_id": types.StringType,
		"secret":    types.StringType,
		"url":       types.StringType,
	}
}

type AppleProviderModel struct {
	Enabled             types.Bool   `tfsdk:"enabled"`
	ClientId            types.String `tfsdk:"client_id"`
	Secret              types.String `tfsdk:"secret"`
	AdditionalClientIds types.String `tfsdk:"additional_client_ids"`
}

func (m AppleProviderModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enabled":               types.BoolType,
		"client_id":             types.StringType,
		"secret":                types.StringType,
		"additional_client_ids": types.StringType,
	}
}

type ExternalModel struct {
	GitHub       types.Object `tfsdk:"github"`
	Google       types.Object `tfsdk:"google"`
	Facebook     types.Object `tfsdk:"facebook"`
	Discord      types.Object `tfsdk:"discord"`
	Twitter      types.Object `tfsdk:"twitter"`
	X            types.Object `tfsdk:"x"`
	Gitlab       types.Object `tfsdk:"gitlab"`
	Bitbucket    types.Object `tfsdk:"bitbucket"`
	Azure        types.Object `tfsdk:"azure"`
	Notion       types.Object `tfsdk:"notion"`
	Spotify      types.Object `tfsdk:"spotify"`
	Twitch       types.Object `tfsdk:"twitch"`
	Slack        types.Object `tfsdk:"slack"`
	SlackOIDC    types.Object `tfsdk:"slack_oidc"`
	LinkedInOIDC types.Object `tfsdk:"linkedin_oidc"`
	Figma        types.Object `tfsdk:"figma"`
	Kakao        types.Object `tfsdk:"kakao"`
	Workos       types.Object `tfsdk:"workos"`
	Zoom         types.Object `tfsdk:"zoom"`
	Apple        types.Object `tfsdk:"apple"`
	Keycloak     types.Object `tfsdk:"keycloak"`
}

func (m ExternalModel) AttributeTypes() map[string]attr.Type {
	std := OAuthProviderModel{}.AttributeTypes()
	withURL := OAuthProviderWithURLModel{}.AttributeTypes()
	apple := AppleProviderModel{}.AttributeTypes()
	return map[string]attr.Type{
		"github":        types.ObjectType{AttrTypes: std},
		"google":        types.ObjectType{AttrTypes: std},
		"facebook":      types.ObjectType{AttrTypes: std},
		"discord":       types.ObjectType{AttrTypes: std},
		"twitter":       types.ObjectType{AttrTypes: std},
		"x":             types.ObjectType{AttrTypes: std},
		"gitlab":        types.ObjectType{AttrTypes: withURL},
		"bitbucket":     types.ObjectType{AttrTypes: std},
		"azure":         types.ObjectType{AttrTypes: withURL},
		"notion":        types.ObjectType{AttrTypes: std},
		"spotify":       types.ObjectType{AttrTypes: std},
		"twitch":        types.ObjectType{AttrTypes: std},
		"slack":         types.ObjectType{AttrTypes: std},
		"slack_oidc":    types.ObjectType{AttrTypes: std},
		"linkedin_oidc": types.ObjectType{AttrTypes: std},
		"figma":         types.ObjectType{AttrTypes: std},
		"kakao":         types.ObjectType{AttrTypes: std},
		"workos":        types.ObjectType{AttrTypes: withURL},
		"zoom":          types.ObjectType{AttrTypes: std},
		"apple":         types.ObjectType{AttrTypes: apple},
		"keycloak":      types.ObjectType{AttrTypes: withURL},
	}
}

type HookModel struct {
	Enabled types.Bool   `tfsdk:"enabled"`
	URI     types.String `tfsdk:"uri"`
	Secrets types.String `tfsdk:"secrets"`
}

func (m HookModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enabled": types.BoolType,
		"uri":     types.StringType,
		"secrets": types.StringType,
	}
}

type HooksModel struct {
	SendEmail                   types.Object `tfsdk:"send_email"`
	SendSMS                     types.Object `tfsdk:"send_sms"`
	CustomAccessToken           types.Object `tfsdk:"custom_access_token"`
	MFAVerificationAttempt      types.Object `tfsdk:"mfa_verification_attempt"`
	PasswordVerificationAttempt types.Object `tfsdk:"password_verification_attempt"`
}

func (m HooksModel) AttributeTypes() map[string]attr.Type {
	h := HookModel{}.AttributeTypes()
	return map[string]attr.Type{
		"send_email":                    types.ObjectType{AttrTypes: h},
		"send_sms":                      types.ObjectType{AttrTypes: h},
		"custom_access_token":           types.ObjectType{AttrTypes: h},
		"mfa_verification_attempt":      types.ObjectType{AttrTypes: h},
		"password_verification_attempt": types.ObjectType{AttrTypes: h},
	}
}

type MFATotpModel struct {
	EnrollEnabled types.Bool `tfsdk:"enroll_enabled"`
	VerifyEnabled types.Bool `tfsdk:"verify_enabled"`
}

func (m MFATotpModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enroll_enabled": types.BoolType,
		"verify_enabled": types.BoolType,
	}
}

type MFAPhoneModel struct {
	EnrollEnabled types.Bool   `tfsdk:"enroll_enabled"`
	VerifyEnabled types.Bool   `tfsdk:"verify_enabled"`
	OtpLength     types.Int64  `tfsdk:"otp_length"`
	Template      types.String `tfsdk:"template"`
	MaxFrequency  types.Int64  `tfsdk:"max_frequency"`
}

func (m MFAPhoneModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enroll_enabled": types.BoolType,
		"verify_enabled": types.BoolType,
		"otp_length":     types.Int64Type,
		"template":       types.StringType,
		"max_frequency":  types.Int64Type,
	}
}

type MFAWebAuthnModel struct {
	EnrollEnabled types.Bool `tfsdk:"enroll_enabled"`
	VerifyEnabled types.Bool `tfsdk:"verify_enabled"`
}

func (m MFAWebAuthnModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enroll_enabled": types.BoolType,
		"verify_enabled": types.BoolType,
	}
}

type MFAModel struct {
	MaxEnrolledFactors types.Int64  `tfsdk:"max_enrolled_factors"`
	TOTP               types.Object `tfsdk:"totp"`
	Phone              types.Object `tfsdk:"phone"`
	WebAuthn           types.Object `tfsdk:"web_authn"`
}

func (m MFAModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"max_enrolled_factors": types.Int64Type,
		"totp":                 types.ObjectType{AttrTypes: MFATotpModel{}.AttributeTypes()},
		"phone":                types.ObjectType{AttrTypes: MFAPhoneModel{}.AttributeTypes()},
		"web_authn":            types.ObjectType{AttrTypes: MFAWebAuthnModel{}.AttributeTypes()},
	}
}

type SecurityModel struct {
	CaptchaEnabled       types.Bool   `tfsdk:"captcha_enabled"`
	CaptchaProvider      types.String `tfsdk:"captcha_provider"`
	CaptchaSecret        types.String `tfsdk:"captcha_secret"`
	ManualLinkingEnabled types.Bool   `tfsdk:"manual_linking_enabled"`
}

func (m SecurityModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"captcha_enabled":        types.BoolType,
		"captcha_provider":       types.StringType,
		"captcha_secret":         types.StringType,
		"manual_linking_enabled": types.BoolType,
	}
}

type SessionsModel struct {
	Timebox           types.Int64 `tfsdk:"timebox"`
	InactivityTimeout types.Int64 `tfsdk:"inactivity_timeout"`
	SinglePerUser     types.Bool  `tfsdk:"single_per_user"`
}

func (m SessionsModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"timebox":            types.Int64Type,
		"inactivity_timeout": types.Int64Type,
		"single_per_user":    types.BoolType,
	}
}
