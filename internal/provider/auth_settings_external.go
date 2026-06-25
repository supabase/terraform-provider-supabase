package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
	"github.com/supabase/cli/pkg/api"
)

// stdProvider describes a standard OAuth provider (enabled + client_id + secret).
// All fields are function pointers so the table is data-only — zero call-site branching.
type stdProvider struct {
	secretKey string
	// API response readers
	enabled  func(*api.AuthConfigResponse) nullable.Nullable[bool]
	clientId func(*api.AuthConfigResponse) nullable.Nullable[string]
	secret   func(*api.AuthConfigResponse) nullable.Nullable[string]
	// TF model field accessors (used for both prior-state reads and result writes)
	getField func(*ExternalModel) types.Object
	setField func(*ExternalModel, types.Object)
	// API body field pointers
	bEnabled  func(*api.UpdateAuthConfigBody) *nullable.Nullable[bool]
	bClientId func(*api.UpdateAuthConfigBody) *nullable.Nullable[string]
	bSecret   func(*api.UpdateAuthConfigBody) *nullable.Nullable[string]
}

// urlProvider extends stdProvider with an extra URL field (Gitlab, Azure, Workos, Keycloak).
type urlProvider struct {
	stdProvider
	url  func(*api.AuthConfigResponse) nullable.Nullable[string]
	bUrl func(*api.UpdateAuthConfigBody) *nullable.Nullable[string]
}

// stdProviders is the exhaustive table of all 17 standard OAuth providers.
// Adding a new provider = one entry here.
var stdProviders = []stdProvider{
	{
		secretKey: "external_github_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalGithubEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalGithubClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalGithubSecret },
		getField:  func(e *ExternalModel) types.Object { return e.GitHub },
		setField:  func(e *ExternalModel, o types.Object) { e.GitHub = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalGithubEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalGithubClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalGithubSecret },
	},
	{
		secretKey: "external_google_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalGoogleEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalGoogleClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalGoogleSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Google },
		setField:  func(e *ExternalModel, o types.Object) { e.Google = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalGoogleEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalGoogleClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalGoogleSecret },
	},
	{
		secretKey: "external_facebook_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalFacebookEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalFacebookClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalFacebookSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Facebook },
		setField:  func(e *ExternalModel, o types.Object) { e.Facebook = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalFacebookEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalFacebookClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalFacebookSecret },
	},
	{
		secretKey: "external_discord_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalDiscordEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalDiscordClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalDiscordSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Discord },
		setField:  func(e *ExternalModel, o types.Object) { e.Discord = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalDiscordEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalDiscordClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalDiscordSecret },
	},
	{
		secretKey: "external_twitter_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalTwitterEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalTwitterClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalTwitterSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Twitter },
		setField:  func(e *ExternalModel, o types.Object) { e.Twitter = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalTwitterEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalTwitterClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalTwitterSecret },
	},
	{
		secretKey: "external_x_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalXEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalXClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalXSecret },
		getField:  func(e *ExternalModel) types.Object { return e.X },
		setField:  func(e *ExternalModel, o types.Object) { e.X = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalXEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalXClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalXSecret },
	},
	{
		secretKey: "external_bitbucket_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalBitbucketEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalBitbucketClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalBitbucketSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Bitbucket },
		setField:  func(e *ExternalModel, o types.Object) { e.Bitbucket = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalBitbucketEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalBitbucketClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalBitbucketSecret },
	},
	{
		secretKey: "external_notion_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalNotionEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalNotionClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalNotionSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Notion },
		setField:  func(e *ExternalModel, o types.Object) { e.Notion = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalNotionEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalNotionClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalNotionSecret },
	},
	{
		secretKey: "external_spotify_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalSpotifyEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalSpotifyClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalSpotifySecret },
		getField:  func(e *ExternalModel) types.Object { return e.Spotify },
		setField:  func(e *ExternalModel, o types.Object) { e.Spotify = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalSpotifyEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalSpotifyClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalSpotifySecret },
	},
	{
		secretKey: "external_twitch_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalTwitchEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalTwitchClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalTwitchSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Twitch },
		setField:  func(e *ExternalModel, o types.Object) { e.Twitch = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalTwitchEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalTwitchClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalTwitchSecret },
	},
	{
		secretKey: "external_slack_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalSlackEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalSlackClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalSlackSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Slack },
		setField:  func(e *ExternalModel, o types.Object) { e.Slack = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalSlackEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalSlackClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalSlackSecret },
	},
	{
		secretKey: "external_slack_oidc_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalSlackOidcEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalSlackOidcClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalSlackOidcSecret },
		getField:  func(e *ExternalModel) types.Object { return e.SlackOIDC },
		setField:  func(e *ExternalModel, o types.Object) { e.SlackOIDC = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalSlackOidcEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalSlackOidcClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalSlackOidcSecret },
	},
	{
		secretKey: "external_linkedin_oidc_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalLinkedinOidcEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalLinkedinOidcClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalLinkedinOidcSecret },
		getField:  func(e *ExternalModel) types.Object { return e.LinkedInOIDC },
		setField:  func(e *ExternalModel, o types.Object) { e.LinkedInOIDC = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalLinkedinOidcEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalLinkedinOidcClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalLinkedinOidcSecret },
	},
	{
		secretKey: "external_figma_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalFigmaEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalFigmaClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalFigmaSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Figma },
		setField:  func(e *ExternalModel, o types.Object) { e.Figma = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalFigmaEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalFigmaClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalFigmaSecret },
	},
	{
		secretKey: "external_kakao_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalKakaoEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalKakaoClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalKakaoSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Kakao },
		setField:  func(e *ExternalModel, o types.Object) { e.Kakao = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalKakaoEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalKakaoClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalKakaoSecret },
	},
	{
		secretKey: "external_zoom_secret",
		enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalZoomEnabled },
		clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalZoomClientId },
		secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalZoomSecret },
		getField:  func(e *ExternalModel) types.Object { return e.Zoom },
		setField:  func(e *ExternalModel, o types.Object) { e.Zoom = o },
		bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalZoomEnabled },
		bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalZoomClientId },
		bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalZoomSecret },
	},
}

// urlProviders is the table for the 4 providers that also have a URL field.
var urlProviders = []urlProvider{
	{
		stdProvider: stdProvider{
			secretKey: "external_gitlab_secret",
			enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalGitlabEnabled },
			clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalGitlabClientId },
			secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalGitlabSecret },
			getField:  func(e *ExternalModel) types.Object { return e.Gitlab },
			setField:  func(e *ExternalModel, o types.Object) { e.Gitlab = o },
			bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalGitlabEnabled },
			bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalGitlabClientId },
			bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalGitlabSecret },
		},
		url:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalGitlabUrl },
		bUrl: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalGitlabUrl },
	},
	{
		stdProvider: stdProvider{
			secretKey: "external_azure_secret",
			enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalAzureEnabled },
			clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalAzureClientId },
			secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalAzureSecret },
			getField:  func(e *ExternalModel) types.Object { return e.Azure },
			setField:  func(e *ExternalModel, o types.Object) { e.Azure = o },
			bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalAzureEnabled },
			bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalAzureClientId },
			bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalAzureSecret },
		},
		url:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalAzureUrl },
		bUrl: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalAzureUrl },
	},
	{
		stdProvider: stdProvider{
			secretKey: "external_workos_secret",
			enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalWorkosEnabled },
			clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalWorkosClientId },
			secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalWorkosSecret },
			getField:  func(e *ExternalModel) types.Object { return e.Workos },
			setField:  func(e *ExternalModel, o types.Object) { e.Workos = o },
			bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalWorkosEnabled },
			bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalWorkosClientId },
			bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalWorkosSecret },
		},
		url:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalWorkosUrl },
		bUrl: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalWorkosUrl },
	},
	{
		stdProvider: stdProvider{
			secretKey: "external_keycloak_secret",
			enabled:   func(r *api.AuthConfigResponse) nullable.Nullable[bool]   { return r.ExternalKeycloakEnabled },
			clientId:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalKeycloakClientId },
			secret:    func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalKeycloakSecret },
			getField:  func(e *ExternalModel) types.Object { return e.Keycloak },
			setField:  func(e *ExternalModel, o types.Object) { e.Keycloak = o },
			bEnabled:  func(b *api.UpdateAuthConfigBody) *nullable.Nullable[bool]   { return &b.ExternalKeycloakEnabled },
			bClientId: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalKeycloakClientId },
			bSecret:   func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalKeycloakSecret },
		},
		url:  func(r *api.AuthConfigResponse) nullable.Nullable[string] { return r.ExternalKeycloakUrl },
		bUrl: func(b *api.UpdateAuthConfigBody) *nullable.Nullable[string] { return &b.ExternalKeycloakUrl },
	},
}

func externalToModel(ctx context.Context, resp *api.AuthConfigResponse, prior *AuthSettingsResourceModel, resolve secretResolver) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	var priorExt ExternalModel
	if prior != nil && !prior.External.IsNull() && !prior.External.IsUnknown() {
		var d diag.Diagnostics
		priorExt, d = readObj[ExternalModel](ctx, prior.External)
		diags.Append(d...)
	}

	ext := ExternalModel{}

	for _, p := range stdProviders {
		priorSecret := types.StringNull()
		priorM, d := readObj[OAuthProviderModel](ctx, p.getField(&priorExt))
		diags.Append(d...)
		if !d.HasError() {
			priorSecret = priorM.Secret
		}
		obj, d := buildObj(ctx, OAuthProviderModel{
			Enabled:  NullableToBool(p.enabled(resp)),
			ClientId: NullableToString(p.clientId(resp)),
			Secret:   resolve(p.secretKey, priorSecret),
		})
		diags.Append(d...)
		p.setField(&ext, obj)
	}

	for _, p := range urlProviders {
		priorSecret := types.StringNull()
		priorM, d := readObj[OAuthProviderWithURLModel](ctx, p.getField(&priorExt))
		diags.Append(d...)
		if !d.HasError() {
			priorSecret = priorM.Secret
		}
		obj, d := buildObj(ctx, OAuthProviderWithURLModel{
			Enabled:  NullableToBool(p.enabled(resp)),
			ClientId: NullableToString(p.clientId(resp)),
			Secret:   resolve(p.secretKey, priorSecret),
			URL:      NullableToString(p.url(resp)),
		})
		diags.Append(d...)
		p.setField(&ext, obj)
	}

	// Apple has a unique shape (additional_client_ids) so it's handled explicitly.
	priorAppleSecret := types.StringNull()
	priorApple, d := readObj[AppleProviderModel](ctx, priorExt.Apple)
	diags.Append(d...)
	if !d.HasError() {
		priorAppleSecret = priorApple.Secret
	}
	appleObj, d := buildObj(ctx, AppleProviderModel{
		Enabled:             NullableToBool(resp.ExternalAppleEnabled),
		ClientId:            NullableToString(resp.ExternalAppleClientId),
		Secret:              resolve("external_apple_secret", priorAppleSecret),
		AdditionalClientIds: NullableToString(resp.ExternalAppleAdditionalClientIds),
	})
	diags.Append(d...)
	ext.Apple = appleObj

	result, d := buildObj(ctx, ext)
	diags.Append(d...)
	return result, diags
}

func externalToBody(ctx context.Context, extObj types.Object, body *api.UpdateAuthConfigBody) diag.Diagnostics {
	if extObj.IsNull() || extObj.IsUnknown() {
		return nil
	}
	ext, diags := readObj[ExternalModel](ctx, extObj)

	for _, p := range stdProviders {
		obj := p.getField(&ext)
		if obj.IsNull() || obj.IsUnknown() {
			continue
		}
		m, d := readObj[OAuthProviderModel](ctx, obj)
		diags.Append(d...)
		*p.bEnabled(body) = setBool(m.Enabled)
		*p.bClientId(body) = setStr(m.ClientId)
		if !m.Secret.IsNull() && !m.Secret.IsUnknown() {
			*p.bSecret(body) = nullable.NewNullableWithValue(m.Secret.ValueString())
		}
	}

	for _, p := range urlProviders {
		obj := p.getField(&ext)
		if obj.IsNull() || obj.IsUnknown() {
			continue
		}
		m, d := readObj[OAuthProviderWithURLModel](ctx, obj)
		diags.Append(d...)
		*p.bEnabled(body) = setBool(m.Enabled)
		*p.bClientId(body) = setStr(m.ClientId)
		if !m.Secret.IsNull() && !m.Secret.IsUnknown() {
			*p.bSecret(body) = nullable.NewNullableWithValue(m.Secret.ValueString())
		}
		*p.bUrl(body) = setStr(m.URL)
	}

	if !ext.Apple.IsNull() && !ext.Apple.IsUnknown() {
		apple, d := readObj[AppleProviderModel](ctx, ext.Apple)
		diags.Append(d...)
		body.ExternalAppleEnabled = setBool(apple.Enabled)
		body.ExternalAppleClientId = setStr(apple.ClientId)
		if !apple.Secret.IsNull() && !apple.Secret.IsUnknown() {
			body.ExternalAppleSecret = nullable.NewNullableWithValue(apple.Secret.ValueString())
		}
		body.ExternalAppleAdditionalClientIds = setStr(apple.AdditionalClientIds)
	}

	return diags
}
