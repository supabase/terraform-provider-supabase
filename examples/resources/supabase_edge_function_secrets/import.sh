# Edge function secrets can be imported using the project reference.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
#
# IMPORTANT: The Supabase management API only returns SHA-256 digests of secret
# values, never the plaintext. After import:
#   - `secret_digests` is populated with the digests returned by the API.
#   - `secrets[*].value` is null for every imported secret.
#   - Secrets with a SUPABASE_ prefix are not imported (they are reserved).
#
# After running this command, supply the plaintext secret values in your Terraform
# configuration and run `terraform plan` to reconcile state. Terraform will show
# a plan to update the secrets to match the values in your config.
#
# Note: Secret values are sensitive. Always use a remote state backend with
# encryption to protect sensitive values stored in Terraform state.
terraform import supabase_edge_function_secrets.example mayuaycdtijbctgqbycg
