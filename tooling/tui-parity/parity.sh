#!/usr/bin/env bash
# -*- compile-command: "./parity.sh --label network --keys-orig Up,Right,Enter --keys-go Right"; -*-
#
# Copyright 2026 Glenn Lewis. All rights reserved.
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# parity.sh — capture the same logical scenario from BOTH the original Python
# `nomadnet` and the Go port, then print their structural summaries side by
# side so regressions are obvious.
#
# Because the two TUIs are driven differently (the original reaches a page via
# Up→Right→Enter; the Go port via Right), you pass separate key sequences per
# target. The --frame option selects which captured frame to summarize
# (default: the last one captured, i.e. after all keys).
#
# Usage:
#   parity.sh --label <name> [--size WxH] [--frame N]
#             [--keys-orig K1,K2,...] [--keys-go K1,K2,...]
#             [--boot-orig S] [--boot-go S] [--out DIR]
#
# Example — compare the Network page:
#   ./parity.sh --label network --keys-orig Up,Right,Enter --keys-go Right
#
# Example — compare the New Conversation dialog:
#   ./parity.sh --label newconv --keys-orig C-n --keys-go C-n --frame 1

set -u

LABEL=""
SIZE="135x32"
KEYS_ORIG=""
KEYS_GO=""
FRAME=""
BOOT_ORIG=""
BOOT_GO=""
OUT="./captures"
HERE="$(cd "$(dirname "$0")" && pwd)"

usage() { sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }
while [ $# -gt 0 ]; do
  case "$1" in
    --label)     LABEL="$2"; shift 2;;
    --size)      SIZE="$2"; shift 2;;
    --keys-orig) KEYS_ORIG="$2"; shift 2;;
    --keys-go)   KEYS_GO="$2"; shift 2;;
    --frame)     FRAME="$2"; shift 2;;
    --boot-orig) BOOT_ORIG="$2"; shift 2;;
    --boot-go)   BOOT_GO="$2"; shift 2;;
    --out)       OUT="$2"; shift 2;;
    -h|--help)   usage 0;;
    *) echo "unknown arg: $1" >&2; usage 1;;
  esac
done
[ -n "$LABEL" ] || { echo "--label is required" >&2; exit 1; }

ORIG_DIR="$OUT/orig"; GO_DIR="$OUT/go"
"$HERE/capture.sh" --target orig --size "$SIZE" --keys "$KEYS_ORIG" \
  --label "$LABEL" --out "$ORIG_DIR" ${BOOT_ORIG:+--boot "$BOOT_ORIG"} > /dev/null
"$HERE/capture.sh" --target go --size "$SIZE" --keys "$KEYS_GO" \
  --label "$LABEL" --out "$GO_DIR" ${BOOT_GO:+--boot "$BOOT_GO"} > /dev/null

W="${SIZE%x*}"; H="${SIZE#*x}"
# Pick the frame to summarize: explicit --frame N, else last frame (look at manifest).
pick_frame() {
  local dir="$1"
  if [ -n "$FRAME" ]; then printf '%02d' "$FRAME"; return; fi
  local n; n=$(grep -E '^frames=' "$dir/manifest.txt" | cut -d= -f2)
  printf '%02d' $((n-1))
}
FO="$(pick_frame "$ORIG_DIR")"; FG="$(pick_frame "$GO_DIR")"
OE="$ORIG_DIR/${LABEL}_${W}x${H}_${FO}_esc.txt"
GE="$GO_DIR/${LABEL}_${W}x${H}_${FG}_esc.txt"

echo "================================================================"
echo " PARITY: $LABEL  (${SIZE})  frame orig=$FO go=$FG"
echo "================================================================"
echo
echo "-------- ORIGINAL ($OE) --------"
python3 "$HERE/summary.py" "$OE"
echo
echo "-------- GO PORT ($GE) --------"
python3 "$HERE/summary.py" "$GE"
echo
echo "Diff the raw frames with:"
echo "  diff <(python3 $HERE/ansiview.py --plain \"$OE\") <(python3 $HERE/ansiview.py --plain \"$GE\")"