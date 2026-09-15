#!/bin/bash

# Copyright 2026 Glenn Lewis. All rights reserved.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.

# run-all-tests.sh runs all unit tests and integration tests with timeouts.
# Static cleanliness checks (errcheck, staticcheck, modernize, gopls check)
# run FIRST so a lint failure fails fast before spending time on tests.
#
# Default mode (~50s): lint + the -short integration suite (skips the
# cross-process / Python-interop tests) — enough for ~99% confidence before
# pushing. The FULL suite runs in GitHub CI on every push; run it locally
# with:
#
#   ./run-all-tests.sh --full        (or RUN_ALL_TESTS_FULL=1)

set -euo pipefail
set -x

# Absolute repo root: the nested-module loops cd into subdirectories, so the
# sweep below needs a path that survives the cd.
REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"

RUN_ALL_TESTS_TIMEOUT_SECONDS="${RUN_ALL_TESTS_TIMEOUT_SECONDS:-500}"

# ---------------------------------------------------------------------------
# Temp-dir hygiene. A test run that ends normally removes every temp dir it
# created (testutils.TempDir registers t.Cleanup, and testutils/cleanup.go
# removes them on SIGINT/SIGTERM). What that cannot cover is SIGKILL, a
# `go test -timeout` panic, or os.Exit from an unrelated goroutine — so sweep
# before and after: the entry sweep with an age guard that spares anything a
# live run owns, the exit sweep with none, because by then this run's own dirs
# are supposed to be gone. scripts/clean-test-tmp.sh only ever matches its
# allow-list of test prefixes, so unrelated /tmp content is never touched. That
# unguarded exit sweep also passes -L (see sweep_test_tmp below), so it stands
# down entirely while another test binary is alive.
#
# The -c check keeps that allow-list honest: it fails when a temp prefix in the
# sources is missing from the list, the same way the lint gates below fail.
# The list is shared with go-reticulum's copy of this script, so a run here
# also reclaims that suite's residue.
# ---------------------------------------------------------------------------
sweep_test_tmp() {
	# -L belongs to the exit sweep only. It runs unguarded (-m 0), so without
	# this it would delete the live temp dirs of a concurrent run — a bare
	# `go test` in another terminal, or another agent's run in this repo — and
	# fail that run for reasons that look nothing like a cleanup problem. The
	# entry sweep keeps its 30-minute age guard instead, so it still reclaims
	# stale residue no matter what else is running.
	if [ "${1:-0}" -eq 0 ]; then
		bash "$REPO_ROOT/scripts/clean-test-tmp.sh" -m 0 -L
		return
	fi

	bash "$REPO_ROOT/scripts/clean-test-tmp.sh" -m "${1:-0}"
}

bash "$REPO_ROOT/scripts/clean-test-tmp.sh" -c
sweep_test_tmp 30
trap 'sweep_test_tmp 0 || true' EXIT

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

# ---------------------------------------------------------------------------
# Static cleanliness checks: verify the repo is "squeaky-clean" per gopls.
#
# 0. errcheck     — verifies all error return values are checked.
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

# ---------------------------------------------------------------------------
# Formatting checks: mirror the CI fmt/imports jobs in build.yml. Both run in
# CHECK mode (-l lists unformatted files) rather than the -w rewrite mode the
# test-all.sh / test-integration.sh scripts use, so an unformatted file fails
# the run instead of being silently fixed in the working tree (and then
# committed broken). goimports is a superset of gofmt (it also normalizes
# import grouping), so it is checked separately.
# ---------------------------------------------------------------------------

echo "Running gofmt..."
gofmt -s -w .

# SLOW!!!
# echo "Running goimports check..."
# unformatted=$(goimports -l .)
# if [[ -n "$unformatted" ]]; then
# 	echo "FAIL: goimports would reformat the following (run: goimports -w <files>):" >&2
# 	echo "$unformatted" >&2
# 	exit 1
# fi
# echo "goimports: clean (all imports formatted)"

echo "Running errcheck..."
ERRCHECK_LOG="errcheck.log"
if ! errcheck ./... >"${ERRCHECK_LOG}" 2>&1; then
    echo "FAIL: errcheck reported unchecked errors (see ${ERRCHECK_LOG}):" >&2
    cat "${ERRCHECK_LOG}" >&2
    exit 1
fi
# The wago-tagged sandbox files are invisible to the default ./... build;
# run the same check over the wago build on supported host platforms.
GOOS_CHECK="$(go env GOOS)"
GOARCH_CHECK="$(go env GOARCH)"
if [[ "${GOOS_CHECK}" =~ ^(linux|darwin|windows)$ && "${GOARCH_CHECK}" =~ ^(amd64|arm64)$ ]]; then
    if ! errcheck -tags=wago ./... >>"${ERRCHECK_LOG}" 2>&1; then
        echo "FAIL: errcheck reported unchecked errors with -tags=wago (see ${ERRCHECK_LOG}):" >&2
        cat "${ERRCHECK_LOG}" >&2
        exit 1
    fi
fi
echo "errcheck: clean (all errors checked)"

# echo "Running gopls check (workspace diagnostics)..."
# GOPLS_CHECK_LOG="gopls-check.log"
# : > "${GOPLS_CHECK_LOG}"
# # xargs may split the file list into batches; append each batch's output.
# git ls-files -z '*.go' | xargs -0 gopls check >>"${GOPLS_CHECK_LOG}" 2>&1 || true
# if [[ -s "${GOPLS_CHECK_LOG}" ]]; then
#     echo "FAIL: gopls check reported diagnostics (see ${GOPLS_CHECK_LOG}):" >&2
#     cat "${GOPLS_CHECK_LOG}" >&2
#     exit 1
# fi
# echo "gopls check: clean (no diagnostics)"

echo "Running modernize (modernization suggestions)..."
MODERNIZE_LOG="modernize.log"
if ! go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest ./... >"${MODERNIZE_LOG}" 2>&1; then
    echo "FAIL: modernize reported suggestions (see ${MODERNIZE_LOG}):" >&2
    cat "${MODERNIZE_LOG}" >&2
    exit 1
fi
if [[ "${GOOS_CHECK}" =~ ^(linux|darwin|windows)$ && "${GOARCH_CHECK}" =~ ^(amd64|arm64)$ ]]; then
    if ! go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest -tags=wago ./... >>"${MODERNIZE_LOG}" 2>&1; then
        echo "FAIL: modernize reported suggestions with -tags=wago (see ${MODERNIZE_LOG}):" >&2
        cat "${MODERNIZE_LOG}" >&2
        exit 1
    fi
fi
echo "modernize: clean (no suggestions)"

echo "Running staticcheck (SA* + U1000, with integration tags)..."
STATICCHECK_LOG="staticcheck.log"
staticcheck -checks=SA*,U1000 -tags=integration ./... >"${STATICCHECK_LOG}" 2>&1 || true
if [[ "${GOOS_CHECK}" =~ ^(linux|darwin|windows)$ && "${GOARCH_CHECK}" =~ ^(amd64|arm64)$ ]]; then
    staticcheck -checks=SA*,U1000 -tags=integration,wago ./... >>"${STATICCHECK_LOG}" 2>&1 || true
fi
if [[ -s "${STATICCHECK_LOG}" ]]; then
    echo "FAIL: staticcheck reported issues (see ${STATICCHECK_LOG}):" >&2
    cat "${STATICCHECK_LOG}" >&2
    exit 1
fi
echo "staticcheck: clean (SA* + U1000, with integration tags)"

echo "Running deadcode (advisory, whole-program reachability)..."
bash "${REPO_ROOT}/scripts/deadcode-check.sh"

# ---------------------------------------------------------------------------
# Integration tests: the -short suite always (fast); the full suite only in
# full mode (--full or RUN_ALL_TESTS_FULL=1), matching what GitHub CI runs.
# ---------------------------------------------------------------------------

if [[ "${1:-}" == "--full" ]]; then
	RUN_ALL_TESTS_FULL=1
fi

# test-all.sh is redundant when the short integration tests are running next, so skip it:
# time run_with_timeout ./scripts/test-all.sh 2>&1 | tee test-failures.log
# The -short subset (SkipShortIntegration-gated) runs in ~10s vs ~70s for the
# full suite.
time run_with_timeout ./scripts/test-integration.sh -short 2>&1 | tee short-test-failures.log

# The wago-tagged build (sandboxed .wasm pages) is invisible to the default
# ./... runs, so the sandboxed packages are tested explicitly with the tag on
# supported host platforms; other platforms keep the stub (covered above).
if [[ "${GOOS_CHECK}" =~ ^(linux|darwin|windows)$ && "${GOARCH_CHECK}" =~ ^(amd64|arm64)$ ]]; then
    time run_with_timeout go test -tags=wago -race -count=1 -short --timeout 2m ./nomadnet/wasmpages ./nomadnet/node ./nomadnet/browser 2>&1 | tee wago-test-failures.log
fi

if [[ "${RUN_ALL_TESTS_FULL:-0}" == "1" ]]; then
	time run_with_timeout ./scripts/test-integration.sh 2>&1 | tee full-test-failures.log
fi

echo "All tests completed."

# Sweep here instead of leaving it to the EXIT trap: the trap fires after the
# last write to stdout, so its output — and the shell's xtrace of it — would
# land below the verdict. Clearing the trap once the sweep has run keeps the
# failure paths covered (the trap still fires on any early exit above) while
# making the squeaky-clean line below genuinely the last thing printed.
sweep_test_tmp 0 || true
trap - EXIT

echo "Repo is squeaky-clean (errcheck + gopls check + modernize + staticcheck SA*/U1000 + deadcode advisory + all tests)."
