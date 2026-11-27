# Your Cognito user pool's region and ID.
resource "supabase_third_party_auth" "aws_cognito" {
  project_ref     = "mayuaycdtijbctgqbycg"
  oidc_issuer_url = "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_AbCdEfGhI"
}
