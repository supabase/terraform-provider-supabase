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

# Generate terraform docs
.PHONY: generate-docs
generate-docs:
	@go generate ./...

# Runs all generate targets
.PHONY: generate
generate: generate-docs generate-json

SUPABASE_API_URL ?= https://api.supabase.com
API_SPEC_V1 ?= $(SUPABASE_API_URL)/api/v1-yaml
API_SPEC_V2 ?= $(SUPABASE_API_URL)/api/v2-yaml

# Refresh the committed Management API OpenAPI snapshots and regenerate the client.
#   make update-api-spec                                                  production
#   make update-api-spec SUPABASE_API_URL=https://api.supabase.green      staging
#   make update-api-spec SUPABASE_API_URL=http://127.0.0.1:8080           a branch of the API running locally
#   make update-api-spec API_SPEC_V1=file:///path/v1.yaml API_SPEC_V2=file:///path/v2.yaml
.PHONY: update-api-spec
update-api-spec:
	curl -fsSL "$(API_SPEC_V1)" -o internal/openapi/v1-openapi.yaml
	curl -fsSL "$(API_SPEC_V2)" -o internal/openapi/v2-openapi.yaml
	go generate ./internal/openapi/...
