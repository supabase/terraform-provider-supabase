# API keys can be imported using the project reference and the key's UUID,
# separated by a '/'.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
# - key_id: The UUID of the API key
terraform import supabase_apikey.example <project_ref>/<key_id>
