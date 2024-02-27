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

define TERRAFORMRC
provider_installation {
  dev_overrides {
	"supabase/supabase" = "$$PWD"
  }
}
endef

export MAINTF
export TERRAFORMRC

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

# Generate schema.json for documentation
.PHONY: generate-json
generate-json:
	@echo "Generating docs/schema.json"

	@mkdir temp

	@echo "  - Generating temporary build"
	@go build -o ./temp

	@echo "  - Creating temporary terraform config and definition"
	@echo "$$TERRAFORMRC" > ./temp/local.tfrc.tpl
	@echo "$$MAINTF" > ./temp/main.tf
	@cd temp; envsubst '$${PWD}' < local.tfrc.tpl > local.tfrc

	@echo "  - Writing terraform schema to JSON"
	@cd temp; export TF_CLI_CONFIG_FILE="$$PWD/local.tfrc" && \
		terraform providers schema -json > schema.json && \
		jq . schema.json > ../docs/schema.json	

	@echo "Cleaning up"
	@rm -r temp
