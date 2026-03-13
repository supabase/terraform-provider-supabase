# Edge function secrets can be imported using the project reference.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
#
# Note: Secret values are sensitive and will be stored in Terraform state.
# Ensure your state file is properly secured.
terraform import supabase_edge_function_secrets.example mayuaycdtijbctgqbycg
