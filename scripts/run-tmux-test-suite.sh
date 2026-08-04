#!/usr/bin/env bash -ex
SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
REPO_ROOT="${SCRIPT_DIR}/.."
cd ${REPO_ROOT}
go run ./cmd/run-tmux-test-suite "$@"
