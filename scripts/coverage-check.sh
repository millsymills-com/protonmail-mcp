#!/usr/bin/env bash
set -euo pipefail

AGGREGATE_MIN=90.0
PER_PKG_MIN=75.0

INCLUDED=()
while IFS= read -r pkg; do
  [[ -n "$pkg" ]] && INCLUDED+=("$pkg")
done < "$(dirname "$0")/../cover-pkgs.txt"

profile="${1:-cov.out}"
fail=0

# go test ./... -coverpkg=... writes a separate coverage block per test
# package, so the same source line can appear many times in cov.out (once
# per dependent test package). go tool cover -func handles this internally
# by merging blocks set-wise, but our per-package awk pass would otherwise
# double-count. Pre-merge into a temporary profile keyed by block.
merged="$(mktemp -t cov-merged.XXXXXX)"
trap 'rm -f "$merged"' EXIT
awk 'NR == 1 { print; next }
{
    key = $1 " " $2
    if ($3 + 0 > 0) covered[key] = 1
    lines[key] = $0
}
END {
    for (k in lines) {
        n = split(lines[k], parts, " ")
        parts[n] = (covered[k] ? 1 : 0)
        out = parts[1]
        for (i = 2; i <= n; i++) out = out " " parts[i]
        print out
    }
}' "$profile" > "$merged"

# Aggregate from go tool cover -func total: line.
aggregate=$(go tool cover -func="$profile" | awk '/^total:/ { gsub("%", "", $NF); print $NF }')
echo "aggregate: ${aggregate}%"

# Per-package: parse the merged coverprofile directly. Each non-mode line is:
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
    ' "$merged")
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
