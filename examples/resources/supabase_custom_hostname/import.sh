# Custom hostnames can be imported using the project reference.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
#
# A project has one custom hostname. Import leaves activate false. Set
# activate = true in configuration to reverify and activate when status is not
# already 5_services_reconfigured.
terraform import supabase_custom_hostname.example <project_ref>
