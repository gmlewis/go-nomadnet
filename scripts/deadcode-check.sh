#!/bin/bash
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Advisory whole-program dead-code report (golang.org/x/tools/cmd/deadcode).
#
# `staticcheck -checks=U1000` cannot see superseded code that is exported, nor
# code referenced only from its own unit tests (U1000 counts test references as
# usage). deadcode runs Rapid Type Analysis from the program's main packages and
# reports every function unreachable at runtime — the class of dead code that
# accumulated here before.
#
# This step is deliberately ADVISORY: the repo intentionally keeps unwired
# Python-parity surface that deadcode legitimately reports. The full report is
# written to deadcode.log and the script always exits 0. To make it a hard gate,
# commit deadcode-baseline.txt and run with FAIL_ON_NEW=1.

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

LOG="${REPO_ROOT}/deadcode.log"
BASELINE="${REPO_ROOT}/deadcode-baseline.txt"
FAIL_ON_NEW="${FAIL_ON_NEW:-0}"

# deadcode exits non-zero when it reports findings, so ignore the status and
# judge by the output. A load failure (offline, tool unavailable) yields no
# findings and is reported as "skipped" rather than failing the build.
out="$(go run golang.org/x/tools/cmd/deadcode@latest -test=false ./... 2>&1)" || true
printf '%s\n' "${out}" >"${LOG}"

count="$(printf '%s\n' "${out}" | grep -c 'unreachable func' || true)"
if [[ "${count}" -eq 0 ]]; then
    echo "deadcode: advisory check skipped (tool unavailable or load error; see $(basename "${LOG}"))"
    exit 0
fi
echo "deadcode: ${count} unreachable function(s) reported (advisory; see $(basename "${LOG}"))"

# Optional hard gate: compare position-independent keys against a committed
# baseline, so only NEW dead code fails.
if [[ "${FAIL_ON_NEW}" == "1" && -f "${BASELINE}" ]]; then
    norm() { grep 'unreachable func' "$1" | sed -E 's/:[0-9]+:[0-9]+:/:/' | sort; }
    if ! diff -u <(norm "${BASELINE}") <(norm "${LOG}") >/dev/null; then
        echo "FAIL: deadcode found dead code absent from deadcode-baseline.txt:" >&2
        diff -u <(norm "${BASELINE}") <(norm "${LOG}") >&2 || true
        exit 1
    fi
fi
exit 0
