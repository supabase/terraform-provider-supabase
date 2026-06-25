package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

// secretResolver decides what value to store for a sensitive field.
//
// On Create/Update (driftCheck=false) the plan value is returned as-is so the
// user's plaintext is preserved in state.
//
// On Read (driftCheck=true) the API-returned hash is compared against the stored
// hash; a mismatch means an out-of-band change and the plaintext is nulled out so
// the next plan shows a diff against the HCL config.
type secretResolver func(apiKey string, priorVal types.String) types.String

// extractAuthSecretHashes pulls the API-returned hashes for all sensitive fields
// from an AuthConfigResponse. The API never returns plaintext — only hashes.
func extractAuthSecretHashes(resp *api.AuthConfigResponse) map[string]string {
	hashes := map[string]string{}
	set := func(key string, n nullable.Nullable[string]) {
		if n.IsSpecified() && !n.IsNull() {
			hashes[key] = n.MustGet()
		}
	}
	set("smtp_pass", resp.SmtpPass)
	set("security_captcha_secret", resp.SecurityCaptchaSecret)
	set("hook_send_email_secrets", resp.HookSendEmailSecrets)
	set("hook_send_sms_secrets", resp.HookSendSmsSecrets)
	set("hook_custom_access_token_secrets", resp.HookCustomAccessTokenSecrets)
	set("hook_mfa_verification_attempt_secrets", resp.HookMfaVerificationAttemptSecrets)
	set("hook_password_verification_attempt_secrets", resp.HookPasswordVerificationAttemptSecrets)
	set("external_github_secret", resp.ExternalGithubSecret)
	set("external_google_secret", resp.ExternalGoogleSecret)
	set("external_facebook_secret", resp.ExternalFacebookSecret)
	set("external_discord_secret", resp.ExternalDiscordSecret)
	set("external_twitter_secret", resp.ExternalTwitterSecret)
	set("external_x_secret", resp.ExternalXSecret)
	set("external_gitlab_secret", resp.ExternalGitlabSecret)
	set("external_bitbucket_secret", resp.ExternalBitbucketSecret)
	set("external_azure_secret", resp.ExternalAzureSecret)
	set("external_notion_secret", resp.ExternalNotionSecret)
	set("external_spotify_secret", resp.ExternalSpotifySecret)
	set("external_twitch_secret", resp.ExternalTwitchSecret)
	set("external_slack_secret", resp.ExternalSlackSecret)
	set("external_slack_oidc_secret", resp.ExternalSlackOidcSecret)
	set("external_linkedin_oidc_secret", resp.ExternalLinkedinOidcSecret)
	set("external_figma_secret", resp.ExternalFigmaSecret)
	set("external_kakao_secret", resp.ExternalKakaoSecret)
	set("external_workos_secret", resp.ExternalWorkosSecret)
	set("external_zoom_secret", resp.ExternalZoomSecret)
	set("external_apple_secret", resp.ExternalAppleSecret)
	set("external_keycloak_secret", resp.ExternalKeycloakSecret)
	return hashes
}

// priorSecretHashes extracts the stored hash map from prior state.
// Returns an empty map when prior is nil or secret_hashes is null/unknown.
func priorSecretHashes(ctx context.Context, prior *AuthSettingsResourceModel) map[string]string {
	if prior == nil || prior.SecretHashes.IsNull() || prior.SecretHashes.IsUnknown() {
		return map[string]string{}
	}
	result := map[string]string{}
	prior.SecretHashes.ElementsAs(ctx, &result, false)
	return result
}

// hashMapValue converts a flat string map into a types.Map for the secret_hashes attribute.
func hashMapValue(hashes map[string]string) (types.Map, diag.Diagnostics) {
	elems := make(map[string]attr.Value, len(hashes))
	for k, v := range hashes {
		elems[k] = types.StringValue(v)
	}
	return types.MapValue(types.StringType, elems)
}
