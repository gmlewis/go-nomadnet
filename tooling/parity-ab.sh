#!/usr/bin/env bash
# -*- compile-command: "./parity-ab.sh --pages tooling/parity-fixtures/retibooks-page"; -*-
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# parity-ab.sh — local-loopback live A/B comparator for nomadnet ⇄ gonomadnet.
#
# Stands up a headless gonomadnet fixture node (cmd/serve-page) serving a page
# directory over a loopback TCP RNS interface, then launches BOTH the Python
# nomadnet client and the gonomadnet client in separate tmux PTYs, connects each
# to the served node by URL (Ctrl-u), and captures the rendered browser frame of
# the SAME page from both. The two captures are decoded with ansiview.py and
# diffed, surfacing full-TUI layout + chrome (menubar, list pane, browser frame,
# footer link hints) differences that the offline logical-line comparator
# (workflow B) cannot see. No real network; deterministic served content.
#
# This is workflow C in the parity skill, and the base for the mouse driver (D).
#
# Usage:
#   parity-ab.sh --pages <dir> [--size WxH] [--out DIR] [--boot SECS]
#                [--connect-wait SECS] [--port N] [--keep] [--py-only | --go-only]
#
# <dir> must contain index.mu (the page served at /page/index.mu, what both
# clients browse after Ctrl-u <hash>).
#
# Output: <out>/server.{out,err}, <out>/{py,go}_frame.txt (raw capture-pane -e),
# <out>/{py,go}.json (ansiview decode), and a diff report on stdout.

set -u

PAGES=""
SIZE="135x32"
OUT=""
BOOT=""
CONNECT_WAIT=""
PORT=""
KEEP=0
PY_ONLY=0
GO_ONLY=0

usage() { sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --pages)         PAGES="$2"; shift 2;;
    --size)          SIZE="$2"; shift 2;;
    --out)           OUT="$2"; shift 2;;
    --boot)          BOOT="$2"; shift 2;;
    --connect-wait)  CONNECT_WAIT="$2"; shift 2;;
    --port)          PORT="$2"; shift 2;;
    --keep)          KEEP=1; shift;;
    --py-only)       PY_ONLY=1; shift;;
    --go-only)       GO_ONLY=1; shift;;
    -h|--help)       usage 0;;
    *) echo "unknown arg: $1" >&2; usage 1;;
  esac
done

[ -n "$PAGES" ] || { echo "--pages <dir> is required (must contain index.mu)" >&2; exit 1; }
[ -f "$PAGES/index.mu" ] || { echo "$PAGES/index.mu not found" >&2; exit 1; }
[ "$PY_ONLY" = "1" ] && [ "$GO_ONLY" = "1" ] && { echo "--py-only and --go-only are mutually exclusive" >&2; exit 1; }

W="${SIZE%x*}"; H="${SIZE#*x}"
case "$W" in ''|*[!0-9]*) echo "bad width in --size $SIZE" >&2; exit 1;; esac
case "$H" in ''|*[!0-9]*) echo "bad height in --size $SIZE" >&2; exit 1;; esac

[ -n "$BOOT" ]         || BOOT=30
[ -n "$CONNECT_WAIT" ] || CONNECT_WAIT=15

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TS="$(date +%s)"
[ -n "$OUT" ] || OUT="/tmp/parity-ab-${TS}"
mkdir -p "$OUT"

ANSIVIEW="$REPO_ROOT/tooling/tui-parity/ansiview.py"
PY3="/opt/homebrew/bin/python3"
[ -x "$PY3" ] || PY3="python3"

# ---------------------------------------------------------------------------
# 1. Build + start the fixture node (cmd/serve-page).
# ---------------------------------------------------------------------------
SERVE_BIN="$OUT/serve-page"
echo "building serve-page..." >&2
( cd "$REPO_ROOT" && GOCACHE=/tmp/go-cache go build -o "$SERVE_BIN" ./cmd/serve-page ) || { echo "go build serve-page failed" >&2; exit 1; }

SERVE_PORT_ARG=""
[ -n "$PORT" ] && SERVE_PORT_ARG="-port $PORT"
"$SERVE_BIN" -pages "$PAGES" $SERVE_PORT_ARG > "$OUT/server.out" 2> "$OUT/server.err" &
SRV_PID=$!
cleanup() {
  [ "$KEEP" = "1" ] && return
  tmux kill-session -t parity-py 2>/dev/null || true
  tmux kill-session -t parity-go 2>/dev/null || true
  kill "$SRV_PID" 2>/dev/null || true
  wait "$SRV_PID" 2>/dev/null || true
}
trap cleanup EXIT

# Wait for the server to print NODE_HASH + PORT.
i=0
while [ $i -lt 50 ]; do
  [ -s "$OUT/server.out" ] && grep -q NODE_HASH "$OUT/server.out" && grep -q PORT "$OUT/server.out" && break
  sleep 0.2; i=$((i+1))
done
HASH="$(grep NODE_HASH "$OUT/server.out" | cut -d= -f2)"
SVPORT="$(grep PORT "$OUT/server.out" | cut -d= -f2)"
[ -n "$HASH" ] && [ -n "$SVPORT" ] || { echo "serve-page did not report NODE_HASH/PORT" >&2; cat "$OUT/server.out" >&2; cat "$OUT/server.err" >&2; exit 1; }
echo "fixture node: hash=$HASH port=$SVPORT" >&2

# ---------------------------------------------------------------------------
# 2. Per-client RNS configs (TCPClientInterface to the server port). Each client
#    needs its own RNS config dir so they do not share an identity.
# ---------------------------------------------------------------------------
make_rns_config() {
  local dir="$1"
  mkdir -p "$dir"
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

# Per-client nomadnet APP config dir. Pre-seeded with the full default nomadnet
# config (tooling/parity-fixtures/default-nomadnet.cfg, the verbatim default
# Python ships). A present config file means neither target treats this as a
# first run, so Python does not set firstrun=True and boots straight to
# Conversations (Main.py:27) instead of the first-run Guide — a deterministic
# start state for the keyboard connect sequence. (A partial hand-written config
# crashes nomadnet on missing keys like max_peers, so the full default is
# required.)
DEFAULT_CFG="$REPO_ROOT/tooling/parity-fixtures/default-nomadnet.cfg"
make_app_config() {
  local dir="$1"
  mkdir -p "$dir/pages" "$dir/files"
  cp "$DEFAULT_CFG" "$dir/config"
}

# ---------------------------------------------------------------------------
# 3. Launch + connect + capture for one target.
#    keys: Home,Up escapes to the menu; Right to Network (menu index 1); Enter
#    selects it; Down to the body, Right to the browser pane; Ctrl-u opens the
#    URL dialog; type the hash; Enter connects. (Same sequence the
#    test-gonomadnet-input-box harness validates for both targets.)
# ---------------------------------------------------------------------------
connect_and_capture() {
  local target="$1"   # py | go
  local rnscfg="$2"   appcfg="$3"
  local sess="parity-$target"
  local launch
  if [ "$target" = "py" ]; then
    launch="COLORTERM=truecolor nomadnet -t --config '$appcfg' --rnsconfig '$rnscfg'"
  else
    launch="cd '$REPO_ROOT' && COLORTERM=truecolor go run ./cmd/gonomadnet -t -config '$appcfg' -rnsconfig '$rnscfg'"
  fi

  local tmuxconf
  tmuxconf="$(mktemp /tmp/parity-ab-tmux-XXXXXX)"
  printf 'terminal-features RGB\n' > "$tmuxconf"
  tmux -f "$tmuxconf" new-session -d -x "$W" -y "$H" -s "$sess" "$launch; echo __EXIT=\$?; sleep 600"
  rm -f "$tmuxconf"

  # Boot: wait for the menu bar (Conversations) to appear.
  echo "[$target] booting (up to ${BOOT}s)..." >&2
  local b=0
  while [ $b -lt "$BOOT" ]; do
    if tmux capture-pane -t "$sess" -p 2>/dev/null | grep -q "Conversations"; then break; fi
    sleep 1; b=$((b+1))
  done
  tmux capture-pane -t "$sess" -p > "$OUT/${target}_boot.txt" 2>/dev/null

  echo "[$target] navigating to Network + Ctrl-u connect to $HASH" >&2
  tmux send-keys -t "$sess" Home
  sleep 0.5
  tmux send-keys -t "$sess" Up
  sleep 0.5
  tmux send-keys -t "$sess" Right
  sleep 0.5
  tmux send-keys -t "$sess" Enter
  sleep 1
  tmux send-keys -t "$sess" Down
  sleep 0.5
  tmux send-keys -t "$sess" Right
  sleep 0.5
  tmux send-keys -t "$sess" C-u
  sleep 1
  # Type the hash literally, then submit.
  tmux send-keys -t "$sess" -l "$HASH"
  sleep 0.5
  tmux send-keys -t "$sess" Enter

  echo "[$target] waiting ${CONNECT_WAIT}s for the page to render..." >&2
  sleep "$CONNECT_WAIT"

  tmux capture-pane -t "$sess" -p -e > "$OUT/${target}_frame.txt" 2>/dev/null
  tmux capture-pane -t "$sess" -p   > "$OUT/${target}_frame_plain.txt" 2>/dev/null
  tmux kill-session -t "$sess" 2>/dev/null || true
  echo "[$target] captured -> $OUT/${target}_frame.txt" >&2
}

# ---------------------------------------------------------------------------
# 4. Run both targets.
# ---------------------------------------------------------------------------
if [ "$GO_ONLY" != "1" ]; then
  PY_RNS="$OUT/rns-py"; PY_APP="$OUT/app-py"
  make_rns_config "$PY_RNS"; make_app_config "$PY_APP"
  connect_and_capture py "$PY_RNS" "$PY_APP"
fi
if [ "$PY_ONLY" != "1" ]; then
  GO_RNS="$OUT/rns-go"; GO_APP="$OUT/app-go"
  make_rns_config "$GO_RNS"; make_app_config "$GO_APP"
  connect_and_capture go "$GO_RNS" "$GO_APP"
fi

# ---------------------------------------------------------------------------
# 5. Decode + diff.
# ---------------------------------------------------------------------------
decode() {
  local target="$1"
  local frame="$OUT/${target}_frame.txt"
  [ -s "$frame" ] || { echo "[]" > "$OUT/${target}.json"; return; }
  # ansiview.py takes the capture file as a positional argument (no stdin).
  "$PY3" "$ANSIVIEW" --json "$frame" > "$OUT/${target}.json" 2>/dev/null \
    || echo "[]" > "$OUT/${target}.json"
}
if [ "$GO_ONLY" != "1" ]; then decode py; fi
if [ "$PY_ONLY" != "1" ]; then decode go; fi

echo
echo "================ parity-ab diff ================"
if [ "$PY_ONLY" != "1" ] && [ "$GO_ONLY" != "1" ]; then
  "$PY3" - "$OUT/py.json" "$OUT/go.json" <<'PY'
import json, re, sys
py = json.load(open(sys.argv[1]))
go = json.load(open(sys.argv[2]))
# ansiview.py --json emits a top-level list of {"row","text","styles"} rows.
pr, gr = py if isinstance(py, list) else py.get("rows", []), \
         go if isinstance(go, list) else go.get("rows", [])
print("py rows=%d go rows=%d" % (len(pr), len(gr)))

# Per-run noise classes that are NOT parity bugs. Normalizing them lets the
# comparator run autonomously hundreds of times without flagging values that
# legitimately differ every run:
#  - <hex> identity/LXMF/addr hashes: each client has its own fresh identity,
#    so the local-node info pane differs per target (not a port bug).
#  - "Announced : <relative time>": depends on capture moment, not rendering.
#  - transfer footer: byte count, elapsed seconds, bandwidth all vary per run.
HASH     = re.compile(r"<[0-9a-fA-F]{16,}>")
ANNOUNCED = re.compile(r"(Announced\s*:)\s*.+")
XFER      = re.compile(r"(\d+(\.\d+)?)\s*(B|b/s|Kb/s|Mb/s|Gb/s)|in\s+\d+(\.\d+)?s")
def norm(t):
    t = HASH.sub("<H>", t)
    t = ANNOUNCED.sub(r"\1 <t>", t)
    t = XFER.sub("<x>", t)
    return t

n = max(len(pr), len(gr))
real, noise = 0, 0
for i in range(n):
    pt = pr[i].get("text","") if i < len(pr) else ""
    gt = gr[i].get("text","") if i < len(gr) else ""
    if pt == gt:
        continue
    if norm(pt) == norm(gt):
        noise += 1
        print("row %d noise (normalized equal)" % i)
        continue
    real += 1
    print("row %d REAL DIFF" % i)
    print("  py: %r" % pt[:120])
    print("  go: %r" % gt[:120])
print("--- %d real diff(s), %d noise row(s) ---" % (real, noise))
sys.exit(1 if real else 0)
PY
  rc=$?
  echo "parity-ab: exit $rc (nonzero = real rendering/layout diffs found)"
else
  echo "(single-target run — no A/B diff)"
fi
echo "output dir: $OUT"
[ "$KEEP" = "1" ] && echo "(--keep: server + tmux sessions left running)" >&2