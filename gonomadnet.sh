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

GOTRACEBACK=all "$(go env GOPATH)/bin/gonomadnet" -pprof-addr 127.0.0.1:6060 \
    2>"gonomadnet-$(hostname -s)-kill-QUIT-$(date +%s).log"
