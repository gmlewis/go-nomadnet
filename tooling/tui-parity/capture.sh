#!/usr/bin/env bash
# -*- compile-command: "./capture.sh --target orig --size 135x32 --keys Up,Right,Enter"; -*-
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# capture.sh — headless TUI capture harness for behavioral-parity testing.
#
# Drives either the source-of-truth Python `nomadnet` (urwid) or this Go port
# (`gonomadnet`, tview/tcell) inside a detached tmux session (a real PTY, which
# both toolkits require), injects a sequence of keystrokes, and captures the
# rendered screen after each key — both as plain text and as ANSI-styled text
# (with SGR color/reverse-video escapes preserved) so focus highlights and
# color application can be inspected without a human at a terminal.
#
# See README.md in this directory for the full workflow and examples.
#
# Usage:
#   capture.sh --target orig|go [--size WxH] [--keys K1,K2,...] \
#              [--out DIR] [--label NAME] [--boot SECS] [--config DIR] \
#              [--fresh] [--bin PATH] [--extra ARGS...]
#
# Examples:
#   # Original first-run Guide at the recommended size, walk the topic list:
#   ./capture.sh --target orig --size 135x32 --fresh \
#       --keys Left,Down,Down,Down,Down,Down,Down,Enter --label guide
#
#   # Go port Network page, then switch menu with Right, then walk the list:
#   ./capture.sh --target go --size 135x32 --keys Right,Down,Down,Down \
#       --label network --boot 25
#
# Output files (per frame NN = 00..): <out>/<label>_<W>x<H>_<NN>_plain.txt
#                                     <out>/<label>_<W>x<H>_<NN>_esc.txt
#                                     <out>/<label>_<W>x<H>_<NN>.txt   (trimmed plain)
# Plus <out>/manifest.txt describing the run.

set -u

TARGET=""
SIZE="135x32"
KEYS=""
OUT="./captures"
LABEL="cap"
BOOT=""
CONFIG=""
FRESH=0
BIN=""
EXTRA=()

usage() { sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --target)  TARGET="$2"; shift 2;;
    --size)    SIZE="$2"; shift 2;;
    --keys)    KEYS="$2"; shift 2;;
    --out)     OUT="$2"; shift 2;;
    --label)   LABEL="$2"; shift 2;;
    --boot)    BOOT="$2"; shift 2;;
    --config)  CONFIG="$2"; shift 2;;
    --fresh)   FRESH=1; shift;;
    --bin)     BIN="$2"; shift 2;;
    --extra)   EXTRA+=("$2"); shift 2;;
    -h|--help) usage 0;;
    *) echo "unknown arg: $1" >&2; usage 1;;
  esac
done

[ "$TARGET" = "orig" ] || [ "$TARGET" = "go" ] || { echo "--target orig|go is required" >&2; exit 1; }
W="${SIZE%x*}"; H="${SIZE#*x}"
case "$W" in ''|*[!0-9]*) echo "bad width in --size $SIZE" >&2; exit 1;; esac
case "$H" in ''|*[!0-9]*) echo "bad height in --size $SIZE" >&2; exit 1;; esac

# Default boot time: Go needs to compile (go run) + init RNS; original is fast.
[ -n "$BOOT" ] || { [ "$TARGET" = "go" ] && BOOT=25 || BOOT=5; }

# Resolve a config dir. The original and the port both accept --config <dir>;
# a fresh dir triggers the first-run Guide for the original and isolates state.
if [ -z "$CONFIG" ]; then
  CONFIG="$(mktemp -d "${TMPDIR:-/tmp}/tui-parity-${TARGET}-XXXXXX")"
  CREATED_CFG=1
else
  CREATED_CFG=0
  if [ "$FRESH" = "1" ] && [ -d "$CONFIG" ]; then
    rm -rf "$CONFIG"; mkdir -p "$CONFIG"
  fi
fi
# Resolve the binary/command for the target.
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
if [ "$TARGET" = "orig" ]; then
  if [ -z "$BIN" ]; then
    if command -v nomadnet >/dev/null 2>&1; then
      BIN="nomadnet"
    else
      echo "nomadnet not found on PATH. Install it, or pass --bin /path/to/nomadnet" >&2
      exit 1
    fi
  fi
  CMD="$BIN --config $CONFIG ${EXTRA[*]:-}"
else
  # Go port: `go run` from the repo root. --bin can override with a prebuilt binary.
  if [ -z "$BIN" ]; then
    CMD="cd $REPO_ROOT && go run ./cmd/gonomadnet -t -config $CONFIG ${EXTRA[*]:-}"
  else
    CMD="$BIN -t -config $CONFIG ${EXTRA[*]:-}"
  fi
fi

mkdir -p "$OUT"
SESS="tui-parity-$$"
tmux -f /dev/null new-session -d -x "$W" -y "$H" -s "$SESS" "$CMD; echo __EXIT=\$?; sleep 120"

cap() {
  local idx="$1"
  tmux capture-pane -t "$SESS" -p -e > "$OUT/${LABEL}_${W}x${H}_${idx}_esc.txt" 2>/dev/null
  tmux capture-pane -t "$SESS" -p   > "$OUT/${LABEL}_${W}x${H}_${idx}_plain.txt" 2>/dev/null
  awk '{sub(/[ \t]+$/,"")} {lines[NR]=$0} END{for(i=NR;i>=1;i--){if(lines[i]!=""){last=i;break}} for(i=1;i<=last;i++) print lines[i]}' \
    "$OUT/${LABEL}_${W}x${H}_${idx}_plain.txt" > "$OUT/${LABEL}_${W}x${H}_${idx}.txt" 2>/dev/null
}

# Wait for boot. We poll the pane; once it has non-empty content we capture,
# but we still wait the full --boot for toolkits that paint late (Go/tview).
sleep "$BOOT"

cap 00
FRAMES=1
if [ "$KEYS" != "" ]; then
  IFS=',' read -ra KARR <<< "$KEYS"
  for k in "${KARR[@]}"; do
    tmux send-keys -t "$SESS" -N 1 "$k"
    sleep 1
    cap "$(printf '%02d' "$FRAMES")"
    FRAMES=$((FRAMES+1))
  done
fi

tmux kill-session -t "$SESS" 2>/dev/null || true

{
  echo "target=$TARGET size=${W}x${H} label=$LABEL boot=${BOOT}s config=$CONFIG"
  echo "keys=$KEYS"
  echo "frames=$FRAMES"
  echo "bin/cmd=$CMD"
  echo "frames_captured:"
  i=0
  while [ "$i" -lt "$FRAMES" ]; do
    f="$(printf '%02d' "$i")"
    if [ "$i" -eq 0 ]; then
      prev="(initial)"
    else
      prev="$(echo "$KEYS" | cut -d, -f"$i")"
    fi
    echo "  ${LABEL}_${W}x${H}_${f}.txt  (key before this frame: $prev)"
    i=$((i+1))
  done
} > "$OUT/manifest.txt"

echo "captured $FRAMES frame(s) -> $OUT  (config: $CONFIG)"
echo "manifest: $OUT/manifest.txt"