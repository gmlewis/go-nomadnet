#!/bin/bash

# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Use of this source code is governed by the Reticulum License
# that can be found in the LICENSE file.

# run-all-tests.sh runs all unit tests and integration tests with timeouts.
# This is the top-level test script for the go-lxst project.

set -euo pipefail
set -x

RUN_ALL_TESTS_TIMEOUT_SECONDS="${RUN_ALL_TESTS_TIMEOUT_SECONDS:-30}"

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

time run_with_timeout ./scripts/test-all.sh 2>&1 | tee test-failures.log
time run_with_timeout ./scripts/test-integration.sh -short 2>&1 | tee short-test-failures.log
time run_with_timeout ./scripts/test-integration.sh 2>&1 | tee full-test-failures.log

echo "All tests completed."
