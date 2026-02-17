# Edge functions can be imported using the project reference and the function slug,
# separated by a '/'.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
# - slug: Found in the Supabase dashboard under Edge Functions,
#   or in the function's URL: https://supabase.com/dashboard/project/<project_ref>/functions/<slug>
#
# Note: Local-only fields (entrypoint, import_map, etc.) will be null after
# import and must be set manually in your Terraform configuration.
terraform import supabase_edge_function.example <project_ref>/<slug>
