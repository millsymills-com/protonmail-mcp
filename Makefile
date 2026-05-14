GO ?= go
COVER_PKGS := github.com/millsmillsymills/protonmail-mcp/cmd/protonmail-mcp,github.com/millsmillsymills/protonmail-mcp/internal/server,github.com/millsmillsymills/protonmail-mcp/internal/tools,github.com/millsmillsymills/protonmail-mcp/internal/session,github.com/millsmillsymills/protonmail-mcp/internal/protonraw,github.com/millsmillsymills/protonmail-mcp/internal/proterr,github.com/millsmillsymills/protonmail-mcp/internal/log,github.com/millsmillsymills/protonmail-mcp/internal/keychain

.PHONY: test test-race coverage coverage-check verify-cassettes record

test:
	$(GO) test ./...

test-race:
	$(GO) test ./... -race

coverage:
	$(GO) test ./... -coverprofile=cov.out -coverpkg=$(COVER_PKGS)

coverage-check: coverage
	./scripts/coverage-check.sh cov.out

verify-cassettes:
	$(GO) run ./cmd/testvcr-lint

record:
ifndef SCENARIO
	$(error SCENARIO is required, e.g. make record SCENARIO=whoami_happy)
endif
	$(GO) run -tags recording ./cmd/record-cassettes '$(SCENARIO)'
