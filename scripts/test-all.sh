#!/bin/bash -e
# -*- compile-command: "./test-all.sh"; -*-

# Copyright 2026 Glenn Lewis. All rights reserved.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.

# test-all.sh runs all unit tests with race detection and static analysis.
# It runs in -short mode so every test stays well under 5 seconds: any test that
# cannot meet that budget (a full-package type check, a live network round-trip,
# a cross-process tmux harness, ...) calls testutils.SkipShortIntegration at
# its top and is skipped here. Those tests still run in the full integration
# suite (scripts/test-integration.sh), which omits -short and allows 4m.

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
REPO_ROOT="${SCRIPT_DIR}/.."

GO_TEST_TIMEOUT="${GO_TEST_TIMEOUT:-2m}"

cd "${REPO_ROOT}"

echo "Running gofmt..."
gofmt -s -w .

echo "Running unit tests with race detector (-short, <5s per test)..."
go test -race -count=1 -short --timeout "${GO_TEST_TIMEOUT}" "$@" ./...

# The wago-tagged build (sandboxed .wasm pages) is invisible to the default
# ./... run, so the sandboxed packages are tested explicitly with the tag on
# supported host platforms; other platforms keep the stub.
GOOS_CHECK="$(go env GOOS)"
GOARCH_CHECK="$(go env GOARCH)"
if [[ "${GOOS_CHECK}" =~ ^(linux|darwin|windows)$ && "${GOARCH_CHECK}" =~ ^(amd64|arm64)$ ]]; then
	echo "Running unit tests with -tags=wago (-short, sandboxed .wasm pages)..."
	go test -tags=wago -race -count=1 -short --timeout "${GO_TEST_TIMEOUT}" ./nomadnet/wasmpages ./nomadnet/node ./nomadnet/browser
fi

echo "Running go vet..."
go vet ./...

echo "Done."
