# Supabase Terraform Provider

The [Supabase Provider](https://registry.terraform.io/providers/supabase/supabase/latest/docs) allows Terraform to manage resources hosted on [Supabase](https://supabase.com/) platform.

You may use this provider to version control your project settings or setup CI/CD pipelines for automatically provisioning projects and branches.

- [CI/CD example](https://github.com/supabase/supabase-action-example/tree/main/supabase/remotes)
- [Step-by-step tutorials](docs/tutorial.md)
- [Contributing guide](CONTRIBUTING.md)

## Using the provider

This simple example imports an existing Supabase project and synchronises its API settings.

```hcl
terraform {
  required_providers {
    supabase = {
      source  = "supabase/supabase"
      version = "~> 1.0"
    }
  }
}

provider "supabase" {
  access_token = file("${path.module}/access-token")
}

# Define a linked project variable as user input
variable "linked_project" {
  type = string
}

# Import the linked project resource
import {
  to = supabase_project.production
  id = var.linked_project
}

resource "supabase_project" "production" {
  organization_id   = "nknnyrtlhxudbsbuazsu"
  name              = "tf-project"
  database_password = "tf-example"
  region            = "ap-southeast-1"

  lifecycle {
    ignore_changes = [database_password]
  }
}

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
