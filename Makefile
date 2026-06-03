GO ?= go
COVER_PKGS := github.com/millsymills-com/protonmail-mcp/cmd/protonmail-mcp,github.com/millsymills-com/protonmail-mcp/internal/server,github.com/millsymills-com/protonmail-mcp/internal/tools,github.com/millsymills-com/protonmail-mcp/internal/session,github.com/millsymills-com/protonmail-mcp/internal/protonraw,github.com/millsymills-com/protonmail-mcp/internal/proterr,github.com/millsymills-com/protonmail-mcp/internal/log,github.com/millsymills-com/protonmail-mcp/internal/keychain,github.com/millsymills-com/protonmail-mcp/internal/keyring,github.com/millsymills-com/protonmail-mcp/internal/credfile

.PHONY: test test-race coverage coverage-check verify-cassettes record

test:
	$(GO) test ./...

test-race:
	$(GO) test ./... -race

coverage:
	$(GO) test ./... -race -coverprofile=cov.out -coverpkg=$(COVER_PKGS)

coverage-check: coverage
	./scripts/coverage-check.sh cov.out

verify-cassettes:
	$(GO) run ./cmd/testvcr-lint

record:
ifndef SCENARIO
	$(error SCENARIO is required, e.g. make record SCENARIO=whoami_happy)
endif
	$(GO) run -tags recording ./cmd/record-cassettes '$(SCENARIO)'
