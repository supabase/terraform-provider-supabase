resource "supabase_custom_hostname" "example" {
  project_ref     = "mayuaycdtijbctgqbycg"
  custom_hostname = "api.example.com"

  # After apply, create these DNS records, then set activate = true:
  # - CNAME custom_hostname -> custom_origin_server
  # - TXT ownership_verification.name -> ownership_verification.value
  # - TXT ssl.validation_records[*].txt_name -> ssl.validation_records[*].txt_value
  #
  # activate = true
}
