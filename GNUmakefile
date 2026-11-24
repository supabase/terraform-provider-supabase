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
	TF_ACC=1 go test ./internal/provider/... -v -parallel 7 $(TESTARGS) -timeout 120m

# Unlike the other test targets, this talks to the real Management API and
# permanently deletes projects whose names start with "acc-test". The
# -sweep flag needs a value, and "all" is a placeholder because the API is global.
.PHONY: sweep
sweep: ## Delete leftover acceptance test resources from your account (needs SUPABASE_ACCESS_TOKEN)
	@test -n "$$SUPABASE_ACCESS_TOKEN" || (echo "SUPABASE_ACCESS_TOKEN is required, for example: SUPABASE_ACCESS_TOKEN=... make sweep"; exit 1)
	@echo "WARNING: this is destructive. It permanently deletes projects named acc-test* from the account for this access token."
	go test ./internal/provider -v -sweep=all $(SWEEPARGS) -timeout 60m

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

.PHONY: changelog
changelog: ## Add a changelog entry for the current change (interactive)
	@go tool -modfile=tools/go.mod changie new

.PHONY: changelog-release
changelog-release: ## Fold unreleased entries into CHANGELOG.md, VERSION=<x.y.z|major|minor|patch>
	@test -n "$(VERSION)" || (echo "VERSION is required, for example: make changelog-release VERSION=1.2.3"; exit 1)
	@go tool -modfile=tools/go.mod changie batch $(VERSION)
	@go tool -modfile=tools/go.mod changie merge
