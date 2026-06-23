GO ?= go
COVER_PKGS := $(shell paste -sd, cover-pkgs.txt)

.PHONY: test test-race coverage coverage-check verify-cassettes record pii-hash

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

# Hash a known-PII literal for internal/testvcr/pii-denylist.txt. Input is read
# hidden from a prompt (never an argv/env, so it stays out of shell history and
# the process table) and normalized to match denylistHits: lowercased, internal
# whitespace collapsed to single spaces, trimmed. Paste the digest into the
# denylist with a non-identifying label.
pii-hash:
	@read -rs -p 'PII literal (hidden): ' raw; echo >&2; \
	norm=$$(printf '%s' "$$raw" | tr '[:upper:]' '[:lower:]' | tr -s '[:space:]' ' '); \
	norm=$${norm# }; norm=$${norm% }; \
	printf '%s' "$$norm" | shasum -a 256 | cut -d' ' -f1

# Set RECORD_EMAIL / RECORD_PASSWORD / RECORD_TOTP_SECRET in the environment
# first. Single-quote credentials or set them with `read -rs` so the shell does
# not interpolate `$`, backticks, or `!` into the secret: an unquoted
# `export RECORD_PASSWORD=...` silently truncates anything after a `$`, and the
# recorder would otherwise authenticate with a mangled credential.
record:
ifndef SCENARIO
	$(error SCENARIO is required, e.g. make record SCENARIO=whoami_happy)
endif
	$(GO) run -tags recording ./cmd/record-cassettes '$(SCENARIO)'
