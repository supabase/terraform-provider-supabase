# Branches can be imported using the branch ID.
#
# - branch_id: The UUID of the branch. Retrieve it via the Supabase CLI:
#     supabase branches list
terraform import supabase_branch.development <branch_id>
