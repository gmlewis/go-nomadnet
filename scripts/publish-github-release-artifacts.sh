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
# The Go program's LAST step prunes release-asset storage: the assets of every
# release older than the newest 10 releases that still have assets are deleted.
# Releases, their notes, and their git tags are never deleted, so module
# checksums and changelog links stay valid.
#
# Usage:
#
#	./scripts/publish-github-release-artifacts.sh          # publish a new release
#	./scripts/publish-github-release-artifacts.sh --force  # replace an existing one
#
#	# Preview everything a publish would do, changing nothing remotely:
#	./scripts/publish-github-release-artifacts.sh --dry-run
#
#	# Fast preview (or real run) of the asset pruning alone, no build:
#	./scripts/publish-github-release-artifacts.sh --prune-only --dry-run
#	./scripts/publish-github-release-artifacts.sh --prune-only
#
#	# Change the retention window (default 10 releases):
#	./scripts/publish-github-release-artifacts.sh --keep-releases 20
#
# The prune is safe to interrupt: it recomputes its plan from live state on
# every run, so re-running --prune-only picks up where it left off.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}/.."
cd "${REPO_ROOT}"

go run ./cmd/publish-github-release-artifacts "$@"

# Pull the tags into this repo, but only for a run that could have created one.
# A publish tags HEAD, so the tag it pushed is worth fetching; --prune-only and
# --dry-run touch no tags at all, and pulling then is not just pointless but
# actively misleading: under `set -e` a failing `git pull` (for example with an
# unstaged edit in the tree) would make a successful prune report failure.
for arg in "$@"; do
	case "${arg}" in
	# Only the forms that turn these flags ON skip the pull; --dry-run=false or
	# --prune-only=false is a real publish, which does tag HEAD and wants the pull.
	--prune-only | --prune-only=true | -n | -n=true | --dry-run | --dry-run=true)
		exit 0
		;;
	esac
done

git pull --tags
