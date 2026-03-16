# Migrations can be imported using the project reference.
#
# - project_ref: Found in the Supabase dashboard under Project Settings -> General,
#   or in the project's URL: https://supabase.com/dashboard/project/<project_ref>
#
# IMPORTANT: The Supabase management API does not currently expose a list of
# applied migrations. After import:
#   - The resource is created with an empty `migrations` list.
#   - You must manually add migration entries to match what has been applied,
#     or plan to apply new migrations.
#
# Import will:
#   - Verify the project exists
#   - Create state with `project_ref` and an empty `migrations` list
#   - Issue a warning about the import limitation
#
# After running this command:
# 1. Add migration entries to your Terraform configuration for any migrations
#    that were previously applied (if you have historical records), OR
# 2. Start fresh by adding only new migrations that will be applied going forward
#
# Note: Migration content is stored in state (marked sensitive). Always use a
# remote state backend with encryption to protect sensitive values.
terraform import supabase_migrations.example mayuaycdtijbctgqbycg
