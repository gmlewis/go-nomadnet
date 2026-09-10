#!/bin/bash -ex
# Build + run gonomadnet with live pprof and full-goroutine-stack capture on
# SIGQUIT. Portable across macOS / Linux / aarch64 Jetson / Crostini Penguin:
# does not depend on the caller's CWD or on $GOPATH/bin being on $PATH.
#
# Filename convention: ...-kill-QUIT-<epoch>.log is the file a later
# `kill -QUIT <pid>` writes the GOTRACEBACK=all goroutine dump into (stderr is
# redirected here); the script itself never sends the signal.
cd "$(dirname "$0")"
go install ./cmd/gonomadnet

# Always restore the terminal when gonomadnet exits — including an abrupt crash
# in a non-event-loop goroutine (transport callback, ticker, the draw drainer).
# tview restores the tty ONLY for panics in its own event-loop goroutine
# (application.go defer-recover-Fini); a crash anywhere else leaves the
# terminal in raw mode + the alternate screen, spewing escape-sequence garbage
# and forcing a manual `reset`. This EXIT trap runs regardless of how the
# process ends (clean quit, panic in any goroutine, signal kill), so the user
# never has to `reset` by hand. It is idempotent: harmless on a clean exit.
#   stty sane              -> restore cooked termios (undo tcell MakeRaw)
#   \033[?1049l            -> leave the alternate screen (discard its garbage,
#                            restore the primary screen + its scrollback)
#   \033[?25h              -> unhide the cursor (tcell hides it on engage)
#   \033[0m                -> reset SGR attributes
# Capture the real exit status BEFORE the trap's own commands overwrite $?,
# then re-`exit` with it at the end. Without this, bash adopts the status of the
# trap's last command (the printf, which succeeds) as the script's exit status,
# masking any upstream failure (e.g. `go install`) and reporting success.
trap 'rc=$?; stty sane 2>/dev/null; printf "\033[?1049l\033[?25h\033[0m"; exit $rc' EXIT

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
