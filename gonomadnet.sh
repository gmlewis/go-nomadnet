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
GOTRACEBACK=all exec "$(go env GOPATH)/bin/gonomadnet" -pprof-addr 127.0.0.1:6060 \
    2>"gonomadnet-$(hostname -s)-kill-QUIT-$(date +%s).log"
