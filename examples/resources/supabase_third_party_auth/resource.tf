resource "supabase_third_party_auth" "oidc" {
  project_ref     = "mayuaycdtijbctgqbycg"
  oidc_issuer_url = "https://issuer.example.com"
}

resource "supabase_third_party_auth" "jwks_url" {
  project_ref = "mayuaycdtijbctgqbycg"
  jwks_url    = "https://issuer.example.com/.well-known/jwks.json"
}

resource "supabase_third_party_auth" "custom_jwks" {
  project_ref = "mayuaycdtijbctgqbycg"

  # custom_jwks must contain public JWKS material only.
  custom_jwks = jsonencode({
    keys = [
      {
        kty = "RSA"
        kid = "example-key"
        n   = "example-modulus"
        e   = "AQAB"
      }
    ]
  })
}
