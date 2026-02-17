# Settings can be imported using the project reference.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
#
# On import, all setting categories (API, auth, etc.) are read from the API.
# You can then selectively manage specific settings in your Terraform configuration.
terraform import supabase_settings.production <project_ref>
