#!/bin/bash -e
# -*- compile-command: "./test-all.sh"; -*-

# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Use of this source code is governed by the Reticulum License
# that can be found in the LICENSE file.

# test-all.sh runs all unit tests with race detection and static analysis.

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
REPO_ROOT="${SCRIPT_DIR}/.."

ERRCHECK_BIN="$(command -v errcheck || true)"
if [[ -z "${ERRCHECK_BIN}" ]]; then
	go install github.com/kisielk/errcheck@latest
	ERRCHECK_BIN="$(go env GOPATH)/bin/errcheck"
fi

GOIMPORTS_BIN="$(command -v goimports || true)"
if [[ -z "${GOIMPORTS_BIN}" ]]; then
	go install golang.org/x/tools/cmd/goimports@latest
	GOIMPORTS_BIN="$(go env GOPATH)/bin/goimports"
fi

STATICCHECK_BIN="$(command -v staticcheck || true)"
if [[ -z "${STATICCHECK_BIN}" ]]; then
	go install honnef.co/go/tools/cmd/staticcheck@latest
	STATICCHECK_BIN="$(go env GOPATH)/bin/staticcheck"
fi

GO_TEST_TIMEOUT="${GO_TEST_TIMEOUT:-2m}"

cd "${REPO_ROOT}"

echo "Running goimports..."
"${GOIMPORTS_BIN}" -w .

echo "Running unit tests with race detector..."
go test -race -count=1 --timeout "${GO_TEST_TIMEOUT}" "$@" ./...

echo "Running go vet..."
go vet ./...

echo "Running errcheck..."
"${ERRCHECK_BIN}" ./...

echo "Running staticcheck..."
"${STATICCHECK_BIN}" -checks=SA* ./...

echo "Done."
