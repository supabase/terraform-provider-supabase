# Branches can be imported using the branch ID.
#
# - branch_id: The UUID of the branch. Run `supabase branches list` to find it.
#
# For a persistent branch, set `persistent = true` in the resource configuration before the first plan or apply after import.
# Import cannot currently determine the branch's existing persistence setting.
terraform import supabase_branch.development <branch_id>
