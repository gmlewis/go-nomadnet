#!/bin/bash -e
# -*- compile-command: "./test-integration.sh"; -*-

# Copyright 2026 Glenn Lewis. All rights reserved.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.

# test-integration.sh runs integration tests with the 'integration' build tag.
# These tests verify Go/Python parity and NomadNet/LXMF integration.

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
REPO_ROOT="${SCRIPT_DIR}/.."

# Point to the original Python repo directories for parity testing
export ORIGINAL_NOMADNET_REPO_DIR="${ORIGINAL_NOMADNET_REPO_DIR:-$HOME/src/github.com/markqvist/nomadnet}"

GO_TEST_TIMEOUT="${GO_TEST_TIMEOUT:-4m}"

if [[ -z "${GO_TEST_TAGS:-}" ]]; then
	if [[ "$(uname -a)" == *"Darwin"* ]]; then
		GO_TEST_TAGS="integration,darwin"
	elif [[ "$(uname -a)" == *"Linux"* ]]; then
		GO_TEST_TAGS="integration,linux"
	else
		GO_TEST_TAGS="integration"
	fi
fi

echo "Using go test tags: ${GO_TEST_TAGS}"

if [[ -z "${GO_TEST_P:-}" && "$(uname -a)" == *"Darwin"* ]]; then
	GO_TEST_P=8
fi
if [[ -z "${GO_TEST_PARALLEL:-}" && "$(uname -a)" == *"Darwin"* ]]; then
	GO_TEST_PARALLEL=1
fi

GO_TEST_ARGS=()
if [[ -n "${GO_TEST_P:-}" ]]; then
	GO_TEST_ARGS+=(-p "${GO_TEST_P}")
	echo "Using go test package parallelism: ${GO_TEST_P}"
fi
if [[ -n "${GO_TEST_PARALLEL:-}" ]]; then
	GO_TEST_ARGS+=(-parallel "${GO_TEST_PARALLEL}")
	echo "Using go test intra-package parallelism: ${GO_TEST_PARALLEL}"
fi

cd "${REPO_ROOT}"

echo "Running gofmt..."
gofmt -s -w .

echo "Running integration tests..."
go test "${GO_TEST_ARGS[@]}" -race -tags="${GO_TEST_TAGS}" -count=1 -timeout "${GO_TEST_TIMEOUT}" "$@" ./...

echo "Running go vet..."
go vet ./...

echo "Done."
