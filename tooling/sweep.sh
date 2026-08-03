#!/usr/bin/env bash
# -*- compile-command: "tooling/sweep.sh"; -*-
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# sweep.sh — curated mechanical key-sequence parity sweep across every menu of
# both the source-of-truth Python nomadnet (urwid) and the Go port
# (gonomadnet, tview/tcell), using tui-parity/capture.sh (tmux, per-key frames,
# truecolor-forced). Fully isolated: a fresh no-interface RNS config per call
# AND a per-target seeded nomadnet config (firstrun=False) copied per state, so
# NO live network and NO touch to ~/.reticulum or ~/.nomadnetwork.
#
# Why seeded (not --fresh) configs: a freshly-created nomadnet config sets
# firstrun=True, which makes the Guide page MODAL — menu activation does not
# switch the body, so per-menu content can't be exercised. A seeded config (an
# existing config dir the app already wrote once) loads with firstrun=False and
# boots straight to the Conversations page, where menu navigation works. The
# one "firstrun" state uses --fresh to capture the real Guide-boot parity.
#
# Key model (verified): at a seeded boot, focus is in the Conversations BODY.
# Up reaches the menu (highlight on Conversations, the active page); Right
# highlights the next menu item; Enter activates it and focus moves to that
# page's body. Menu order: Conversations(0) Network(1) Channels(2) Log(3)
# Interfaces(4) Config(5) Guide(6) Quit(7). Destructive confirms (Quit-activate,
# delete, block, remove) are NOT triggered.
#
# The structural diff NORMALIZES away artifacts that are not parity bugs:
#   - identity/LXMF hashes in <hex> (the two seeds have different identities)
#   - relative time phrases ("just now", "N seconds ago", …) (the seeds were
#     generated at slightly different instants)
# so the frame-by-frame report flags only genuine layout/text/color divergence.
#
# Output: $OUT/<target>/<state>/*.txt frames + $OUT/report.txt (per-state
# MATCH/DIFF). Usage:
#   tooling/sweep.sh [--go-bin PATH] [--out DIR] [--size WxH] [--boot-go S] [--boot-py S]
set -u

GO_BIN=""
OUT="/tmp/sweep-runs"
SIZE="100x30"
BOOT_GO="12"
BOOT_PY="8"
SEED_ORIG="/tmp/sweep-seed-orig"
SEED_GO="/tmp/sweep-seed-go"
while [ $# -gt 0 ]; do
  case "$1" in
    --go-bin)   GO_BIN="$2"; shift 2;;
    --out)      OUT="$2"; shift 2;;
    --size)     SIZE="$2"; shift 2;;
    --boot-go)  BOOT_GO="$2"; shift 2;;
    --boot-py)  BOOT_PY="$2"; shift 2;;
    --seed-orig) SEED_ORIG="$2"; shift 2;;
    --seed-go)  SEED_GO="$2"; shift 2;;
    -h|--help)  sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'; exit 0;;
    *) echo "unknown arg: $1" >&2; exit 1;;
  esac
done

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CAP="$REPO_ROOT/tooling/tui-parity/capture.sh"

# Resolve / build the Go binary once.
if [ -z "$GO_BIN" ]; then
  GO_BIN="/tmp/gonomadnet-sweep-bin"
  echo "building Go binary -> $GO_BIN ..."
  ( cd "$REPO_ROOT" && GOCACHE=/tmp/go-cache go build -o "$GO_BIN" ./cmd/gonomadnet ) || { echo "go build failed" >&2; exit 1; }
fi

# make_isolated_rns writes a no-interface RNS config dir (fully offline).
make_isolated_rns() {
  local d; d=$(mktemp -d /tmp/sweep-rns-XXXXXX)
  cat > "$d/config" <<'EOF'
[reticulum]
  share_instance = No
  enable_transport = No

[logging]
  loglevel = 4

[interfaces]
EOF
  echo "$d"
}

# gen_seed boots a target fresh for a few seconds so the app writes its default
# nomadnet config + identity into seeddir, then kills it. The resulting dir
# loads with firstrun=False on subsequent boots. (colormode is 24bit by default
# in both ports' generated configs, so no patching needed for truecolor parity.)
gen_seed() {
  local target="$1" bin="$2" rnsflag="$3" seeddir="$4"
  rm -rf "$seeddir"; mkdir -p "$seeddir"
  local rns; rns=$(make_isolated_rns)
  local tconf; tconf=$(mktemp); printf 'terminal-features RGB\n' > "$tconf"
  local sess="seed-$$-$RANDOM"
  if [ "$target" = "orig" ]; then
    tmux -f "$tconf" new-session -d -x 100 -y 30 -s "$sess" \
      "COLORTERM=truecolor $bin --config $seeddir $rnsflag $rns; sleep 120"
  else
    tmux -f "$tconf" new-session -d -x 100 -y 30 -s "$sess" \
      "COLORTERM=truecolor $bin -t -config $seeddir $rnsflag $rns; sleep 120"
  fi
  sleep 4
  tmux kill-session -t "$sess" 2>/dev/null || true
  rm -f "$tconf"
  [ -f "$seeddir/config" ] || { echo "seed generation FAILED for $target ($seeddir)" >&2; exit 1; }
}

# Ensure seeds exist.
if [ ! -f "$SEED_ORIG/config" ]; then
  echo "==> generating Python seed config -> $SEED_ORIG"
  PYBIN="$(command -v nomadnet || true)"
  [ -n "$PYBIN" ] || { echo "nomadnet not found on PATH" >&2; exit 1; }
  gen_seed orig "$PYBIN" "--rnsconfig" "$SEED_ORIG"
fi
if [ ! -f "$SEED_GO/config" ]; then
  echo "==> generating Go seed config -> $SEED_GO"
  gen_seed go "$GO_BIN" "-rnsconfig" "$SEED_GO"
fi

# Curated states: name|keys|mode  (mode = seeded|fresh; seeded copies the
# per-target seed config, fresh uses an empty dir = firstrun Guide boot).
# Seeded boot focus is in the Conversations body, so menu nav is Up,Right*,Enter.
STATES=(
  "boot||seeded"
  "firstrun||fresh"
  "conversations|Down,Up,C-n,Escape,C-g,C-g|seeded"
  "network|Up,Right,Enter,C-l,Down,Up|seeded"
  "channels|Up,Right,Right,Enter,C-u,C-y,F8,Down,Up|seeded"
  "log|Up,Right,Right,Right,Enter,Down,Up,PgUp,PgDn|seeded"
  "interfaces|Up,Right,Right,Right,Right,Enter,Down,Up|seeded"
  "config|Up,Right,Right,Right,Right,Right,Enter,Down,Up|seeded"
  "guide|Up,Right,Right,Right,Right,Right,Right,Enter,Down,Down,Down,Enter,Up,Enter|seeded"
  "quit|Up,Right,Right,Right,Right,Right,Right,Right|seeded"
)

run_target() {
  local target="$1" boot="$2" binarg="$3" seed="$4"
  local outdir="$OUT/$target"
  mkdir -p "$outdir"
  # Go flags are single-dash; Python argparse wants double-dash for --rnsconfig.
  local rnsflag
  if [ "$target" = "orig" ]; then rnsflag="--rnsconfig"; else rnsflag="-rnsconfig"; fi
  for entry in "${STATES[@]}"; do
    local name="${entry%%|*}"; local rest="${entry#*|}"
    # Split on separate lines: a `local a=.. b=$a` multi-assignment evaluates
    # the RHS against the pre-local state, so rest would be unset here.
    local keys="${rest%%|*}"; local mode="${rest#*|}"
    local rns; rns=$(make_isolated_rns)
    local args=( --target "$target" --boot "$boot" --size "$SIZE" \
                 --out "$outdir/$name" --label "$name" --extra "$rnsflag $rns" )
    if [ -n "$binarg" ]; then args+=( --bin "$binarg" ); fi
    if [ -n "$keys" ]; then args+=( --keys "$keys" ); fi
    if [ "$mode" = "fresh" ]; then
      args+=( --fresh )
    else
      # Copy the seeded config so each state starts from the same non-firstrun
      # baseline without mutating the seed. capture.sh uses --config as-is when
      # --fresh is absent, so pass the copy directly.
      local cfgcopy; cfgcopy=$(mktemp -d /tmp/sweep-cfg-XXXXXX)
      cp -R "$seed"/. "$cfgcopy"/
      args+=( --config "$cfgcopy" )
    fi
    echo "  [$target] $name  (mode=$mode keys: ${keys:-(none)})"
    "$CAP" "${args[@]}" >/dev/null 2>&1 || echo "    CAPTURE FAILED for $target/$name"
  done
}

echo "==> capturing Python (orig)"
run_target orig "$BOOT_PY" "" "$SEED_ORIG"
echo "==> capturing Go"
run_target go "$BOOT_GO" "$GO_BIN" "$SEED_GO"

# ---- normalize: mask non-parity artifacts (hashes, relative times, temp
# paths) so the diff flags only genuine layout/text divergence. ----
normalize() {
  sed -E \
    -e 's/<[0-9a-fA-F]{16,}>/<HASH>/g' \
    -e 's/(just now|[0-9]+ (seconds?|minutes?|hours?|days?|months?|years?) ago)/<RELTIME>/g' \
    -e 's@/tmp/[A-Za-z0-9_./-]+@<TMPPATH>@g' \
    "$1"
}

# ---- diff: per state, frame-by-frame normalized structural comparison ----
echo "==> diffing"
REPORT="$OUT/report.txt"
: > "$REPORT"
TOTAL=0; MATCHES=0; DIFFS=0
for entry in "${STATES[@]}"; do
  name="${entry%%|*}"
  py="$OUT/orig/$name"; go="$OUT/go/$name"
  [ -d "$py" ] && [ -d "$go" ] || { echo "  $name: MISSING captures" | tee -a "$REPORT"; continue; }
  py_frames=( $(ls "$py"/*.txt 2>/dev/null | grep -E '_[0-9]+\.txt$' | sort) )
  go_frames=( $(ls "$go"/*.txt 2>/dev/null | grep -E '_[0-9]+\.txt$' | sort) )
  npy=${#py_frames[@]}; ngo=${#go_frames[@]}
  TOTAL=$((TOTAL+1))
  state_status="MATCH"; detail=""
  if [ "$npy" -ne "$ngo" ]; then
    state_status="DIFF"; detail="frame count py=$npy go=$ngo"
  else
    for ((i=0;i<npy;i++)); do
      if ! diff -q <(normalize "${py_frames[$i]}") <(normalize "${go_frames[$i]}") >/dev/null 2>&1; then
        state_status="DIFF"
        detail="first differs at frame $i ($(basename "${py_frames[$i]}"))"
        break
      fi
    done
  fi
  if [ "$state_status" = "MATCH" ]; then MATCHES=$((MATCHES+1)); else DIFFS=$((DIFFS+1)); fi
  printf "  %-14s %s  %s\n" "$name" "$state_status" "$detail" | tee -a "$REPORT"
done

echo | tee -a "$REPORT"
printf "SUMMARY: %d states, %d MATCH, %d DIFF\n" "$TOTAL" "$MATCHES" "$DIFFS" | tee -a "$REPORT"
echo "report: $REPORT"
echo "frames: $OUT/{orig,go}/<state>/"