# Migrations Resource Example

This example demonstrates how to use the `supabase_migrations` resource to apply SQL migrations to a Supabase project database.

## Prerequisites

1. A Supabase project (get the project ref from your dashboard)
2. Supabase access token (generate from [Account Settings](https://supabase.com/dashboard/account/tokens))
3. Migration SQL files

## Example Structure

```
.
├── resource.tf              # Terraform configuration
├── import.sh               # Import command example
└── migrations/             # SQL migration files
    ├── 001_initial_schema.sql
    ├── 002_add_users_table.sql
    └── 003_add_posts_table.sql
```

## Usage

1. **Set your Supabase credentials:**

```bash
export SUPABASE_ACCESS_TOKEN="your-access-token-here"
```

2. **Update the project_ref in resource.tf:**

Replace `mayuaycdtijbctgqbycg` with your actual project reference.

3. **Initialize Terraform:**

```bash
terraform init
```

4. **Review the plan:**

```bash
terraform plan
```

This will:
- Discover all `.sql` files under `migrations_dir` at plan time
- Compute SHA-256 digests for each migration
- Show what migrations will be applied

5. **Apply migrations:**

```bash
terraform apply
```

This will apply migrations sequentially in the order specified.

## Adding New Migrations

To add new migrations:

1. Create a new SQL file (e.g., `004_add_comments_table.sql`)
2. Place it in the `migrations/` directory.

The resource configuration stays simple:

```hcl
resource "supabase_migrations" "example" {
  project_ref = "your-project-ref"
  migrations_dir = "./migrations"
}
```

3. Run `terraform apply`

**Important**: Never modify existing migration files after they've been applied. Always create new migrations.

## Migration Best Practices

1. **Use sequential numbering**: Name files like `001_description.sql`, `002_description.sql`, etc.
2. **One logical change per migration**: Keep migrations focused and small
3. **Use idempotent SQL**: Include `IF NOT EXISTS` clauses where possible
4. **Test in development first**: Apply to a test project before production
5. **Add comments**: Document why each migration is needed
6. **Never modify applied migrations**: Create new migrations to make changes
7. **Version control**: Commit migration files to Git
8. **Backup before applying**: Always have a database backup before applying to production

## Import Existing Migrations

If you have an existing project with migrations already applied:

```bash
terraform import supabase_migrations.example your-project-ref
```

**Note**: The import will create the resource with an empty migrations list. You'll need to manually add entries to match what was previously applied.

## Limitations

- **No rollback**: Once applied, migrations cannot be automatically rolled back
- **Append-only**: You cannot remove or reorder migrations, only append new ones
- **File-based**: Migration content must be in files (inline SQL not supported)
- **API availability**: The Supabase API client must include the migration endpoint

## Troubleshooting

### Migration fails to apply

If a migration fails:
1. Check the error message for SQL syntax issues
2. Verify the migration file exists and is readable
3. Review the state to see which migrations were successfully applied
4. Fix the failing migration and run `terraform apply` again

### Cannot modify existing migration

If you see an error about modifying an existing migration:
- You likely changed the content of an already-applied migration file
- Create a new migration instead
- Or use `terraform state rm` to remove the resource and recreate it (use with caution)

### API method not available

If you see an error about `V1ApplyAMigrationWithResponse` not being available:
- The API client needs to be regenerated with the updated OpenAPI spec
- Contact the provider maintainers or wait for a provider update
