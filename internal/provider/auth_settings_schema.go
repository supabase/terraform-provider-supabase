package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func (r *AuthSettingsResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Auth settings resource for a Supabase project.",

		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
			}),
		},

		Attributes: map[string]schema.Attribute{
			"project_ref": schema.StringAttribute{
				MarkdownDescription: "Project reference ID.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Resource identifier (same as project_ref).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_url": schema.StringAttribute{
				MarkdownDescription: "The base URL of the site.",
				Optional:            true,
				Computed:            true,
			},
			"jwt_exp": schema.Int64Attribute{
				MarkdownDescription: "JWT expiry in seconds.",
				Optional:            true,
				Computed:            true,
			},
			"disable_signup": schema.BoolAttribute{
				MarkdownDescription: "Disable new user sign-ups.",
				Optional:            true,
				Computed:            true,
			},
			"external_email_enabled": schema.BoolAttribute{
				MarkdownDescription: "Allow email sign-ins.",
				Optional:            true,
				Computed:            true,
			},
			"external_phone_enabled": schema.BoolAttribute{
				MarkdownDescription: "Allow phone sign-ins.",
				Optional:            true,
				Computed:            true,
			},
			"password_min_length": schema.Int64Attribute{
				MarkdownDescription: "Minimum password length.",
				Optional:            true,
				Computed:            true,
			},
			"smtp": schema.SingleNestedAttribute{
				MarkdownDescription: "Custom SMTP server settings.",
				Optional:            true,
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"admin_email": schema.StringAttribute{
						MarkdownDescription: "Sender email address.",
						Optional:            true,
						Computed:            true,
					},
					"host": schema.StringAttribute{
						MarkdownDescription: "SMTP host.",
						Optional:            true,
						Computed:            true,
					},
					"port": schema.StringAttribute{
						MarkdownDescription: "SMTP port.",
						Optional:            true,
						Computed:            true,
					},
					"user": schema.StringAttribute{
						MarkdownDescription: "SMTP username.",
						Optional:            true,
						Computed:            true,
					},
					"pass": schema.StringAttribute{
						MarkdownDescription: "SMTP password.",
						Optional:            true,
						Computed:            true,
						Sensitive:           true,
					},
					"sender_name": schema.StringAttribute{
						MarkdownDescription: "Display name for the sender.",
						Optional:            true,
						Computed:            true,
					},
					"max_frequency": schema.Int64Attribute{
						MarkdownDescription: "Minimum interval between emails sent to the same address (seconds).",
						Optional:            true,
						Computed:            true,
					},
				},
			},
			"external": schema.SingleNestedAttribute{
				MarkdownDescription: "OAuth provider configuration.",
				Optional:            true,
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"github":        schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"google":        schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"facebook":      schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"discord":       schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"twitter":       schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"x":             schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"gitlab":        schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthWithURLProviderAttributes()},
					"bitbucket":     schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"azure":         schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthWithURLProviderAttributes()},
					"notion":        schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"spotify":       schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"twitch":        schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"slack":         schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"slack_oidc":    schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"linkedin_oidc": schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"figma":         schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"kakao":         schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"workos":        schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthWithURLProviderAttributes()},
					"zoom":          schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthProviderAttributes()},
					"apple":         schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: appleProviderAttributes()},
					"keycloak":      schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: oauthWithURLProviderAttributes()},
				},
			},
			"hooks": schema.SingleNestedAttribute{
				MarkdownDescription: "Auth hook configuration.",
				Optional:            true,
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"send_email":                    schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: hookAttributes()},
					"send_sms":                      schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: hookAttributes()},
					"custom_access_token":           schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: hookAttributes()},
					"mfa_verification_attempt":      schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: hookAttributes()},
					"password_verification_attempt": schema.SingleNestedAttribute{Optional: true, Computed: true, Attributes: hookAttributes()},
				},
			},
			"mfa": schema.SingleNestedAttribute{
				MarkdownDescription: "Multi-factor authentication settings.",
				Optional:            true,
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"max_enrolled_factors": schema.Int64Attribute{
						MarkdownDescription: "Maximum number of enrolled MFA factors per user.",
						Optional:            true,
						Computed:            true,
					},
					"totp": schema.SingleNestedAttribute{
						MarkdownDescription: "TOTP factor settings.",
						Optional:            true,
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"enroll_enabled": schema.BoolAttribute{Optional: true, Computed: true},
							"verify_enabled": schema.BoolAttribute{Optional: true, Computed: true},
						},
					},
					"phone": schema.SingleNestedAttribute{
						MarkdownDescription: "Phone MFA factor settings.",
						Optional:            true,
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"enroll_enabled": schema.BoolAttribute{Optional: true, Computed: true},
							"verify_enabled": schema.BoolAttribute{Optional: true, Computed: true},
							"otp_length": schema.Int64Attribute{
								MarkdownDescription: "Length of the OTP code.",
								Optional:            true,
								Computed:            true,
							},
							"template": schema.StringAttribute{
								MarkdownDescription: "SMS template for MFA OTP.",
								Optional:            true,
								Computed:            true,
							},
							"max_frequency": schema.Int64Attribute{
								MarkdownDescription: "Minimum interval between MFA challenge requests (seconds).",
								Optional:            true,
								Computed:            true,
							},
						},
					},
					"web_authn": schema.SingleNestedAttribute{
						MarkdownDescription: "WebAuthn factor settings.",
						Optional:            true,
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"enroll_enabled": schema.BoolAttribute{Optional: true, Computed: true},
							"verify_enabled": schema.BoolAttribute{Optional: true, Computed: true},
						},
					},
				},
			},
			"security": schema.SingleNestedAttribute{
				MarkdownDescription: "Security settings.",
				Optional:            true,
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"captcha_enabled": schema.BoolAttribute{
						MarkdownDescription: "Enable CAPTCHA protection.",
						Optional:            true,
						Computed:            true,
					},
					"captcha_provider": schema.StringAttribute{
						MarkdownDescription: "CAPTCHA provider: `hcaptcha` or `turnstile`.",
						Optional:            true,
						Computed:            true,
					},
					"captcha_secret": schema.StringAttribute{
						MarkdownDescription: "CAPTCHA secret key.",
						Optional:            true,
						Computed:            true,
						Sensitive:           true,
					},
					"manual_linking_enabled": schema.BoolAttribute{
						MarkdownDescription: "Enable manual account linking.",
						Optional:            true,
						Computed:            true,
					},
				},
			},
			"sessions": schema.SingleNestedAttribute{
				MarkdownDescription: "Session settings.",
				Optional:            true,
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"timebox": schema.Int64Attribute{
						MarkdownDescription: "Maximum session duration (seconds). 0 means unlimited.",
						Optional:            true,
						Computed:            true,
					},
					"inactivity_timeout": schema.Int64Attribute{
						MarkdownDescription: "Inactivity timeout (seconds). 0 means unlimited.",
						Optional:            true,
						Computed:            true,
					},
					"single_per_user": schema.BoolAttribute{
						MarkdownDescription: "Allow only one active session per user.",
						Optional:            true,
						Computed:            true,
					},
				},
			},
			"secret_hashes": schema.MapAttribute{
				MarkdownDescription: "API-returned hashes for sensitive fields, keyed by API field name. Used for out-of-band drift detection.",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func oauthProviderAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"enabled":   schema.BoolAttribute{Optional: true, Computed: true},
		"client_id": schema.StringAttribute{Optional: true, Computed: true},
		"secret":    schema.StringAttribute{Optional: true, Computed: true, Sensitive: true},
	}
}

func oauthWithURLProviderAttributes() map[string]schema.Attribute {
	attrs := oauthProviderAttributes()
	attrs["url"] = schema.StringAttribute{Optional: true, Computed: true}
	return attrs
}

func appleProviderAttributes() map[string]schema.Attribute {
	attrs := oauthProviderAttributes()
	attrs["additional_client_ids"] = schema.StringAttribute{Optional: true, Computed: true}
	return attrs
}

func hookAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"enabled": schema.BoolAttribute{Optional: true, Computed: true},
		"uri":     schema.StringAttribute{Optional: true, Computed: true},
		"secrets": schema.StringAttribute{Optional: true, Computed: true, Sensitive: true},
	}
}
