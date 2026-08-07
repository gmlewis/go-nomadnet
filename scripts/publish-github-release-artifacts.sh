#!/usr/bin/env bash
# -*- compile-command: "./scripts/publish-github-release-artifacts.sh"; -*-
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program. If not, see <https://www.gnu.org/licenses/>.

# publish-github-release-artifacts.sh is a thin wrapper around the Go program
# cmd/publish-github-release-artifacts. It changes to the repo root and
# forwards all arguments (e.g. --force) verbatim so the Go program does the
# real work of building, checksumming, and publishing the GitHub release.
#
# Usage:
#
#	./scripts/publish-github-release-artifacts.sh          # publish a new release
#	./scripts/publish-github-release-artifacts.sh --force  # replace an existing one

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}/.."
cd "${REPO_ROOT}"

go run ./cmd/publish-github-release-artifacts "$@"