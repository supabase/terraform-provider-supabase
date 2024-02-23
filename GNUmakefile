default: testacc

define MAINTF
terraform {
  required_providers {
    supabase = {
      source  = "supabase/supabase"
      version = "~> 1.0"
    }
  }
}
endef

export MAINTF

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

# Generate schema.json for documentation
.PHONY: generate-json
generate-json:
	@echo "Generating docs/schema.json"

	@mkdir temp

	@echo "Generating temporary build"
	@go build -o ./temp

	@echo "Create temporary terraform definition"
	@echo "$$MAINTF" > ./temp/main.tf

	@echo "Write terraform schema to JSON"
	@cd temp; terraform init && terraform providers schema -json > ../docs/schema.json

	@echo "Cleaning up"
	@rm -r temp
