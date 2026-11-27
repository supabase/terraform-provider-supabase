# Replace my-firebase-project with your Firebase project ID.
resource "supabase_third_party_auth" "firebase" {
  project_ref     = "mayuaycdtijbctgqbycg"
  oidc_issuer_url = "https://securetoken.google.com/my-firebase-project"
}
