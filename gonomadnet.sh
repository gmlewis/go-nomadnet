#!/bin/bash -ex
# Build + run gonomadnet with live pprof and full-goroutine-stack capture on
# SIGQUIT. Portable across macOS / Linux / aarch64 Jetson / Crostini Penguin:
# does not depend on the caller's CWD or on $GOPATH/bin being on $PATH.
#
# Filename convention: ...-kill-QUIT-<epoch>.log is the file a later
# `kill -QUIT <pid>` writes the GOTRACEBACK=all goroutine dump into (stderr is
# redirected here); the script itself never sends the signal.
cd "$(dirname "$0")"
# -tags=wago links the in-process wasm page sandbox: .wasm files in
# ~/.nomadnetwork/storage/pages/ render as executable pages (dynamic Micron
# markup) instead of being served statically. On platforms the wago runtime
# does not support, the tag is harmlessly ignored and gonomadnet falls back
# to the zero-overhead static-serving stub.

# Always hand the terminal back, however gonomadnet ends.
#
# gonomadnet does this itself on every exit path it can reach: tui.RestoreTerminal
# writes the sequence below and puts back the terminal's line settings, from
# Ctrl-Q, the Quit menu item, a signal, or a panic in any goroutine the client
# wraps. What it cannot reach is a panic in a goroutine it does not wrap
# (go-reticulum's own transports and timers) or a SIGKILL — the process is gone
# before any of its code runs. Those are what this trap is for: without it the
# terminal is left in raw mode, on the alternate screen, still reporting the
# mouse, so the shell is unusable until the user works out that `reset` fixes it,
# and every mouse movement prints escape-sequence garbage first.
#
# The settings are the ones the script recorded before the client was started,
# not a generic `stty sane`, and the codes are the client's own — the same list,
# in the same order, as tui.TerminalRestoreSeq(). They are checked against it by
# cmd/gonomadnet/terminal-restore-trap_test.go, so the two cannot drift apart.
#
# Everything is written to /dev/tty, the terminal the client draws on, and every
# step stands down rather than failing when there is no terminal at all (a
# headless run, where the client falls back to daemon mode and takes no
# terminal). It is idempotent: harmless on a clean exit.
saved_stty=$(stty -g </dev/tty 2>/dev/null || true)
terminal_restore_seq=$'\033[?2026l\033[?1000l\033[?1002l\033[?1003l\033[?1005l\033[?1006l\033[?1015l\033[?1004l\033[?2004l\033[?9001l\033[<u\033[>4;0m\033[?7h\033[?1l\033>\033[?1049l\033[?25h\033[0m'
restore_terminal() {
    [ -n "$saved_stty" ] && stty "$saved_stty" </dev/tty 2>/dev/null || true
    printf '%s' "$terminal_restore_seq" >/dev/tty 2>/dev/null || true
}
# Capture the real exit status BEFORE the trap's own commands overwrite $?, then
# re-`exit` with it at the end. Without this, bash adopts the status of the
# trap's last command as the script's exit status, masking any upstream failure
# (e.g. `go install`) and reporting success.
trap 'rc=$?; restore_terminal; exit $rc' EXIT

go install -tags=wago ./cmd/gonomadnet

# If any "gonomadnet -d" daemon is running, temporarily stop it so the headed
# TUI below doesn't fight it over the RNS shared instance (@rns/default) and
# other on-disk state. "Temporarily" means systemctl stop — the unit stays
# enabled, so the daemon comes back at the next reboot (or a manual start).
# A bare kill is NOT enough: rr-fleet-gonomadnet.service has Restart=always,
# so systemd would just relaunch it 5s later.
#
# Detection matches the daemon's own argv (".../gonomadnet -d"); the (^|/)
# anchor keeps it from matching unrelated command lines that merely contain
# the string (editor greps, shell wrappers). pgrep is guarded because a
# non-match exits 1 and the shebang's -e would abort the script on it.
daemon_pids=$(pgrep -f '(^|/)gonomadnet -d' || true)
if [ -n "$daemon_pids" ]; then
    echo "gonomadnet -d daemon running (pids: $(echo "$daemon_pids" | tr '\n' ' '))"
    unit=$(systemctl list-units --type=service --no-legend --no-pager 2>/dev/null \
        | awk '$1 ~ /gonomadnet/ && $4 == "running" {print $1; exit}')
    if [ -n "$unit" ]; then
        # The unit is a system service (User=glenn but managed by PID 1), so
        # stopping it needs root; sudo prompts for a password interactively.
        if ! sudo systemctl stop "$unit"; then
            echo "ERROR: failed to stop $unit — daemon still running; aborting so" >&2
            echo "the headed instance doesn't fight the daemon. Start it again later." >&2
            exit 1
        fi
        echo "stopped $unit (stays stopped until next reboot; unit remains enabled)"
    fi
    # Anything not under systemd (manually launched daemons) won't respond to
    # systemctl stop — SIGTERM it directly and wait for it to release state.
    daemon_pids=$(pgrep -f '(^|/)gonomadnet -d' || true)
    if [ -n "$daemon_pids" ]; then
        kill $daemon_pids 2>/dev/null || true
        i=0
        while pgrep -f '(^|/)gonomadnet -d' >/dev/null && [ $i -lt 10 ]; do
            sleep 1; i=$((i+1))
        done
        if pgrep -f '(^|/)gonomadnet -d' >/dev/null; then
            echo "ERROR: daemon ignored SIGTERM; refusing to SIGKILL it (may corrupt state)." >&2
            exit 1
        fi
        echo "stopped manually-launched daemon(s)"
    fi
    # Give the kernel a moment to release the @rns/default socket before the
    # headed instance binds it.
    sleep 1
fi

env -u NO_COLOR TERM=xterm-256color COLORTERM=truecolor TCELL_TRUECOLOR=1 \
GOTRACEBACK=all "$(go env GOPATH)/bin/gonomadnet" -pprof-addr 127.0.0.1:6060 \
    2>"gonomadnet-$(hostname -s)-kill-QUIT-$(date +%s).log"
