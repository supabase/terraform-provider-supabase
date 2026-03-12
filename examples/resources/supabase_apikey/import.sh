# API keys can be imported using the project reference and the key's UUID,
# separated by a '/'.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
# - api_key_id: The `id` field of the target key from the JSON output of:
#     supabase projects api-keys --output json --project-ref <project_ref>
terraform import supabase_apikey.example <project_ref>/<api_key_id>
