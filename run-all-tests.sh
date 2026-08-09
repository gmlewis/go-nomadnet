#!/bin/bash

# Copyright 2026 Glenn Lewis. All rights reserved.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.

# run-all-tests.sh runs all unit tests and integration tests with timeouts.

set -euo pipefail
set -x

RUN_ALL_TESTS_TIMEOUT_SECONDS="${RUN_ALL_TESTS_TIMEOUT_SECONDS:-60}"

run_with_timeout() {
	if command -v timeout >/dev/null 2>&1; then
		timeout --foreground "${RUN_ALL_TESTS_TIMEOUT_SECONDS}s" "$@"
		return
	fi
	if command -v gtimeout >/dev/null 2>&1; then
		gtimeout --foreground "${RUN_ALL_TESTS_TIMEOUT_SECONDS}s" "$@"
		return
	fi

	python3 - "${RUN_ALL_TESTS_TIMEOUT_SECONDS}" "$@" <<'PY'
import subprocess
import sys

timeout = int(sys.argv[1])
cmd = sys.argv[2:]

try:
    raise SystemExit(subprocess.run(cmd, check=False, timeout=timeout).returncode)
except subprocess.TimeoutExpired:
    print(f"timed out after {timeout}s: {' '.join(cmd)}", file=sys.stderr)
    raise SystemExit(124)
PY
}

# test-all.sh is redundant when the short integration tests are running next, so skip it:
# time run_with_timeout ./scripts/test-all.sh 2>&1 | tee test-failures.log

time run_with_timeout ./scripts/test-integration.sh -short 2>&1 | tee short-test-failures.log
time run_with_timeout ./scripts/test-integration.sh 2>&1 | tee full-test-failures.log

echo "All tests completed."

# ---------------------------------------------------------------------------
# Static cleanliness checks: verify the repo is "squeaky-clean" per gopls.
#
# 1. gopls check  — workspace diagnostics (compiler errors, vet-style
#    warnings). gopls check always exits 0 and prints diagnostics to stdout,
#    so cleanliness is judged by whether it produced any output. It takes
#    filenames (not package patterns), so every tracked .go file is
#    enumerated.
#
# 2. modernize    — the standalone modernize analyzer, the headless
#    equivalent of the editor's "gopls modernize" code actions. (There is no
#    `gopls modernize` CLI subcommand; the analyzer is invoked via
#    `go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest`.)
#    Without -fix it only reports; it exits non-zero when suggestions exist.
#    Requires network on first run to fetch the analyzer module.
# ---------------------------------------------------------------------------

echo "Running gopls check (workspace diagnostics)..."
GOPLS_CHECK_LOG="gopls-check.log"
: > "${GOPLS_CHECK_LOG}"
# xargs may split the file list into batches; append each batch's output.
git ls-files -z '*.go' | xargs -0 gopls check >>"${GOPLS_CHECK_LOG}" 2>&1 || true
if [[ -s "${GOPLS_CHECK_LOG}" ]]; then
    echo "FAIL: gopls check reported diagnostics (see ${GOPLS_CHECK_LOG}):" >&2
    cat "${GOPLS_CHECK_LOG}" >&2
    exit 1
fi
echo "gopls check: clean (no diagnostics)"

echo "Running modernize (modernization suggestions)..."
MODERNIZE_LOG="modernize.log"
if ! go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest ./... >"${MODERNIZE_LOG}" 2>&1; then
    echo "FAIL: modernize reported suggestions (see ${MODERNIZE_LOG}):" >&2
    cat "${MODERNIZE_LOG}" >&2
    exit 1
fi
echo "modernize: clean (no suggestions)"

echo "Repo is squeaky-clean (gopls check + modernize)."
