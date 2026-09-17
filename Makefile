SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := check

GO ?= go
# Source-built analyzers must use the same Go version as the code they inspect.
GO_VERSION := $(shell $(GO) env GOVERSION)
GOLANGCI_VERSION := v2.13.2
ACTIONLINT_VERSION := v1.7.12
GOVULNCHECK_VERSION := v1.8.0
MARKDOWNLINT_VERSION := 0.23.2
GOMARKDOC_VERSION := v1.1.0

TOOL_DIR := $(CURDIR)/.bin
GOLANGCI := $(TOOL_DIR)/golangci-lint/$(GOLANGCI_VERSION)/golangci-lint
ACTIONLINT := $(TOOL_DIR)/actionlint/$(ACTIONLINT_VERSION)/$(GO_VERSION)/actionlint
GOMARKDOC := $(TOOL_DIR)/gomarkdoc/$(GOMARKDOC_VERSION)/$(GO_VERSION)/gomarkdoc
GOVULNCHECK := $(TOOL_DIR)/govulncheck/$(GOVULNCHECK_VERSION)/$(GO_VERSION)/govulncheck

.PHONY: docs check tools fmt fmt-check test build lint workflow-lint docs-check docs-lint vuln tidy-check

check: fmt-check test build lint workflow-lint docs-check docs-lint vuln tidy-check

tools: $(GOLANGCI) $(ACTIONLINT) $(GOVULNCHECK) $(GOMARKDOC)

$(GOLANGCI):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_VERSION)/install.sh | sh -s -- -b "$(dir $(GOLANGCI))" $(GOLANGCI_VERSION)

$(ACTIONLINT):
	GOBIN="$(dir $(ACTIONLINT))" $(GO) install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

$(GOVULNCHECK):
	GOBIN="$(dir $(GOVULNCHECK))" $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(GOMARKDOC):
	GOBIN="$(dir $(GOMARKDOC))" $(GO) install github.com/princjef/gomarkdoc/cmd/gomarkdoc@$(GOMARKDOC_VERSION)

fmt: $(GOLANGCI)
	"$(GOLANGCI)" fmt

fmt-check: $(GOLANGCI)
	"$(GOLANGCI)" fmt --diff

test:
	$(GO) test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

build:
	$(GO) build ./...

lint: $(GOLANGCI)
	"$(GOLANGCI)" config verify
	"$(GOLANGCI)" run ./...

workflow-lint: $(ACTIONLINT)
	"$(ACTIONLINT)"

docs: $(GOMARKDOC)
	"$(GOMARKDOC)" --config .gomarkdoc.yml .

docs-check: $(GOMARKDOC)
	"$(GOMARKDOC)" --config .gomarkdoc.yml --check .

docs-lint:
	npx --yes markdownlint-cli2@$(MARKDOWNLINT_VERSION)

vuln: $(GOVULNCHECK)
	"$(GOVULNCHECK)" ./...

tidy-check:
	$(GO) mod tidy -diff
