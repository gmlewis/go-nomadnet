#!/usr/bin/env bash
# -*- compile-command: "tooling/record-cast.sh --target go --out go_session-003.cast"; -*-
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# record-cast.sh — asciinema .cast recorder that forces 24-bit truecolor for
# BOTH the source-of-truth Python nomadnet (urwid) and the Go port
# (`gonomadnet`, tview/tcell), so palette *color values* can be diffed directly
# between the two .cast recordings with no 256-vs-truecolor colormode artifact.
#
# This is the recorder counterpart to parse_screencast.py: it produces the .cast
# files that `parse_screencast.py --diff` compares. See tooling/README.md.
#
# Why forcing is needed (and how each target is forced):
#
#   Python nomadnet emits whichever colormode its [textui] "colormode" config
#   selects. urwid's set_terminal_properties is AUTHORITATIVE — it does NOT
#   consult the terminal's advertised color capability — so a config with
#   colormode=256 records 256-color SGR (38;5;N) EVEN inside a truecolor
#   terminal. The existing python_session.cast is 256-color for exactly this
#   reason (an older nomadnet wrote colormode=256 as the default). Forcing:
#   seed the config with colormode=24bit so Python emits 38;2;r;g;b.
#
#   The Go port (tcell) emits truecolor when the terminfo entry has the RGB
#   capability (truecolor terminals like ghostty do). tcell ALSO honors
#   COLORTERM=truecolor: when set, LookupTerminfo fabricates the 38;2;r;g;b
#   escapes even for a 256-color terminfo entry (tcell terminfo.go). Forcing:
#   export COLORTERM=truecolor so Go truecolor is robust regardless of $TERM.
#   (Go's own config colormode also defaults to 24bit.)
#
# To preserve your real node identity + discovered peers (so the Network page
# actually populates after Ctrl-L), record-cast.sh COPIES your real nomadnet
# config dir (default ~/.nomadnetwork) to a temp dir, forces colormode=24bit in
# the copy's [textui] section, and runs the app with --config <temp>. Your real
# config is never modified. Pass --fresh to start from an empty config instead
# (both targets then self-create their default config — which already uses
# colormode=24bit — and boot the first-run Guide).
#
# Usage:
#   record-cast.sh --target orig|go [--out FILE.cast] [--config DIR] [--fresh]
#                  [--force] [--bin PATH] [--idle SECS] [--title TITLE]
#                  [--extra ARGS...]
#
# Examples (mirror the existing casts, now truecolor-forced):
#   # Python  (was: asciinema rec python_session.cast --command "nomadnet")
#   tooling/record-cast.sh --target orig --out python_session.cast
#
#   # Go port  (was: asciinema rec go_session-002.cast --command "./gonomadnet.sh")
#   tooling/record-cast.sh --target go --out go_session-003.cast --force
#
#   # First-run Guide (empty config), Go, truecolor:
#   tooling/record-cast.sh --target go --out guide_session.cast --fresh
#
# Then compare color directly (no colormode caveat):
#   python3 tooling/parse_screencast.py --diff python_session.cast go_session-003.cast
#
# Interact with the app, then quit it (Go: menu → Quit, or Ctrl-Q; Python: the
# Quit menu item, or Ctrl-C) to end the recording; asciinema writes the .cast on
# exit. The recorded BYTES are truecolor regardless of whether the live terminal
# can render truecolor — asciinema records what the program emits, not what the
# terminal displays — so the .cast is directly comparable even if you record in
# a 256-color terminal (the live preview will just look approximated).
#
# Requires: asciinema (`asciinema rec`), python3 (stdlib only), and either
# nomadnet on PATH (--target orig) or a buildable repo (--target go).

set -u

TARGET=""
OUT=""
CONFIG=""
FRESH=0
FORCE=0
BIN=""
IDLE=""
TITLE=""
EXTRA=()

usage() { sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --target) TARGET="$2"; shift 2;;
    --out)    OUT="$2"; shift 2;;
    --config) CONFIG="$2"; shift 2;;
    --fresh)  FRESH=1; shift;;
    --force)  FORCE=1; shift;;
    --bin)    BIN="$2"; shift 2;;
    --idle)   IDLE="$2"; shift 2;;
    --title)  TITLE="$2"; shift 2;;
    --extra)  EXTRA+=("$2"); shift 2;;
    -h|--help) usage 0;;
    *) echo "unknown arg: $1" >&2; usage 1;;
  esac
done

[ "$TARGET" = "orig" ] || [ "$TARGET" = "go" ] || { echo "--target orig|go is required" >&2; exit 1; }

command -v asciinema >/dev/null 2>&1 || { echo "asciinema not found on PATH; install it (e.g. brew install asciinema)" >&2; exit 1; }

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
[ -n "$OUT" ] || OUT="${TARGET}_session.cast"

# Real nomadnet config dir to copy from (default ~/.nomadnetwork).
REAL_CONFIG="${CONFIG:-$HOME/.nomadnetwork}"

TEMP_CONFIG=""
RUN_WRAPPER=""
cleanup() {
  [ -n "$TEMP_CONFIG" ] && rm -rf "$TEMP_CONFIG"
  [ -n "$RUN_WRAPPER" ] && rm -f "$RUN_WRAPPER"
}
trap cleanup EXIT

TEMP_CONFIG="$(mktemp -d "${TMPDIR:-/tmp}/record-cast-${TARGET}-XXXXXX")"

# force_colormode ensures [textui] colormode=24bit in the INI config file $1, in
# place, preserving comments and all other lines (no configparser reformatting).
force_colormode() {
  python3 - "$1" <<'PY'
import sys
p = sys.argv[1]
try:
    with open(p) as f:
        lines = f.read().splitlines()
except FileNotFoundError:
    lines = []
out = []
in_tui = False
seen_textui = False
done = False
for ln in lines:
    s = ln.strip()
    if s.startswith('[') and s.endswith(']'):
        if in_tui and not done:
            out.append('colormode = 24bit')
            done = True
        in_tui = (s == '[textui]')
        if in_tui:
            seen_textui = True
        out.append(ln)
        continue
    if (in_tui and not ln.lstrip().startswith('#') and '=' in s
            and s.split('=', 1)[0].strip().lower() == 'colormode'):
        out.append('colormode = 24bit')
        done = True
        continue
    out.append(ln)
if in_tui and not done:
    out.append('colormode = 24bit')
    done = True
if not seen_textui:
    if out and out[-1].strip():
        out.append('')
    out.append('[textui]')
    out.append('colormode = 24bit')
with open(p, 'w') as f:
    f.write('\n'.join(out) + '\n')
PY
}

if [ "$FRESH" = "1" ]; then
  # Empty config dir; both targets self-create defaults (colormode=24bit) and
  # boot the first-run Guide. Do NOT pre-seed a config file — nomadnet only sets
  # firstrun=True (and shows the Guide) when the config file is ABSENT.
  :
else
  if [ -d "$REAL_CONFIG" ]; then
    # Copy the real config (identity, storage, directory) so the Network page can
    # populate from live RNS announces. cp -a "src/." "dst/" copies the contents
    # (incl. dotfiles) into the existing temp dir. The real config is untouched.
    cp -a "$REAL_CONFIG/." "$TEMP_CONFIG/"
    if [ -f "$TEMP_CONFIG/config" ]; then
      force_colormode "$TEMP_CONFIG/config"
    fi
  else
    echo "note: real config dir '$REAL_CONFIG' not found; using fresh config" >&2
  fi
fi

# Resolve the run command for the target.
if [ "$TARGET" = "orig" ]; then
  if [ -z "$BIN" ] && ! command -v nomadnet >/dev/null 2>&1; then
    echo "nomadnet not found on PATH. Install it, or pass --bin /path/to/nomadnet" >&2
    exit 1
  fi
  [ -z "$BIN" ] && BIN="nomadnet"
  RUN_CMD="\"$BIN\" --config \"$TEMP_CONFIG\" ${EXTRA[*]:-}"
else
  if [ -n "$BIN" ]; then
    RUN_CMD="\"$BIN\" -t -config \"$TEMP_CONFIG\" ${EXTRA[*]:-}"
  else
    RUN_CMD="go run ./cmd/gonomadnet -t -config \"$TEMP_CONFIG\" ${EXTRA[*]:-}"
  fi
fi

# A small wrapper so asciinema --command never has to parse shell metacharacters:
# asciinema execs "bash <wrapper>", and the wrapper cds + execs the app.
RUN_WRAPPER="$(mktemp "${TMPDIR:-/tmp}/record-cast-run-XXXXXX.sh")"
cat > "$RUN_WRAPPER" <<EOF
#!/usr/bin/env bash
cd "$REPO_ROOT"
exec $RUN_CMD
EOF
chmod +x "$RUN_WRAPPER"

# Force Go (tcell) truecolor: tcell fabricates 38;2;r;g;b when COLORTERM is
# truecolor/24bit, regardless of the terminfo entry. (Python ignores this; its
# colormode is config-authoritative — already forced above.)
export COLORTERM=truecolor

ASC_ARGS=(rec "$OUT" --command "bash $RUN_WRAPPER")
if [ "$FORCE" = "1" ]; then
  ASC_ARGS+=(--overwrite)
elif [ -e "$OUT" ]; then
  echo "refusing to overwrite existing '$OUT'; pass --force to overwrite" >&2
  exit 1
fi
[ -n "$IDLE" ]  && ASC_ARGS+=(--idle-time-limit "$IDLE")
[ -n "$TITLE" ] && ASC_ARGS+=(--title "$TITLE")

if [ "$FRESH" = "1" ]; then
  echo "recording -> $OUT  (target=$TARGET, fresh config -> first-run Guide)"
  echo "  COLORTERM=$COLORTERM  (Python colormode: nomadnet self-creates 24bit; Go forced via COLORTERM)"
else
  echo "recording -> $OUT  (target=$TARGET, config=$TEMP_CONFIG)"
  echo "  COLORTERM=$COLORTERM  (Python colormode forced to 24bit in the config copy; Go forced via COLORTERM)"
fi
echo "  interact, then quit the app to stop the recording."
echo

asciinema "${ASC_ARGS[@]}"
RC=$?

echo
echo "recorded $OUT  (asciinema exit $RC)"
# Verify the colormode of the resulting cast's emitted SGR.
if [ -f "$OUT" ]; then
  python3 - "$OUT" <<'PY'
import json, sys
fn = sys.argv[1]
tc = cc = False
with open(fn, errors="replace") as f:
    for line in f:
        try:
            d = json.loads(line)
        except Exception:
            continue
        if isinstance(d, list) and d[1] == 'o':
            if "38;2;" in d[2] or "48;2;" in d[2]:
                tc = True
            if "38;5;" in d[2] or "48;5;" in d[2]:
                cc = True
print(f"  cast colormode: {'truecolor (38;2;)' if tc else 'NO truecolor SGR seen'}"
      + ("  + 256-color SGR also present" if cc and tc else
         "  (256-color only)" if cc and not tc else ""))
PY
  echo "  compare with: python3 tooling/parse_screencast.py --diff <py.cast> $OUT"
fi