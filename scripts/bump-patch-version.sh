#!/usr/bin/env bash
# -*- compile-command: "./scripts/bump-patch-version.sh"; -*-
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

# bump-patch-version.sh bumps ONLY the patch version component of the VERSION
# constant in nomadnet/version/version.go, leaving major and minor untouched
# (e.g. "0.1.0" -> "0.1.1", "1.9.3" -> "1.9.4").
#
# It refuses to run if the VERSION string is not a clean "MAJOR.MINOR.PATCH"
# semver (no pre-release/build metadata) to avoid silently corrupting the file.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}/.."
VERSION_FILE="${REPO_ROOT}/nomadnet/version/version.go"

if [[ ! -f "${VERSION_FILE}" ]]; then
	echo "error: version file not found: ${VERSION_FILE}" >&2
	exit 1
fi

# Extract the current quoted VERSION string from the Go source.
CURRENT="$(grep -E '^[[:space:]]*const[[:space:]]+VERSION[[:space:]]*=[[:space:]]*"[^"]+"' "${VERSION_FILE}" \
	| head -1 \
	| sed -E 's/.*"([^"]+)".*/\1/')"

if [[ -z "${CURRENT}" ]]; then
	echo "error: could not parse VERSION constant from ${VERSION_FILE}" >&2
	exit 1
fi

if ! echo "${CURRENT}" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "error: VERSION \"${CURRENT}\" is not a clean MAJOR.MINOR.PATCH semver;" \
		"refusing to bump." >&2
	exit 1
fi

MAJOR="$(echo "${CURRENT}" | cut -d. -f1)"
MINOR="$(echo "${CURRENT}" | cut -d. -f2)"
PATCH="$(echo "${CURRENT}" | cut -d. -f3)"
NEW_PATCH=$((PATCH + 1))
NEW_VERSION="${MAJOR}.${MINOR}.${NEW_PATCH}"

# In-place replace of the VERSION line. We anchor on the full line so we only
# touch the assignment and leave the surrounding file (copyright, comments)
# byte-for-byte unchanged. Use POSIX [[:space:]] (portable to BSD sed on
# macOS) rather than GNU \s.
if ! sed -i.bak -E \
	"s|^([[:space:]]*const[[:space:]]+VERSION[[:space:]]*=[[:space:]]*\")${CURRENT}(\")|\1${NEW_VERSION}\2|" \
	"${VERSION_FILE}"; then
	echo "error: failed to update ${VERSION_FILE}" >&2
	exit 1
fi
rm -f "${VERSION_FILE}.bak"

echo "Bumped version: ${CURRENT} -> ${NEW_VERSION} in ${VERSION_FILE}"
