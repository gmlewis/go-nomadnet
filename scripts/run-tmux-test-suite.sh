#!/usr/bin/env bash
# -*- compile-command: "./scripts/run-tmux-test-suite.sh --headed"; -*-
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# run-tmux-test-suite.sh — a "playwright-style" end-to-end test of the Go
# NomadNet TUI (go run ./cmd/gonomadnet) driven entirely by tmux remote-control
# keystrokes against a live instance, so you can watch the whole scripted
# sequence run in a real terminal.
#
# What it does (in order):
#   1. Visit every main-menu command in order (Conversations, Network, Channels,
#      Log, Interfaces, Config, Guide) — everything except Quit — and snapshot
#      each rendered page.
#   2. Select Network, DownArrow into the body, then Ctrl-L to switch to the
#      Announce Stream and watch for announcing nodes.
#   3. Wait for announcing nodes, open one (Enter -> Announce Info), choose the
#      "Connect" button (Right, Enter), wait for its index.mu to render in the
#      Remote Node pane, then Right into the browser, follow a couple of links,
#      and return to the node list. Repeat for up to 7 nodes (skipping any that
#      fail to render and trying the next, exactly as requested).
#   4. Go to the Guide menu, walk each of the 12 topics in order: select the
#      topic, move to the right panel, scroll to the bottom, go back to the left
#      panel, select the next topic. Each new topic should re-render from row 0;
#      this script captures the reader right after each topic change (before any
#      manual scroll reset) so the KNOWN scroll-not-reset bug
#      (tui/guide.go showTopic/renderMarkup has no ScrollTo(0,0)) is reproduced
#      and visible in the log. It then presses Home (which DOES scroll to the top)
#      and re-captures, so the log contrasts the buggy state vs. the correct one.
#   5. Navigate to the Quit menu item and activate it to end the test.
#
# Everything — every keystroke, every wait, every captured screen snapshot — is
# logged to /tmp/gonomadnet-tmux-test-suite-NNN.log where NNN is the number of
# seconds since the epoch at launch. The log is structured so an agent (or a
# human) can replay the same keystroke sequence and pinpoint any bug found.
#
# Usage:
#   ./scripts/run-tmux-test-suite.sh            # run detached; log to /tmp/...
#   ./scripts/run-tmux-test-suite.sh --headed   # attach the tmux session so you
#                                               # can watch it live in this shell
#   ./scripts/run-tmux-test-suite.sh --copy-config   # isolate a copy of your real
#                                               # config so it isn't modified
#   ./scripts/run-tmux-test-suite.sh --fresh          # empty config (boots to the
#                                               # first-run Guide; no live nodes)
#   ./scripts/run-tmux-test-suite.sh --config DIR     # use config dir DIR directly
#
# Options:
#   --headed              Attach the tmux session to this terminal so the test
#                         is watchable. (Default: detached; you may attach
#                         yourself with the printed command.)
#   --copy-config         Copy $GNOMADNET_CONFIG (default ~/.nomadnetwork) to a
#                         temp dir and run against the copy, so your real config
#                         and storage are not modified by the test. Recommended
#                         unless you specifically want to mutate the real config.
#   --fresh               Run with an empty config dir (first-run Guide boot).
#   --config DIR          Use config directory DIR as-is (no copy).
#   --rnsconfig DIR       RNS config dir passed as -rnsconfig.
#   --announce-wait SECS  Seconds to wait for announcing nodes before connecting
#                         (default 25). Env: GNOMADNET_ANNOUNCE_WAIT.
#   --connect-wait SECS   Seconds to wait for a node page to render (default 12).
#                         Env: GNOMADNET_CONNECT_WAIT.
#   --node-iters N        Number of node connect iterations (default 7).
#                         Env: GNOMADNET_NODE_ITERATIONS.
#   --step-delay SECS     Seconds between keystrokes (default 0.4).
#                         Env: GNOMADNET_STEP_DELAY.
#   -h, --help            Show this help.
#
# Requires: tmux, go, and (for live nodes) a populated ~/.nomadnetwork config or
# a copied one. The TUI launches standalone (RNS initializes in the background),
# so the menu-tour and Guide phases work even with no network; the Network
# connect phase only succeeds if announcing nodes are actually reachable.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

HEADED=0
COPY_CONFIG=0
FRESH=0
CONFIG_DIR="${GNOMADNET_CONFIG:-$HOME/.nomadnetwork}"
RNS_CONFIG="${GNOMADNET_RNSCONFIG:-}"
ANNOUNCE_WAIT="${GNOMADNET_ANNOUNCE_WAIT:-25}"
CONNECT_WAIT="${GNOMADNET_CONNECT_WAIT:-12}"
NODE_ITERATIONS="${GNOMADNET_NODE_ITERATIONS:-7}"
STEP_DELAY="${GNOMADNET_STEP_DELAY:-0.4}"

usage() { sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --headed)        HEADED=1; shift;;
    --copy-config)   COPY_CONFIG=1; shift;;
    --fresh)         FRESH=1; shift;;
    --config)        CONFIG_DIR="$2"; shift 2;;
    --rnsconfig)     RNS_CONFIG="$2"; shift 2;;
    --announce-wait) ANNOUNCE_WAIT="$2"; shift 2;;
    --connect-wait)  CONNECT_WAIT="$2"; shift 2;;
    --node-iters)    NODE_ITERATIONS="$2"; shift 2;;
    --step-delay)    STEP_DELAY="$2"; shift 2;;
    -h|--help)       usage 0;;
    *) echo "unknown arg: $1" >&2; usage 1;;
  esac
done

command -v tmux >/dev/null 2>&1 || { echo "tmux not found on PATH" >&2; exit 1; }
command -v go  >/dev/null 2>&1 || { echo "go not found on PATH"  >&2; exit 1; }

NNN="$(date +%s)"
LOG="/tmp/gonomadnet-tmux-test-suite-${NNN}.log"
SESSION="gonet-${NNN}"
: > "$LOG"   # truncate/create the log

# In non-headed mode, mirror everything to the terminal AND the log. In headed
# mode the terminal is reserved for `tmux attach`, so the test driver writes only
# to the log; the preamble/summary use both() to reach both.
if [ "$HEADED" -eq 0 ]; then
  exec > >(tee -a "$LOG") 2>&1
fi

# both(): print a line to the terminal and (in headed mode) also to the log.
both() {
  printf '%s\n' "$*"
  [ "$HEADED" -eq 1 ] && printf '%s\n' "$*" >> "$LOG"
}

# log(): test-progress line. Goes to stdout, which is the tee (non-headed) or the
# redirected driver subshell (headed) — i.e. always into the log.
log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }

# --- tmux driving helpers -----------------------------------------------------
# capture(): current visible pane content (plain text, no color escapes).
capture() { tmux capture-pane -t "$SESSION" -p 2>/dev/null; }

# snapshot(): dump the current pane into the log with a label.
snapshot() {
  local t
  t="$(capture)"
  printf '===== SNAPSHOT: %s =====\n' "$*"
  printf '%s\n' "$t"
  printf '===== END SNAPSHOT =====\n'
}

# send KEYS...: send tmux key names (Up/Down/Left/Right/Enter/Home/End/PageUp/
# PageDown/Escape/Space/C-l/...) then pause STEP_DELAY.
send() { tmux send-keys -t "$SESSION" "$@"; sleep "$STEP_DELAY"; }
# sendl TEXT: send literal text (no tmux key-name interpretation).
sendl() { tmux send-keys -t "$SESSION" -l "$1"; sleep "$STEP_DELAY"; }

# wait_for PATTERN TIMEOUT [label]: poll the pane until PATTERN is present.
wait_for() {
  local pat="$1" timeout="$2" label="${3:-$1}" elapsed=0
  while [ "$elapsed" -lt "$timeout" ]; do
    if capture | grep -qF -- "$pat"; then
      log "wait_for: found ${label} after ${elapsed}s"
      return 0
    fi
    sleep 1; elapsed=$((elapsed + 1))
  done
  log "wait_for: TIMEOUT (${timeout}s) waiting for ${label}"
  return 1
}
# wait_gone PATTERN TIMEOUT [label]: poll the pane until PATTERN is absent.
wait_gone() {
  local pat="$1" timeout="$2" label="${3:-$1}" elapsed=0
  while [ "$elapsed" -lt "$timeout" ]; do
    if ! capture | grep -qF -- "$pat"; then
      log "wait_gone: ${label} gone after ${elapsed}s"
      return 0
    fi
    sleep 1; elapsed=$((elapsed + 1))
  done
  log "wait_gone: TIMEOUT (${timeout}s) waiting for ${label} to disappear"
  return 1
}

# alive(): is the tmux session still present?
alive() { tmux has-session -t "$SESSION" 2>/dev/null; }

step() { log ""; log "########## $* ##########"; }

# --- launch configuration -----------------------------------------------------
TEMP_CONFIG=""
cleanup() {
  if alive; then
    log "cleanup: killing tmux session $SESSION"
    tmux kill-session -t "$SESSION" 2>/dev/null || true
  fi
  [ -n "$TEMP_CONFIG" ] && rm -rf "$TEMP_CONFIG"
}
trap 'cleanup' EXIT

RNS_FLAGS=""
[ -n "$RNS_CONFIG" ] && RNS_FLAGS=" -rnsconfig '$RNS_CONFIG'"

if [ "$FRESH" -eq 1 ]; then
  # Empty config dir: the app self-creates defaults and boots the first-run Guide.
  TEMP_CONFIG="$(mktemp -d "${TMPDIR:-/tmp}/gonet-fresh-XXXXXX")"
  CONFIG_DIR="$TEMP_CONFIG"
  BOTH_FRESH_MSG="fresh config (first-run Guide); no live nodes will be available"
elif [ "$COPY_CONFIG" -eq 1 ]; then
  if [ -d "$CONFIG_DIR" ]; then
    TEMP_CONFIG="$(mktemp -d "${TMPDIR:-/tmp}/gonet-copy-XXXXXX")"
    cp -a "$CONFIG_DIR/." "$TEMP_CONFIG/" 2>/dev/null || true
    CONFIG_DIR="$TEMP_CONFIG"
    BOTH_FRESH_MSG="copied your real config ($CONFIG_DIR) to an isolated temp dir"
  else
    BOTH_FRESH_MSG="config dir '$CONFIG_DIR' not found; using an empty config"
    TEMP_CONFIG="$(mktemp -d "${TMPDIR:-/tmp}/gonet-empty-XXXXXX")"
    CONFIG_DIR="$TEMP_CONFIG"
  fi
else
  BOTH_FRESH_MSG="REAL config $CONFIG_DIR (may be modified by the test)"
fi

# tcell truecolor is robust regardless of $TERM.
export COLORTERM=truecolor

LAUNCH_CMD="cd '$REPO_ROOT' && exec go run ./cmd/gonomadnet -t -config '$CONFIG_DIR'$RNS_FLAGS"

both "gonomadnet tmux test suite"
both "  log     : $LOG"
both "  session : $SESSION  (watch live: tmux attach -t $SESSION)"
both "  config  : $BOTH_FRESH_MSG"
both "  repo    : $REPO_ROOT"
both "  flags   : announce-wait=${ANNOUNCE_WAIT}s connect-wait=${CONNECT_WAIT}s node-iters=${NODE_ITERATIONS} step-delay=${STEP_DELAY}s headed=${HEADED}"
both "  tmux    : $(tmux -V 2>/dev/null || echo unknown)"
both ""

# Create the detached tmux session at a fixed 135x32 (the size the app is tuned
# for) with window-size manual so attaching (or the client) cannot resize it.
tmux new-session -d -s "$SESSION" -n main -x 135 -y 32 -- bash -lc "$LAUNCH_CMD" \
  || { both "FATAL: tmux new-session failed"; exit 1; }
tmux set-option -t "$SESSION" -w window-size manual 2>/dev/null || true
tmux resize-window -t "$SESSION" -x 135 -y 32 2>/dev/null || true

# --- the scripted test sequence -----------------------------------------------
run_tests() {
  log "waiting for the app to start (go compile + intro splash)..."
  # The main display is up once the menu bar is visible.
  if ! wait_for "Conversations" 150 "main display / menu bar"; then
    log "FATAL: app did not start within 150s"
    snapshot "app-start-timeout"
    return 1
  fi
  sleep 1
  snapshot "app started (main display)"

  # ---- Phase 1: tour every main-menu command except Quit --------------------
  step "Phase 1: tour main-menu commands (Conversations..Guide, no Quit)"
  # Boot focus is in the body (Conversations list). Go to the top of that list,
  # then Up escapes to the menu (bodyListAtTop -> FocusMenu). The active page is
  # Conversations (menu index 0), so after this we are on the leftmost button.
  send Home
  send Up
  snapshot "menu reached (on Conversations, index 0)"
  # Enter activates the focused button and KEEPS focus in the menu, so we can
  # Right/Enter through all pages from here.
  # Menu order: 0 Conversations 1 Network 2 Channels 3 Log 4 Interfaces
  # 5 Config 6 Guide 7 Quit. Visit indices 0..6.
  send Enter
  snapshot "page: Conversations"
  send Right
  send Enter
  snapshot "page: Network"
  send Right
  send Enter
  snapshot "page: Channels"
  send Right
  send Enter
  snapshot "page: Log"
  send Right
  send Enter
  snapshot "page: Interfaces"
  send Right
  send Enter
  snapshot "page: Config"
  send Right
  send Enter
  snapshot "page: Guide"
  # We are now in the menu on Guide (index 6). (Quit is index 7; left for later.)

  # ---- Phase 2: Network, Down, Ctrl-L -> Announce Stream --------------------
  step "Phase 2: select Network, Down into body, Ctrl-L -> Announce Stream"
  # From Guide (6) go back to Network (1): five Lefts.
  send Left
  send Left
  send Left
  send Left
  send Left
  send Enter
  snapshot "Network selected (Saved Nodes)"
  send Down     # menu Down drops focus to the body
  send C-l      # toggle Saved Nodes <-> Announce Stream
  snapshot "after Ctrl-L (Announce Stream)"
  log "waiting up to ${ANNOUNCE_WAIT}s for announcing nodes to populate..."
  sleep "$ANNOUNCE_WAIT"
  snapshot "Announce Stream after announce-wait"

  # ---- Phase 3: connect to up to NODE_ITERATIONS nodes ----------------------
  step "Phase 3: connect to nodes, render index.mu, follow links (x${NODE_ITERATIONS})"
  # We are focused on the Announce Stream list (a pileFiller of
  # [tab bar(0), filter bar(1), list(2)]; focus is on the list).
  success=0
  attempts=0
  max_attempts=$((NODE_ITERATIONS * 4))
  while [ "$success" -lt "$NODE_ITERATIONS" ] && [ "$attempts" -lt "$max_attempts" ]; do
    attempts=$((attempts + 1))
    log "iteration: success=$success/$NODE_ITERATIONS attempts=$attempts/$max_attempts"
    if ! alive; then log "session died mid-Phase-3; aborting Phase 3"; break; fi

    # Open the Announce Info for the current list entry.
    send Enter
    if ! wait_for "Connect" 6 "Announce Info Connect button"; then
      log "  no node-type Announce Info opened (empty list or pn/peer entry); skip"
      # Close any detail that may have opened (pn/peer have no Connect button).
      send Escape
      snapshot "phase3 skip (no Connect)"
      send Down
      sleep 1
      continue
    fi
    snapshot "phase3: announce info opened"
    # Button row is [Back, Connect, Msg Op, Save] with focus on Back; Right -> Connect.
    send Right
    send Enter
    log "  Connect pressed; waiting for index.mu to render (Disconnected gone)..."
    if wait_gone "Disconnected" "$CONNECT_WAIT" "browser Disconnected state"; then
      log "  page RENDERED OK"
      snapshot "phase3: node index.mu rendered"
    else
      log "  connect FAILED/timeout (Disconnected still present); skip to next node"
      snapshot "phase3: connect failed (disconnected)"
      # Focus is already back on the list (connectToNode -> showAnnounceStream).
      send Down
      sleep 2
      continue
    fi

    # Move to the browser (Remote Node) pane and follow a couple of links.
    send Right
    sleep 1
    snapshot "phase3: browser pane (before link nav)"
    for link in 1 2; do
      send Down
      sleep 0.5
      send Enter
      log "  followed link $link"
      sleep 2
      snapshot "phase3: after following link $link"
    done
    # Release focus back to the node list (Left at a line's start releases; the
    # last followed link loaded a fresh page whose cursor is at line start, so a
    # few Lefts reliably hand focus back to the Announce Stream list).
    send Left
    send Left
    send Left
    sleep 0.5
    snapshot "phase3: back at node list"
    success=$((success + 1))
    log "  success $success/$NODE_ITERATIONS"
    send Down   # advance to the next node for the next iteration
  done
  log "Phase 3 complete: $success/$NODE_ITERATIONS successful connects in $attempts attempts"

  # ---- Phase 4: Guide — walk all 12 topics, scroll each to bottom ------------
  step "Phase 4: Guide — select each topic, move right, scroll to bottom"
  if ! alive; then log "session died before Phase 4"; return 1; fi
  # From the Announce Stream list, escape up to the menu: list->filter bar->tab
  # bar->menu (pileFiller onUpEscape). Four Ups is enough; extras are no-ops once
  # in the menu.
  send Up
  send Up
  send Up
  send Up
  snapshot "menu reached from Announce Stream (active page = Network, index 1)"
  # Active page is Network (index 1). Go Right 5 to Guide (index 6), Enter, then
  # Down drops to the body (topic list, focused on item 0).
  send Right
  send Right
  send Right
  send Right
  send Right
  send Enter
  snapshot "Guide selected"
  send Down
  snapshot "Guide topic list (item 0, Introduction)"

  # Topic list navigation auto-renders each topic on highlight change
  # (gd.topics.SetChangedFunc -> showTopic). showTopic does NOT reset the reader
  # scroll to row 0 (tui/guide.go: showTopic/renderMarkup has no ScrollTo) — a
  # known bug. We exercise it and capture the reader immediately after each
  # topic change, then press Home (which DOES ScrollToBeginning) and capture
  # again, so the log contrasts buggy vs. correct.
  #
  # Topic 0: select via Enter (AddItem selected callback). Then for each topic,
  # scroll the reader to the bottom, go back to the topics list, move Down to
  # the next topic (which auto-renders it), move right to the reader, and capture.
  send Enter
  snapshot "Guide topic 0 (Introduction) selected"
  send Right
  snapshot "Guide topic 0 reader (top, before scroll) [BUG CHECK: should be at row 0]"
  send End
  sleep 0.5
  snapshot "Guide topic 0 reader scrolled to bottom"
  send Left
  # Topics 1..11: Down auto-renders the next topic (bug: scroll not reset), then
  # capture, then scroll to bottom, capture, back to list.
  topic=1
  while [ "$topic" -le 11 ]; do
    send Down
    sleep 0.3
    send Right
    sleep 0.3
    snapshot "Guide topic $topic reader right after selection (NOT scrolled) [BUG CHECK: should be at row 0; bug leaves it at the previous topic's offset]"
    send Home
    sleep 0.3
    snapshot "Guide topic $topic reader after Home (correct: scrolled to top)"
    send End
    sleep 0.5
    snapshot "Guide topic $topic reader scrolled to bottom"
    send Left
    topic=$((topic + 1))
  done
  log "Phase 4 complete (12 topics walked)"

  # ---- Phase 5: Quit --------------------------------------------------------
  step "Phase 5: navigate to Quit and select it"
  if ! alive; then log "session already gone before Phase 5"; return 0; fi
  # We are on the topic list at item 11. Up to item 0 (11 Ups), then one more Up
  # escapes to the menu (Guide topic list Up-at-0 -> FocusMenu). Send 13 Ups to
  # be safe; extra Ups in the menu are no-ops.
  i=0
  while [ "$i" -lt 13 ]; do send Up; i=$((i + 1)); done
  snapshot "menu reached from Guide (active page = Guide, index 6)"
  # Right to Quit (index 7), Enter to activate -> onQuit -> graceful shutdown.
  send Right
  snapshot "cursor on Quit (index 7)"
  send Enter
  log "Quit activated; waiting for the app to exit..."
  # The app exits and the tmux pane's process ends -> the session closes.
  elapsed=0
  while [ "$elapsed" -lt 15 ]; do
    if ! alive; then log "app exited cleanly after ${elapsed}s"; break; fi
    sleep 1; elapsed=$((elapsed + 1))
  done
  if alive; then
    log "WARN: app did not exit within 15s of Quit; sending Ctrl-Q as a fallback"
    send C-q
    sleep 3
  fi
  snapshot "final state"
  log "run_tests finished"
}

# --- run it -------------------------------------------------------------------
if [ "$HEADED" -eq 1 ]; then
  # Driver writes only to the log; the foreground is reserved for the attach so
  # you can watch the live session.
  ( run_tests ) >> "$LOG" 2>&1 &
  DRIVER_PID=$!
  both "attaching tmux session so you can watch (detach with Ctrl-b d to leave it running)..."
  both "progress is being logged to: $LOG"
  both ""
  tmux attach -t "$SESSION"
  # Attach returns when the app quits (the window closes).
  wait "$DRIVER_PID" 2>/dev/null || true
else
  both "running detached. Watch live in another terminal with:"
  both "  tmux attach -t $SESSION"
  both "progress is being logged to: $LOG"
  both ""
  run_tests
fi

both ""
both "================ DONE ================"
both "log: $LOG"
both "session: $SESSION"
both "to replay/reproduce: relaunch the script (same flags) and follow the"
both "  keystroke sequence recorded in the log (every send/wait/snapshot is"
both "  timestamped)."
if alive; then
  both "(tmux session still alive; it will be killed on exit.)"
fi
# cleanup runs via the EXIT trap.