GO ?= go
COVER_PKGS := $(shell paste -sd, cover-pkgs.txt)

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
