# Using the Supabase Terraform Provider

## Setting up a TF module

1. Create a Personal Access Token in the [Supabase Dashboard](https://supabase.com/dashboard/account/tokens) by going to `Account preferences` > `Access Tokens`.
2. Save your access token locally to `access-token` file or a secure credentials store.
3. Create `module/provider.tf` with the following contents:

```tf
terraform {
  required_providers {
    supabase = {
      source  = "supabase/supabase"
      version = "~> 1.0"
    }
  }
}

provider "supabase" {
  access_token = file("${path.cwd}/access-token")
}
```

Run the command `terraform -chdir=module apply` to confirm that Terraform can find the provider.

## Creating a project

Supabase projects are represented as a TF resource called `supabase_project`.

Create a `module/resource.tf` file with the following contents.

```tf
# Create a project resource
resource "supabase_project" "production" {
  organization_id   = "<your-org-id>"
  name              = "tf-example"
  database_password = "<your-password>"
  region            = "ap-southeast-1"

  lifecycle {
    ignore_changes = [database_password]
  }
}
```

Remember to substitute placeholder values with your own. For sensitive fields such as the password, consider storing and retrieving them from a secure credentials store.

Next, run `terraform -chdir=module apply` to create the new project resource.

### Importing a project

If you have an existing project hosted on Supabase, you can import it into your local Terraform state for tracking and management.

Edit `module/resource.tf` with the following changes.

```tf
# Define a linked project variable as user input
variable "linked_project" {
  type = string
}

import {
  to = supabase_project.production
  id = var.linked_project
}

# Create a project resource
resource "supabase_project" "production" {
  organization_id   = "<your-org-id>"
  name              = "tf-example"
  database_password = "<your-password>"
  region            = "ap-southeast-1"

  lifecycle {
    ignore_changes = [database_password]
  }
}
```

Run `terraform -chdir=module apply`. Enter the ID of your Supabase project at the prompt. If your local TF state is empty, your project will be imported from remote rather than recreated.

Alternatively, you may use the `terraform import ...` command without editing the resource file, for example, `terraform import supabase_project.production abcdefghijklmnopqrst` will let Terraform import your existing project, where `abcdefghijklmnopqrst` is your project's `Reference ID`. After which, running `terraform plan -out plan.out` may help you verify that your infrastructure matches the configuration.

## Configuring a project

Use the `supabase_settings` resource to manage your project settings.

Create `module/settings.tf` with the following contents.

```tf
# Configure api settings for the linked project
resource "supabase_settings" "production" {
  project_ref = var.linked_project

  api = jsonencode({
    db_schema            = "public,storage,graphql_public"
    db_extra_search_path = "public,extensions"
    max_rows             = 1000
  })
}
```

Project settings don't exist on their own. They are created and destroyed together with their corresponding project resource referenced by the `project_ref` field. This means there is no difference between creating and updating `supabase_settings` resource. Deletion is always a no-op.

You can declare any subset of fields to be managed by your TF module. The Supabase provider always performs a partial update when you run `terraform -chdir=module apply`. The underlying API call is also idempotent, so it's safe to apply again if the local state is lost.

To see the full list of settings available, try importing the `supabase_settings` resource instead.

### Configuring branches

One of the most powerful features of TF is the ability to fan out configs to multiple resources. You can easily mirror the configurations of your production project to your branch databases using the `for_each` meta-argument.

Create a `module/branches.tf` file.

```tf
# Fetch all branches of a linked project
data "supabase_branch" "all" {
  parent_project_ref = var.linked_project
}

# Override settings for each preview branch
resource "supabase_settings" "branch" {
  for_each = { for b in data.supabase_branch.all.branches : b.project_ref => b }

  project_ref = each.key

  api = supabase_settings.production.api

  auth = jsonencode({
    site_url = "http://localhost:3001"
  })
}
```

When you run `terraform -chdir=module apply`, the provider will configure all branches associated with your `linked_project` to mirror the `api` settings of your production project.

In addition, the `auth.site_url` settings of your branches will be customised to a localhost URL for all branches. This allows your users to login via a separate domain for testing.

## Committing your changes

Finally, you can commit the entire `module` directory to Git for version control. This allows your CI runner to run `terraform apply` automatically on new config changes. Any command line variables can be passed to CI via `TF_VAR_*` environment variables.

## Resolving config drift

You can still change your configuration through the dashboard. However, making changes through both the dashboard and Terraform can introduce config drift. Resolve the drift by manually editing your `*.tf` files.
