default: help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

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

.PHONY: build
build: ## Build the provider
	go build .

.PHONY: install
install: ## Install the provider into your Go bin directory, for Terraform dev_overrides
	go install .

# Every test mocks the Management API, so TF_ACC=1 is safe and runs the
# acceptance tests that would otherwise be skipped.
.PHONY: test
test: ## Run all tests (pass extra flags with TESTARGS, e.g. TESTARGS="-run TestAccApiKeyResource -v")
	TF_ACC=1 go test ./... $(TESTARGS)

.PHONY: testacc
testacc: ## Run all tests verbosely
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: check
check: lint test generate ## Run everything to do before opening a pull request

.PHONY: generate-json
generate-json: ## Regenerate docs/schema.json

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

.PHONY: generate-docs
generate-docs: ## Regenerate the docs in docs/
	@go generate ./...

.PHONY: generate
generate: generate-docs generate-json ## Regenerate the docs and docs/schema.json
