# Third-party auth integrations can be imported using the project reference and
# the third-party auth integration ID, separated by a '/'.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
# - third_party_auth_id: The UUID of the third-party auth integration.
terraform import supabase_third_party_auth.oidc <project_ref>/<third_party_auth_id>
