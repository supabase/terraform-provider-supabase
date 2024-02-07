# Using the Supabase Terraform Provider

## Setting up a TF module

1. Create a Personal Access Token from Supabase Dashboard.
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

Run the command `terraform -chdir=module apply` which should succeed in finding the provider.

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

Remember to substitue placeholder values with your own. For sensitive fields like password, you may consider retrieving it from a secure credentials store.

Next, run `terraform -chdir=module apply` and confirm creating the new project resource.

### Importing a project

If you have an existing project hosted on Supabase, you may import it into your local terraform state for tracking and management.

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

Run `terraform -chdir=module apply` and you will be prompted to enter the reference ID of an existing Supabase project. If your local TF state is empty, your project will be imported from remote rather than recreated.

Alternatively, you may use the `terraform import ...` command without editing the resource file.

## Configuring a project

Keeping your project settings in-sync is easy with the `supabase_settings` resource.

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

Project settings don't exist on their own. They are created and destroyed together with their corresponding project resource referenced by the `project_ref` field. This means there is no difference between creating and updating `supabase_settings` resource while deletion is always a no-op.

You may declare any subset of fields to be managed by your TF module. The Supabase provider always performs a partial update when you run `terraform -chdir=module apply`. The underlying API call is also idempotent so it's safe to apply again if the local state is lost.

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

Finally, you may commit the entire `module` directory to git for version control. This allows your CI runner to run `terraform apply` automatically on new config changes. Any command line variables can be passed to CI via `TF_VAR_*` environment variables instead.

## Resolving config drift

Tracking your configs in TF module does not mean that you lose the ability to change configs through the dashboard. However, doing so could introduce config drift that you need to resolve manually by adding them to your `*.tf` files.
