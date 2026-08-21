#!/usr/bin/env bash
# -*- compile-command: "./parity-mouse.sh --pages tooling/parity-fixtures/fixture-page --probe --cell 58,11 --py-only"; -*-
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# parity-mouse.sh — SGR mouse-event driver + link-discovery sweep for the
# nomadnet ⇄ gonomadnet parity comparator (workflow D).
#
# Built on workflow C's fixture node (cmd/serve-page) + Ctrl-u connect flow.
# Injects SGR-1006 mouse events into a tmux PTY via `tmux send-keys -l` (literal
# ESC bytes straight to the app's stdin, bypassing tmux's own mouse handling).
# Both renderers parse the SAME wire format:
#
#   ESC[<btn;col;row M   press / motion (1-based col/row)
#   ESC[<btn;col;row m   release
#
#   left click:  ESC[<0;col;row M  then  ESC[<0;col;row m
#   hover/move:  ESC[<35;col;row M  (3|0x20; tcell delivers a no-button motion;
#                 urwid only reports motion while a button is held — ?1002 not
#                 ?1003 — so pure hover is a NO-OP in Python nomadnet; links are
#                 activated by a left PRESS, see LinkableText.mouse_event)
#   wheel up:    ESC[<64;col;row M   wheel down: ESC[<65;col;row M
#
# Modes:
#   --probe --cell C,R [--hover|--click]   inject one event at a cell, capture
#     before/after, report whether that cell's style changed (hover highlight)
#     and/or the browser navigated (URL/title-bar change). Default --click.
#   --sweep   click every browser-pane text-run start cell, detect navigation,
#     build a link-region map per target; diff Go vs Python.
#
# Usage:
#   parity-mouse.sh --pages <dir> [--size WxH] [--boot SECS] [--connect-wait SECS]
#       (--probe --cell C,R [--hover|--click] | --sweep)
#       [--py-only | --go-only] [--keep]
#
# <dir> must contain index.mu (the page both clients browse after Ctrl-u).

set -u

PAGES=""
SIZE="100x28"
BOOT=""
CONNECT_WAIT=""
MODE=""
CELL=""
KEYS=""
EVENT="click"
KEEP=0
PY_ONLY=0
GO_ONLY=0

usage() { sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --pages)         PAGES="$2"; shift 2;;
    --size)          SIZE="$2"; shift 2;;
    --boot)          BOOT="$2"; shift 2;;
    --connect-wait)  CONNECT_WAIT="$2"; shift 2;;
    --probe)         MODE="probe"; shift;;
    --sweep)         MODE="sweep"; shift;;
    --keys)          MODE="keys"; shift;;
    --cell)          CELL="$2"; shift 2;;
    --seq)           KEYS="$2"; shift 2;;
    --hover)         EVENT="hover"; shift;;
    --click)         EVENT="click"; shift;;
    --keep)          KEEP=1; shift;;
    --py-only)       PY_ONLY=1; shift;;
    --go-only)       GO_ONLY=1; shift;;
    -h|--help)       usage 0;;
    *) echo "unknown arg: $1" >&2; usage 1;;
  esac
done

[ -n "$PAGES" ] || { echo "--pages <dir> is required (must contain index.mu)" >&2; exit 1; }
[ -f "$PAGES/index.mu" ] || { echo "$PAGES/index.mu not found" >&2; exit 1; }
[ -n "$MODE" ] || { echo "need --probe, --sweep, or --keys" >&2; exit 1; }
[ "$MODE" = "probe" ] && [ -z "$CELL" ] && { echo "--probe needs --cell C,R (0-based screen coords)" >&2; exit 1; }
[ "$MODE" = "keys" ] && [ -z "$KEYS" ] && { echo "--keys needs --seq 'Right,Right,Enter'" >&2; exit 1; }
[ "$PY_ONLY" = "1" ] && [ "$GO_ONLY" = "1" ] && { echo "--py-only and --go-only are mutually exclusive" >&2; exit 1; }

W="${SIZE%x*}"; H="${SIZE#*x}"
[ "$W" = "${W//[!0-9]/}" ] && [ -n "$W" ] || { echo "bad width in --size $SIZE" >&2; exit 1; }
[ "$H" = "${H//[!0-9]/}" ] && [ -n "$H" ] || { echo "bad height in --size $SIZE" >&2; exit 1; }

[ -n "$BOOT" ] || BOOT=30
[ -n "$CONNECT_WAIT" ] || CONNECT_WAIT=15

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TS="$(date +%s)"
OUT="/tmp/parity-mouse-${TS}"
mkdir -p "$OUT"

ANSIVIEW="$REPO_ROOT/tooling/tui-parity/ansiview.py"
PY3="/opt/homebrew/bin/python3"
[ -x "$PY3" ] || PY3="python3"
DEFAULT_CFG="$REPO_ROOT/tooling/parity-fixtures/default-nomadnet.cfg"

# ---------------------------------------------------------------------------
# 1. Build + start the fixture node (cmd/serve-page). Same as parity-ab.sh.
# ---------------------------------------------------------------------------
SERVE_BIN="$OUT/serve-page"
echo "building serve-page..." >&2
( cd "$REPO_ROOT" && GOCACHE=/tmp/go-cache go build -o "$SERVE_BIN" ./cmd/serve-page ) || { echo "go build serve-page failed" >&2; exit 1; }

# Copy the fixture to a scratch dir and serve THAT, so __LOCAL_NODE__
# placeholders can be rewritten to the real node hash in place after startup
# (the node serves .mu files per request from the filesystem) without mutating
# the checked-in fixture.
PAGES_DIR="$OUT/pages"
rm -rf "$PAGES_DIR"; cp -R "$PAGES" "$PAGES_DIR"

"$SERVE_BIN" -pages "$PAGES_DIR" > "$OUT/server.out" 2> "$OUT/server.err" &
SRV_PID=$!
cleanup() {
  [ "$KEEP" = "1" ] && return
  tmux kill-session -t pms-py 2>/dev/null || true
  tmux kill-session -t pms-go 2>/dev/null || true
  kill "$SRV_PID" 2>/dev/null || true
  wait "$SRV_PID" 2>/dev/null || true
}
trap cleanup EXIT

i=0
while [ $i -lt 50 ]; do
  [ -s "$OUT/server.out" ] && grep -q NODE_HASH "$OUT/server.out" && grep -q PORT "$OUT/server.out" && break
  sleep 0.2; i=$((i+1))
done
HASH="$(grep NODE_HASH "$OUT/server.out" | cut -d= -f2)"
SVPORT="$(grep PORT "$OUT/server.out" | cut -d= -f2)"
[ -n "$HASH" ] && [ -n "$SVPORT" ] || { echo "serve-page did not report NODE_HASH/PORT" >&2; cat "$OUT/server.out" >&2; exit 1; }
echo "fixture node: hash=$HASH port=$SVPORT" >&2

# Rewrite __LOCAL_NODE__ placeholders in the served copy to the real node hash
# so embedded links resolve to THIS (connected) node. The node serves .mu files
# per request from the filesystem, so rewriting after startup is visible to
# later requests. This makes click→navigate produce a real content change
# (links point to a reachable page on the local node), giving a robust
# navigation signal independent of the URL-bar truncation difference.
grep -rl '__LOCAL_NODE__' "$PAGES_DIR" 2>/dev/null | while read -r f; do
  sed "s/__LOCAL_NODE__/$HASH/g" "$f" > "$f.tmp" && mv "$f.tmp" "$f"
done
echo "rewrote __LOCAL_NODE__ -> $HASH in $PAGES_DIR" >&2

make_rns_config() {
  local dir="$1"; mkdir -p "$dir"
  cat > "$dir/config" <<EOF
[reticulum]
share_instance = No
[logging]
loglevel = 4
[interfaces]
  [[parity client]]
    type = TCPClientInterface
    target_host = 127.0.0.1
    target_port = $SVPORT
    interface_enabled = True
EOF
}
make_app_config() {
  local dir="$1"; mkdir -p "$dir/pages" "$dir/files"; cp "$DEFAULT_CFG" "$dir/config"
}

# ---------------------------------------------------------------------------
# 2. Mouse injection helpers (SGR-1006). col/row are 0-based screen coords
#    here; the wire format is 1-based so we add 1.
# ---------------------------------------------------------------------------
# Send literal bytes (including ESC) to a tmux pane's PTY.
send_bytes() { tmux send-keys -t "$1" -l "$2"; }

mouse_move()  { # session col row  (no-button motion: 3|0x20=35)
  send_bytes "$1" "$(printf '\033[<35;%d;%dM' "$(( $2 + 1 ))" "$(( $3 + 1 ))")"
}
mouse_down()  { # session col row  (left press: 0)
  send_bytes "$1" "$(printf '\033[<0;%d;%dM'  "$(( $2 + 1 ))" "$(( $3 + 1 ))")"
}
mouse_up()    { # session col row  (release: 0, terminator m)
  send_bytes "$1" "$(printf '\033[<0;%d;%dm'  "$(( $2 + 1 ))" "$(( $3 + 1 ))")"
}
mouse_click() { # session col row  (press + release)
  mouse_down "$1" "$2" "$3"; sleep 0.15; mouse_up "$1" "$2" "$3"
}
wheel_up()    { send_bytes "$1" "$(printf '\033[<64;%d;%dM' "$(( $2 + 1 ))" "$(( $3 + 1 ))")"; }
wheel_down()  { send_bytes "$1" "$(printf '\033[<65;%d;%dM' "$(( $2 + 1 ))" "$(( $3 + 1 ))")"; }

capture_plain() { tmux capture-pane -t "$1" -p 2>/dev/null; }   # stdout
capture_styles() { tmux capture-pane -t "$1" -p -e 2>/dev/null; }

# ---------------------------------------------------------------------------
# 3. Launch + connect a target to the fixture page (same sequence as
#    parity-ab.sh). Leaves the tmux session ALIVE (caller may --keep).
# ---------------------------------------------------------------------------
boot_target() {
  local target="$1" rnscfg="$2" appcfg="$3"
  local sess="pms-$target" launch
  if [ "$target" = "py" ]; then
    launch="COLORTERM=truecolor nomadnet -t --config '$appcfg' --rnsconfig '$rnscfg'"
  else
    launch="cd '$REPO_ROOT' && COLORTERM=truecolor go run ./cmd/gonomadnet -t -config '$appcfg' -rnsconfig '$rnscfg'"
  fi
  local tmuxconf; tmuxconf="$(mktemp /tmp/parity-mouse-tmux-XXXXXX)"
  printf 'terminal-features RGB\n' > "$tmuxconf"
  tmux -f "$tmuxconf" new-session -d -x "$W" -y "$H" -s "$sess" "$launch; echo __EXIT=\$?; sleep 600"
  rm -f "$tmuxconf"

  echo "[$target] booting (up to ${BOOT}s)..." >&2
  local b=0
  while [ $b -lt "$BOOT" ]; do
    capture_plain "$sess" | grep -q "Conversations" && break
    sleep 1; b=$((b+1))
  done
  echo "[$target] navigating to Network + Ctrl-u connect to $HASH" >&2
  tmux send-keys -t "$sess" Home; sleep 0.5
  tmux send-keys -t "$sess" Up; sleep 0.5
  tmux send-keys -t "$sess" Right; sleep 0.5
  tmux send-keys -t "$sess" Enter; sleep 1
  tmux send-keys -t "$sess" Down; sleep 0.5
  tmux send-keys -t "$sess" Right; sleep 0.5
  tmux send-keys -t "$sess" C-u; sleep 1
  tmux send-keys -t "$sess" -l "$HASH"; sleep 0.5
  tmux send-keys -t "$sess" Enter
  echo "[$target] waiting ${CONNECT_WAIT}s for the page to render..." >&2
  sleep "$CONNECT_WAIT"
  capture_plain "$sess" > "$OUT/${target}_baseline.txt"
  echo "[$target] baseline captured -> $OUT/${target}_baseline.txt" >&2
}

# ---------------------------------------------------------------------------
# 4a. PROBE: inject one event at a cell, compare before/after.
#     Reports: (a) did the target cell's style change (hover highlight)?
#              (b) did the browser title-bar URL change (navigation)?
# ---------------------------------------------------------------------------
run_probe() {
  local target="$1"
  local sess="pms-$target"
  local col="${CELL%,*}" row="${CELL#*,}"
  [ "$col" = "${col//[!0-9]/}" ] && [ "$row" = "${row//[!0-9]/}" ] || { echo "bad --cell $CELL (expect C,R 0-based)" >&2; exit 1; }

  capture_plain "$sess" > "$OUT/${target}_before_plain.txt"
  capture_styles "$sess" > "$OUT/${target}_before.txt"

  case "$EVENT" in
    hover) mouse_move "$sess" "$col" "$row"; sleep 0.5;;
    click) mouse_click "$sess" "$col" "$row"; sleep 2.0;;
  esac

  capture_plain "$sess" > "$OUT/${target}_after_plain.txt"
  capture_styles "$sess" > "$OUT/${target}_after.txt"

  # Navigation signal: did the browser CONTENT (right pane) change? This is
  # robust against the URL-bar truncation difference (Go shows /page/ for every
  # URL). Also extract the cleaned URL-bar text as a secondary signal.
  "$PY3" - "$OUT/${target}_before_plain.txt" "$OUT/${target}_after_plain.txt" \
         "$OUT/${target}_before.txt" "$OUT/${target}_after.txt" \
         "$row" "$col" "$target" "$EVENT" <<'PY'
import re, sys
bp, ap, bs, as_ = sys.argv[1:5]
row, col, target, event = int(sys.argv[5]), int(sys.argv[6]), sys.argv[7], sys.argv[8]

def right_pane(path):
    # The browser pane is everything after the rightmost '||' box separator on
    # each line; strip trailing spaces. Crude but stable for the fixture layout.
    out = []
    for line in open(path, "r", errors="replace").read().split("\n"):
        # drop SGR for the content comparison
        bare = re.sub(r'\x1b\[[0-9;]*m', '', line)
        if '││' in bare:
            out.append(bare.split('││',1)[1].rstrip())
        else:
            out.append(bare.rstrip())
    return "\n".join(out)

def url_bar(path):
    txt = open(path, "r", errors="replace").read()
    bare = re.sub(r'\x1b\[[0-9;]*m', '', txt)
    m = re.search(r'/page/[^\s│]*', bare)
    return m.group(0) if m else "(none)"

before_content = right_pane(bp)
after_content  = right_pane(ap)
content_changed = before_content != after_content
before_url = url_bar(bs)
after_url  = url_bar(as_)

def cell_style(path, row, col):
    lines = open(path, "r", errors="replace").read().split("\n")
    if row >= len(lines): return None
    line = lines[row]
    cur = set(); pos = 0; style_at = None; i = 0
    while i < len(line):
        m = re.match(r'\x1b\[([0-9;]*)m', line[i:])
        if m:
            params = m.group(1)
            if params == "": cur.clear()
            else:
                for p in params.split(";"):
                    if p in ("0",""): cur.clear()
                    elif p == "1": cur.add("bold")
                    elif p == "4": cur.add("underline")
                    elif p == "7": cur.add("reverse")
                    elif p.startswith("38;2;"): cur.add("fg="+p)
                    elif p.startswith("48;2;"): cur.add("bg="+p)
            i += m.end(); continue
        if pos == col: style_at = sorted(cur)
        pos += 1; i += 1
    return style_at

sb = cell_style(bs, row, col)
sa = cell_style(as_, row, col)

print("[%s probe] cell=(%d,%d) event=%s" % (target, col, row, event))
print("  url bar before: %s" % before_url)
print("  url bar after:  %s" % after_url)
print("  content changed: %s" % content_changed)
print("  cell style before: %s" % sb)
print("  cell style after:  %s" % sa)
print("  cell style changed (hover highlight): %s" % (sb != sa))
if content_changed:
    # show the first differing content line
    bl = before_content.split("\n"); al = after_content.split("\n")
    for i in range(max(len(bl),len(al))):
        b = bl[i] if i < len(bl) else ""
        a = al[i] if i < len(al) else ""
        if b != a:
            print("  first content diff at line %d:" % i)
            print("    before: %r" % b[:80])
            print("    after:  %r" % a[:80])
            break
    print("  RESULT: NAVIGATED (content changed)")
else:
    print("  RESULT: no navigation (content unchanged)")
PY
}

# ---------------------------------------------------------------------------
# 4b. KEYS: send a tmux key sequence (comma-separated key names) after the
#     page loads, then report content change + the footer link-peek line.
#     For keyboard arrow-cursor link traversal: e.g. --seq "Right,Right,...,Enter"
#     sends Right to walk the part-cursor onto a link, Enter to follow it.
# ---------------------------------------------------------------------------
run_keys() {
  local target="$1"
  local sess="pms-$target"
  capture_plain "$sess" > "$OUT/${target}_keys_before.txt"
  # Footer link-peek: the bottom status row shows "Link to <target>" when the
  # cursor is on a link (Python marked_link / Go PeekLink). Capture it before.
  local before_footer; before_footer="$(tail -3 "$OUT/${target}_keys_before.txt" | grep -o 'Link to[^ ]*[^ ]' | head -1)"
  [ -n "$before_footer" ] || before_footer="(none)"

  local IFS=','
  for k in $KEYS; do
    # tmux send-keys resolves key names (Right, Enter, C-d, Tab, Space ...).
    tmux send-keys -t "$sess" "$k"
    sleep 0.6
  done
  sleep 1.5
  capture_plain "$sess" > "$OUT/${target}_keys_after.txt"
  local after_footer; after_footer="$(tail -3 "$OUT/${target}_keys_after.txt" | grep -o 'Link to[^ ]*[^ ]' | head -1)"
  [ -n "$after_footer" ] || after_footer="(none)"

  "$PY3" - "$OUT/${target}_keys_before.txt" "$OUT/${target}_keys_after.txt" "$target" <<'PY'
import re, sys
def right_pane(path):
    out = []
    for line in open(path, "r", errors="replace").read().split("\n"):
        bare = re.sub(r'\x1b\[[0-9;]*m', '', line)
        out.append(bare.split('││',1)[1].rstrip() if '││' in bare else bare.rstrip())
    return "\n".join(out)
b = right_pane(sys.argv[1]); a = right_pane(sys.argv[2])
print("[%s keys] content changed: %s" % (sys.argv[3], b != a))
if b != a:
    bl, al = b.split("\n"), a.split("\n")
    for i in range(max(len(bl),len(al))):
        x = bl[i] if i < len(bl) else ""
        y = al[i] if i < len(al) else ""
        if x != y:
            print("  first diff line %d: %r -> %r" % (i, x[:70], y[:70])); break
PY
  echo "[$target keys] footer link-peek before: $before_footer"
  echo "[$target keys] footer link-peek after:  $after_footer"
}

# ---------------------------------------------------------------------------
# 4c. SWEEP: click each browser-pane text-run start cell; detect navigation
#     via a content change; press Ctrl-d (back) to return to the index page
#     before the next click. Builds a link map {cell -> navigated? + dest}.
#     Runs for both targets and diffs the maps.
# ---------------------------------------------------------------------------
# Extract browser-pane CONTENT text-run start cells (0-based row,col) from a
# capture. Restricts to the area between the '││' separator and the trailing
# '│' border; skips divider rows (┄), the URL-bar row (/page/), and blank lines.
# One start cell per contiguous non-space run (links AND plain text; plain-text
# clicks won't navigate, which is exactly the discriminator).
extract_run_cells() {
  "$PY3" - "$1" <<'PY'
import re, sys, json
lines = open(sys.argv[1], "r", errors="replace").read().split("\n")
cells = []
for r, line in enumerate(lines):
    bare = re.sub(r'\x1b\[[0-9;]*m', '', line)
    if '││' not in bare:
        continue
    pane_off = bare.index('││') + 2
    rest = bare[pane_off:]
    # strip the trailing border '│' (and anything after) so border cells excluded
    if '│' in rest:
        rest = rest[:rest.index('│')]
    if '┄' in rest or '/page/' in rest:
        continue
    in_run = False; run_start = 0
    for c, ch in enumerate(rest):
        if ch not in (' ', ''):
            if not in_run:
                in_run = True; run_start = c
        else:
            if in_run:
                cells.append([r, pane_off + run_start]); in_run = False
    if in_run:
        cells.append([r, pane_off + run_start])
print(json.dumps(cells))
PY
}

# Extract the current page heading: the first non-empty content line in the
# browser pane that is not a divider or the URL bar. Used as the navigation
# signal — a click navigated iff the heading changed away from the index page.
page_heading() {
  "$PY3" - "$1" <<'PY'
import re, sys
for line in open(sys.argv[1], "r", errors="replace").read().split("\n"):
    b = re.sub(r'\x1b\[[0-9;]*m', '', line)
    if '││' not in b: continue
    rest = b.split('││', 1)[1]
    if '│' in rest: rest = rest[:rest.index('│')]
    t = rest.strip()
    if t and '┄' not in t and '/page/' not in t:
        print(t); break
PY
}

run_sweep() {
  local target="$1"
  local sess="pms-$target"
  local baseline="$OUT/${target}_sweep_baseline.txt"
  capture_plain "$sess" > "$baseline"
  local index_heading; index_heading="$(page_heading "$baseline")"
  local cells; cells="$(extract_run_cells "$baseline")"
  echo "[$target sweep] index_heading='$index_heading' $(echo "$cells" | "$PY3" -c 'import json,sys; print(len(json.load(sys.stdin)))') candidate cells" >&2

  local result="$OUT/${target}_linkmap.txt"
  : > "$result"
  echo "$cells" | "$PY3" -c '
import json, sys
for c in json.load(sys.stdin): print("%d,%d" % (c[0], c[1]))
' | while IFS=, read -r row col; do
    mouse_click "$sess" "$col" "$row"
    sleep 1.2
    capture_plain "$sess" > "$OUT/${target}_sweep_click.txt"
    local heading; heading="$(page_heading "$OUT/${target}_sweep_click.txt")"
    if [ -n "$heading" ] && [ "$heading" != "$index_heading" ]; then
      echo "$row,$col  LINK  dest=$heading" >> "$result"
      # Go back to the index page for the next click.
      tmux send-keys -t "$sess" C-d
      sleep 1.8
    else
      echo "$row,$col  plain" >> "$result"
    fi
  done
  echo "[$target sweep] link map -> $result" >&2
  cat "$result"
}

# ---------------------------------------------------------------------------
# 5. Run.
# ---------------------------------------------------------------------------
run_one() {
  local target="$1" rnscfg appcfg
  rnscfg="$OUT/rns-$target"; appcfg="$OUT/app-$target"
  make_rns_config "$rnscfg"; make_app_config "$appcfg"
  boot_target "$target" "$rnscfg" "$appcfg"
  case "$MODE" in
    probe) run_probe "$target";;
    sweep) run_sweep "$target";;
    keys)  run_keys "$target";;
  esac
  [ "$KEEP" = "1" ] || tmux kill-session -t "pms-$target" 2>/dev/null || true
}

if [ "$GO_ONLY" != "1" ]; then run_one py; fi
if [ "$PY_ONLY" != "1" ]; then run_one go; fi

# A/B diff for sweep: compare the two link maps (cell -> LINK/plain).
if [ "$MODE" = "sweep" ] && [ "$PY_ONLY" != "1" ] && [ "$GO_ONLY" != "1" ]; then
  echo
  echo "================ parity-mouse link-map diff ================"
  "$PY3" - "$OUT/py_linkmap.txt" "$OUT/go_linkmap.txt" <<'PY'
import sys
def parse(path):
    m = {}
    for line in open(path):
        parts = line.split()
        if len(parts) >= 2:
            m[parts[0]] = parts[1]  # "r,c" -> "LINK" or "plain"
    return m
py = parse(sys.argv[1]); go = parse(sys.argv[2])
py_links = sorted(c for c,v in py.items() if v == "LINK")
go_links = sorted(c for c,v in go.items() if v == "LINK")
print("py link cells: %d  go link cells: %d" % (len(py_links), len(go_links)))
only_py = sorted(set(py_links) - set(go_links))
only_go = sorted(set(go_links) - set(py_links))
if only_py: print("  links only Python found: %s" % only_py)
if only_go: print("  links only Go found: %s" % only_go)
if not only_py and not only_go and len(py_links) == len(go_links):
    print("  link regions MATCH")
else:
    print("  LINK-REGION DIFF")
PY
fi

echo "output dir: $OUT"
[ "$KEEP" = "1" ] && echo "(--keep: server + tmux sessions left running)" >&2