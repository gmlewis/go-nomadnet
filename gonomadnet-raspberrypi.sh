#!/bin/bash -eu
#
# gonomadnet-raspberrypi.sh — the one-stop launcher for the raspberrypi:
# rebuilds the latest tools, runs ONE shared go-reticulum transport
# (gornsd -s) for the whole stack, then serves the RRC chat hub (gorrcd) and
# finally runs the gonomadnet TUI in the FOREGROUND of this terminal.
#
#   gornsd -s    (the shared instance; owns ALL interfaces: the fleet
#                 TCPServerInterface on the standard RNS port 4242, the public
#                 relays, LoRa, AutoInterface)
#   gorrcd       (attached to the shared instance; serves the RRC chat hub —
#                 its announces propagate via gornsd to the whole fleet and
#                 the mesh, so clients reach the hub over the STABLE fleet
#                 link instead of multi-hop relay paths)
#   gonomadnet   (attached to the shared instance; the TUI runs in the
#                 foreground — wrap this script in your own tmux/screen if
#                 you want it to survive the ssh session)
#
# Why this architecture: when gorrcd and gonomadnet run their own RNS
# instances, whichever starts first grabs the fleet interface and the other
# is left with no direct path — client links then ride unstable multi-hop
# public-relay paths, the keepalive machinery tears half-dead links down,
# and every reconnect/re-join spams the rooms. With gornsd -s as the
# permanent shared instance, the roles never flip and every path to the hub
# rides the stable fleet link.
#
# Usage:  gonomadnet-raspberrypi.sh   (no tmux is created or attached by this
#         script — run it inside your own tmux session for persistence)
# Logs:   /tmp/<service>-<epoch-seconds>.log
# pprof:  gornsd 127.0.0.1:6062 · gorrcd 127.0.0.1:6061 · gonomadnet 127.0.0.1:6060
# Stop:   pkill -x gornsd gorrcd gonomadnet   (Ctrl-C stops the foreground TUI)

EPOCH="$(date +%s)"
LOGDIR=/tmp
GORN="$HOME/go/bin/gornsd"
GORRCD="$HOME/go/bin/gorrcd"
GONOMADNET="$HOME/go/bin/gonomadnet"
RETIC_DIR="$HOME/go/src/github.com/gmlewis/go-reticulum"
NOMAD_DIR="$HOME/go/src/github.com/gmlewis/go-nomadnet"

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
export GOTRACEBACK=all

echo "== [1/4] killing existing gonomadnet / nomadnet / gorrcd / gornsd =="
# gonomadnet first (its name is a substring of nothing else), then the
# Python nomadnet, then the hub, then the shared instance. The gonomadnet.sh
# wrapper (bash -ex ./gonomadnet.sh) dies with its child.
pkill -x gonomadnet 2>/dev/null || true
pkill -f "gonomadnet.sh" 2>/dev/null || true
pkill -x nomadnet 2>/dev/null || true
pkill -f "python.*nomadnet" 2>/dev/null || true
pkill -x gorrcd 2>/dev/null || true
pkill -x gornsd 2>/dev/null || true

# Wait until every target process is REALLY gone. Starting gornsd while the
# previous instance still holds the instance lock (or the shared-instance
# socket) makes the new gornsd exit with "already running" and leaves the
# stack half-up.
for _ in $(seq 1 30); do
    if ! pgrep -x "gornsd|gorrcd|gonomadnet|nomadnet" >/dev/null 2>&1; then
        break
    fi
    sleep 0.5
done
# Force-kill anything that ignored SIGTERM.
pkill -9 -x gonomadnet 2>/dev/null || true
pkill -9 -x nomadnet 2>/dev/null || true
pkill -9 -x gorrcd 2>/dev/null || true
pkill -9 -x gornsd 2>/dev/null || true
if pgrep -x "gornsd|gorrcd|gonomadnet|nomadnet" >/dev/null 2>&1; then
    echo "FATAL: some processes survived SIGKILL; resolve manually with pgrep -af 'gornsd|gorrcd|gonomadnet'"
    exit 1
fi

echo "== [2/4] building + installing the latest tools =="
git -C "$RETIC_DIR" pull --ff-only 2>/dev/null || echo "WARN: go-reticulum pull failed; building the local checkout"
( cd "$RETIC_DIR" && go install ./cmd/gornsd ./cmd/gorrcd ) || { echo "FATAL: go-reticulum build failed"; exit 1; }
git -C "$NOMAD_DIR" pull --ff-only 2>/dev/null || echo "WARN: go-nomadnet pull failed; building the local checkout"
( cd "$NOMAD_DIR" && go install ./cmd/gonomadnet ) || { echo "FATAL: go-nomadnet build failed"; exit 1; }

echo "== [3/4] starting gornsd -s (the shared instance; owns all interfaces) =="
nohup "$GORN" -s -v -v -pprof-addr 127.0.0.1:6062 \
    >"$LOGDIR/gornsd-$EPOCH.log" 2>&1 </dev/null &
GORN_PID=$!
disown "$GORN_PID" 2>/dev/null || true
echo "gornsd pid $GORN_PID, log $LOGDIR/gornsd-$EPOCH.log"

echo "== waiting for gornsd and its shared-instance socket =="
SOCKET_UP=no
for _ in $(seq 1 60); do
    # Check liveness FIRST: a gornsd that exits (e.g. refusing to start
    # because another instance still holds the lock) must fail loudly here,
    # never be masked by a socket some dying process still holds.
    if ! kill -0 "$GORN_PID" 2>/dev/null; then
        echo "FATAL: gornsd exited during startup; check $LOGDIR/gornsd-$EPOCH.log"
        exit 1
    fi
    if ss -xa 2>/dev/null | grep -q "rns/default"; then
        SOCKET_UP=yes
        break
    fi
    sleep 0.5
done
if [ "$SOCKET_UP" != yes ]; then
    echo "FATAL: the shared-instance socket never appeared; check the gornsd log"
    exit 1
fi
if ss -tln 2>/dev/null | grep -q ":4242"; then
    echo "the fleet interface is listening on the standard RNS port 4242"
else
    echo "WARN: nothing is listening on port 4242; check the [interfaces]"
    echo "      TCPServerInterface entry in ~/.reticulum/config"
fi

echo "== [4/4] starting gorrcd (attached; serves the RRC chat hub) =="
nohup "$GORRCD" -pprof-addr 127.0.0.1:6061 -log-level DEBUG \
    >"$LOGDIR/gorrcd-$EPOCH.log" 2>&1 </dev/null &
GORRCD_PID=$!
disown "$GORRCD_PID" 2>/dev/null || true
echo "gorrcd pid $GORRCD_PID, log $LOGDIR/gorrcd-$EPOCH.log"

echo
echo "== bootstrap complete =="
echo "logs : /tmp/gornsd-$EPOCH.log"
echo "       /tmp/gorrcd-$EPOCH.log"
echo "       ~/.reticulum/logfile (gornsd's RNS-level log)"
echo "pprof: gornsd 127.0.0.1:6062 · gorrcd 127.0.0.1:6061"
echo
echo "watch the hub log : tail -f /tmp/gorrcd-$EPOCH.log"
echo "hub destination   : bc0a90a0a1799ae7c07fd461ea6b09f0 (rrc.hub)"
echo "stop gornsd+gorrcd: pkill -x gornsd gorrcd"
echo
echo "== starting gonomadnet in the foreground (exit: Ctrl-Q; this"
echo "   terminal is yours to wrap in tmux first) =="

# Hand the terminal to the TUI. Without a TTY gonomadnet falls back to
# daemon mode on its own. stderr carries panic/GOTRACEBACK dumps into the
# epoch-stamped /tmp log.
exec "$GONOMADNET" -t -pprof-addr 127.0.0.1:6060 2>"$LOGDIR/gonomadnet-$EPOCH.log"
