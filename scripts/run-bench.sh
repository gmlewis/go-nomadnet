#!/usr/bin/env bash
# run-bench.sh — run the gonomadnet performance benchmark suite and capture a
# benchstat-friendly baseline. Benchmarks are OPT-IN: the normal test scripts
# (test-all.sh / test-integration.sh) never pass -bench, so CI and the -short
# unit run pay zero cost; this script is the only thing that runs them.
#
# Usage:
#   scripts/run-bench.sh                     # all benchmarks, count=5, 1s each
#   BENCH_COUNT=10 scripts/run-bench.sh      # more iterations for tighter stats
#   BENCH_TIME=3s scripts/run-bench.sh       # longer per-benchmark runtime
#   BENCH_PATTERN='ScrollBar|Wheel' scripts/run-bench.sh   # filter by name
#   BENCH_PKGS='./tui/... ./nomadnet/micron/...' scripts/run-bench.sh
#   SAVE=baseline.txt scripts/run-bench.sh  # also write a saved baseline file
#
# Compare before/after a fix:
#   scripts/run-bench.sh SAVE=before.txt   # before the change
#   <make the change>
#   scripts/run-bench.sh SAVE=after.txt
#   benchstat before.txt after.txt
#
# Live real-terminal profiling (the headless bench cannot reproduce terminal
# flush/cursor costs — use this for the visible flicker/scroll):
#   gonomadnet -textui -pprof-addr 127.0.0.1:6060
#   go tool pprof -http :8080 http://127.0.0.1:6060/debug/pprof/profile?seconds=30
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

BENCH_COUNT="${BENCH_COUNT:-5}"
BENCH_TIME="${BENCH_TIME:-1s}"
BENCH_PATTERN="${BENCH_PATTERN:-.}"
BENCH_PKGS="${BENCH_PKGS:-./tui/... ./nomadnet/micron/...}"
GO_TEST_TIMEOUT="${GO_TEST_TIMEOUT:-10m}"

# GOCACHE=/tmp/go-cache matches the convention in tooling/sweep.sh:68.
export GOCACHE=/tmp/go-cache

# shellcheck disable=SC2086  # BENCH_PKGS is meant to word-split into packages
GOCACHE=/tmp/go-cache go test \
  -run='^$' \
  -bench="$BENCH_PATTERN" \
  -benchmem \
  -count="$BENCH_COUNT" \
  -benchtime="$BENCH_TIME" \
  -timeout "$GO_TEST_TIMEOUT" \
  $BENCH_PKGS 2>&1 | tee /tmp/gonomadnet-bench-latest.txt

if [ -n "${SAVE:-}" ]; then
  cp /tmp/gonomadnet-bench-latest.txt "$SAVE"
  echo "wrote $SAVE"
fi