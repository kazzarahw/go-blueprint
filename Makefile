# go-blueprint — development and verification tasks.
#
# Development tools are pinned in go.mod via `tool` directives and run with
# `go tool <name>`. No separate installation step is needed, and Go itself
# guarantees that CI and local runs use the same tool versions.
#
# `make verify` is the single source of truth for "is this change good".

.DEFAULT_GOAL := help
SHELL := bash
GO := go
FUZZTIME ?= 20s

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "; printf "Usage: make \033[36m<target>\033[0m [VAR=value]\n\n"} /^[a-zA-Z_.-]+:.*?## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2} /^##@ / {printf "\n\033[1m%s\033[0m\n", substr($$0, 5)}' $(MAKEFILE_LIST)

##@ Setup

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

.PHONY: tidy-check
tidy-check: ## Fail if go.mod/go.sum are not tidy
	$(GO) mod tidy -diff

##@ Formatting

.PHONY: fmt
fmt: ## Format all Go source files (goimports with local prefix + gofmt)
	$(GO) tool golangci-lint fmt ./...

.PHONY: fmt-check
fmt-check: ## Fail if any file is not gofmt/goimports clean
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "not gofmt-clean:"; echo "$$out"; exit 1; fi
	@out=$$($(GO) tool goimports -local github.com/kazzarahw/go-blueprint -l .); if [ -n "$$out" ]; then echo "not goimports-clean:"; echo "$$out"; exit 1; fi

##@ Analysis

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: staticcheck
staticcheck: ## Run staticcheck (SA, S, ST, QF, U checks)
	$(GO) tool staticcheck ./...

.PHONY: lint
lint: ## Run golangci-lint (shadow vet, staticcheck, exhaustive, gosec, gocritic, gocognit, revive, ...)
	$(GO) tool golangci-lint run ./...

.PHONY: exhaustive
exhaustive: ## Require switches over enum types to be exhaustive
	$(GO) tool exhaustive ./...

.PHONY: nilaway
nilaway: ## Run NilAway static nil-flow analysis
	$(GO) tool nilaway ./...

.PHONY: vuln
vuln: ## Check for known vulnerabilities in dependencies (requires network)
	$(GO) tool govulncheck ./...

##@ Tests

.PHONY: test
test: ## Run tests with the race detector
	$(GO) test -race -count=1 ./...

.PHONY: fuzz
fuzz: ## Fuzz all fuzz targets (one package per run; override with FUZZTIME=1m)
	@for pkg in $$($(GO) list ./...); do \
		if $(GO) test -list '^Fuzz' $$pkg 2>/dev/null | grep -q '^Fuzz'; then \
			echo "fuzzing $$pkg"; \
			$(GO) test -run='^$$' -fuzz=. -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
		fi; \
	done

.PHONY: cover
cover: ## Run tests with coverage and print per-function coverage
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

##@ Build

.PHONY: build
build: ## Build all packages
	$(GO) build ./...

##@ Verification

.PHONY: verify
verify: tidy-check fmt-check vet staticcheck lint exhaustive nilaway test ## Run the complete verification gate (what CI enforces)

.PHONY: clean
clean: ## Remove local artifacts (coverage.out)
	rm -f coverage.out

##@ Template

.PHONY: smoke
smoke: ## Copy the template into a temp dir, rename the module path, and run make verify there
	$(GO) run ./scripts/smoke
