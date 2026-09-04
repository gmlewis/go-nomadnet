#!/usr/bin/env bash
# -*- compile-command: "./scripts/sync-dependencies.sh -n" -*-
#
# sync-dependencies.sh — sync go.mod replace directives for the forked
# tcell and tview dependencies to the latest commits on their default
# branches, then clean up go.work so no local "use" overrides remain.
#
# Prerequisites:
#   - gh (GitHub CLI) must be installed and authenticated
#   - Local clones of gmlewis/tcell and gmlewis/tview must have NO
#     uncommitted changes (this script aborts immediately if they do).
#     Clone paths default to $HOME/src/github.com/gmlewis/{tcell,tview}
#     and can be overridden with the TCELL_LOCAL/TVIEW_LOCAL env vars.
#   - The working directory must be the go-nomadnet repo root
#
# Usage:
#   ./scripts/sync-dependencies.sh        # perform the sync
#   ./scripts/sync-dependencies.sh -n     # dry-run: print what would change
#
# The script is idempotent: running it multiple times in succession
# produces the same result.

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

TCELL_REPO="gmlewis/tcell"
TCELL_MODULE="github.com/gmlewis/tcell/v2"
TCELL_REPLACE="github.com/gdamore/tcell/v2"
TCELL_LOCAL="${TCELL_LOCAL:-$HOME/src/github.com/gmlewis/tcell}"

TVIEW_REPO="gmlewis/tview"
TVIEW_MODULE="github.com/gmlewis/tview"
TVIEW_REPLACE="github.com/rivo/tview"
TVIEW_LOCAL="${TVIEW_LOCAL:-$HOME/src/github.com/gmlewis/tview}"

DRY_RUN=0

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log()  { echo "[sync-deps] $*"; }
dry()  { if [ "$DRY_RUN" = 1 ]; then echo "[dry-run] $*"; fi; }
run()  { if [ "$DRY_RUN" = 1 ]; then dry "$*"; else log "$*"; "$@"; fi; }

# ---------------------------------------------------------------------------
# Parse flags
# ---------------------------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    -n|--dry-run) DRY_RUN=1; shift;;
    -h|--help)
      sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "unknown flag: $1" >&2; exit 1;;
  esac
done

# ---------------------------------------------------------------------------
# 0. Verify we are in the repo root
# ---------------------------------------------------------------------------

if [ ! -f go.mod ] || [ ! -d tui ]; then
  echo "error: run from the go-nomadnet repo root (go.mod + tui/ not found)" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# 1. Check for uncommitted changes in local tcell/tview clones
# ---------------------------------------------------------------------------

log "checking local clones for uncommitted changes..."

for path in "$TCELL_LOCAL" "$TVIEW_LOCAL"; do
  if [ ! -d "$path/.git" ]; then
    echo "error: $path is not a git repository" >&2
    exit 1
  fi
  # --porcelain: machine-readable; -uno: ignore untracked files (only staged/modified matter)
  status="$(cd "$path" && git status --porcelain -uno)"
  if [ -n "$status" ]; then
    echo "error: uncommitted changes in $path — commit or stash them first:" >&2
    echo "$status" >&2
    exit 1
  fi
done

log "local clones are clean."

# ---------------------------------------------------------------------------
# 2. Get latest commit SHA + date from default branches via gh
# ---------------------------------------------------------------------------

log "querying GitHub for latest commits..."

# Fetch default branch name, commit SHA, and committer date (UTC ISO 8601).
tcell_branch="$(gh api "repos/$TCELL_REPO" --jq '.default_branch')"
tcell_sha="$(gh api "repos/$TCELL_REPO/commits/$tcell_branch" --jq '.sha')"
tcell_date="$(gh api "repos/$TCELL_REPO/commits/$tcell_branch" --jq '.commit.committer.date')"

tview_branch="$(gh api "repos/$TVIEW_REPO" --jq '.default_branch')"
tview_sha="$(gh api "repos/$TVIEW_REPO/commits/$tview_branch" --jq '.sha')"
tview_date="$(gh api "repos/$TVIEW_REPO/commits/$tview_branch" --jq '.commit.committer.date')"

log "  tcell: $tcell_branch @ ${tcell_sha:0:12} ($tcell_date)"
log "  tview: $tview_branch @ ${tview_sha:0:12} ($tview_date)"

# ---------------------------------------------------------------------------
# 3. Construct Go pseudo-versions
# ---------------------------------------------------------------------------

# Pseudo-version format (with a preceding tag):
#   v<base>-0.<yyyyMMdd><hhmmss>-<short-sha>
# Pseudo-version format (no tags, v0.0.0 base):
#   v0.0.0-<yyyyMMdd><hhmmss>-<short-sha>
#
# <base> is the latest semver tag that is an ancestor of the commit,
# with the patch version incremented by 1 (Go convention for commits
# after the latest tag).

# Convert ISO 8601 UTC timestamp to yyyyMMddhhmmss
fmt_ts() {
  # Input: 2026-08-23T18:32:26Z  →  20260823183226
  date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "$1" "+%Y%m%d%H%M%S"
}

tcell_ts="$(fmt_ts "$tcell_date")"
tview_ts="$(fmt_ts "$tview_date")"

tcell_short="${tcell_sha:0:12}"
tview_short="${tview_sha:0:12}"

# --- tcell: find latest v2 tag, increment patch ---
tcell_tags_json="$(gh api "repos/$TCELL_REPO/tags" --paginate --jq '.[].name' 2>/dev/null || true)"
tcell_latest_v2="$(echo "$tcell_tags_json" | grep -E '^v2\.[0-9]+\.[0-9]+$' | sort -Vr | head -1 || true)"

if [ -z "$tcell_latest_v2" ]; then
  echo "error: no v2.x.y tags found for $TCELL_REPO" >&2
  exit 1
fi

# Increment the patch component: v2.13.10 → v2.13.11
tcell_major="$(echo "$tcell_latest_v2" | cut -d. -f1)"   # v2
tcell_minor="$(echo "$tcell_latest_v2" | cut -d. -f2)"   # 13
tcell_patch="$(echo "$tcell_latest_v2" | cut -d. -f3)"  # 10
tcell_patch=$((tcell_patch + 1))
tcell_base="${tcell_major}.${tcell_minor}.${tcell_patch}"  # v2.13.11

tcell_version="${tcell_base}-0.${tcell_ts}-${tcell_short}"

# --- tview: no tags → v0.0.0 base ---
# Check if tview has any tags at all
tview_tags_json="$(gh api "repos/$TVIEW_REPO/tags" --paginate --jq '.[].name' 2>/dev/null || true)"

if [ -n "$tview_tags_json" ]; then
  # Has tags — find latest, increment patch
  tview_latest="$(echo "$tview_tags_json" | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -Vr | head -1 || true)"
  if [ -n "$tview_latest" ]; then
    tview_major="$(echo "$tview_latest" | cut -d. -f1)"
    tview_minor="$(echo "$tview_latest" | cut -d. -f2)"
    tview_patch="$(echo "$tview_latest" | cut -d. -f3)"
    tview_patch=$((tview_patch + 1))
    tview_base="${tview_major}.${tview_minor}.${tview_patch}"
    tview_version="${tview_base}-0.${tview_ts}-${tview_short}"
  else
    tview_version="v0.0.0-${tview_ts}-${tview_short}"
  fi
else
  tview_version="v0.0.0-${tview_ts}-${tview_short}"
fi

log "  tcell pseudo-version: $tcell_version"
log "  tview pseudo-version: $tview_version"

# ---------------------------------------------------------------------------
# 4. Update go.mod replace directives
# ---------------------------------------------------------------------------

log "updating go.mod replace directives..."

run go mod edit -replace "$TCELL_REPLACE=$TCELL_MODULE@$tcell_version"
run go mod edit -replace "$TVIEW_REPLACE=$TVIEW_MODULE@$tview_version"

# ---------------------------------------------------------------------------
# 5. Remove local "use" directives from go.work (if it exists)
# ---------------------------------------------------------------------------

if [ -f go.work ]; then
  log "updating go.work (removing local tcell/tview use directives)..."

  # Use "go work edit -dropuse" to remove the local paths — this is the
  # only supported way to edit go.work and guarantees valid syntax.
  # Idempotent: -dropuse on a path that isn't listed is a silent no-op.
  run go work edit -dropuse "$TCELL_LOCAL" -dropuse "$TVIEW_LOCAL"
else
  log "go.work not found — skipping."
fi

# ---------------------------------------------------------------------------
# 6. go mod tidy + go work sync
# ---------------------------------------------------------------------------

run go mod tidy

if [ -f go.work ]; then
  run go work sync
fi

# ---------------------------------------------------------------------------
# 7. Summary
# ---------------------------------------------------------------------------

log "done."
log "  tcell: $tcell_version"
log "  tview: $tview_version"
if [ "$DRY_RUN" = 1 ]; then
  log "(dry-run: no changes were made)"
fi
