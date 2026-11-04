# Vault secrets can be imported with the project reference and the secret name,
# separated by a '/'.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
# - name: The unique name passed to vault.create_secret.
#
# The import reads vault.decrypted_secrets, so the plaintext value is stored in
# state. Use a remote state backend with encryption.
terraform import supabase_vault_secret.example <project_ref>/<name>

# Alternatively, import with the vault secret UUID.
#
# - id: The UUID returned by vault.create_secret, also stored in this resource's id.
#
# A name that is itself a valid UUID is imported as an id.
terraform import supabase_vault_secret.example <project_ref>/<id>
