#!/usr/bin/env bash
set -euo pipefail

AGGREGATE_MIN=90.0
PER_PKG_MIN=75.0

INCLUDED=(
  "github.com/millsmillsymills/protonmail-mcp/cmd/protonmail-mcp"
  "github.com/millsmillsymills/protonmail-mcp/internal/server"
  "github.com/millsmillsymills/protonmail-mcp/internal/tools"
  "github.com/millsmillsymills/protonmail-mcp/internal/session"
  "github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
  "github.com/millsmillsymills/protonmail-mcp/internal/proterr"
  "github.com/millsmillsymills/protonmail-mcp/internal/log"
  "github.com/millsmillsymills/protonmail-mcp/internal/keychain"
)

profile="${1:-cov.out}"
fail=0

# Aggregate from go tool cover -func total: line.
aggregate=$(go tool cover -func="$profile" | awk '/^total:/ { gsub("%", "", $NF); print $NF }')
echo "aggregate: ${aggregate}%"

# Per-package: parse the coverprofile directly. Each non-mode line is:
#   <file>:<startLine>.<startCol>,<endLine>.<endCol> <numStmts> <count>
# A statement is covered when count > 0.
for pkg in "${INCLUDED[@]}"; do
    pct=$(awk -v p="$pkg/" '
        NR == 1 { next }
        $1 ~ p {
            stmts = $2
            count = $3
            total += stmts
            if (count + 0 > 0) covered += stmts
        }
        END {
            if (total == 0) print "0.0"; else printf("%.1f", (covered / total) * 100)
        }
    ' "$profile")
    printf "  %-70s %s%%\n" "$pkg" "$pct"
    awk -v a="$pct" -v b="$PER_PKG_MIN" 'BEGIN{ exit (a+0 < b+0) ? 1 : 0 }' || {
        echo "FAIL: $pkg below ${PER_PKG_MIN}% floor"
        fail=1
    }
done

awk -v a="$aggregate" -v b="$AGGREGATE_MIN" 'BEGIN{ exit (a+0 < b+0) ? 1 : 0 }' || {
    echo "FAIL: aggregate below ${AGGREGATE_MIN}%"
    fail=1
}

exit "$fail"
