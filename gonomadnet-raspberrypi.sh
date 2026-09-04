#!/bin/bash -eu
#
# gonomadnet-raspberrypi.sh — the one-stop launcher for the raspberrypi:
# rebuilds the latest tools, runs ONE shared go-reticulum transport
# (gornsd -s) for the whole stack, then serves the RRC chat hub (gorrcd) and
# shows the gonomadnet TUI live in this terminal.
#
#   gornsd -s    (the shared instance; owns ALL interfaces: the fleet
#                 TCPServerInterface on the standard RNS port 4242, the public
#                 relays, LoRa, AutoInterface)
#   gorrcd       (attached to the shared instance; serves the RRC chat hub —
#                 its announces propagate via gornsd to the whole fleet and
#                 the mesh, so clients reach the hub over the STABLE fleet
#                 link instead of multi-hop relay paths)
#   gonomadnet   (attached to the shared instance; the TUI, attached to this
#                 terminal at the end — detach with Ctrl-B D and everything
#                 keeps running; re-attach with: tmux attach -t gonomadnet)
#
# Why this architecture: when gorrcd and gonomadnet run their own RNS
# instances, whichever starts first grabs the fleet interface and the other
# is left with no direct path — client links then ride unstable multi-hop
# public-relay paths, the keepalive machinery tears half-dead links down,
# and every reconnect/re-join spams the rooms. With gornsd -s as the
# permanent shared instance, the roles never flip and every path to the hub
# rides the stable fleet link.
#
# Usage:  gonomadnet-raspberrypi.sh   (run interactively over ssh; the
#         gonomadnet TUI attaches to this terminal at the end)
# Logs:   /tmp/<service>-<epoch-seconds>.log
# pprof:  gornsd 127.0.0.1:6062 · gorrcd 127.0.0.1:6061 · gonomadnet 127.0.0.1:6060
# Stop:   pkill -x gornsd gorrcd; tmux kill-session -t gonomadnet

EPOCH="$(date +%s)"
LOGDIR=/tmp
GORN="$HOME/go/bin/gornsd"
GORRCD="$HOME/go/bin/gorrcd"
GONOMADNET="$HOME/go/bin/gonomadnet"
RETIC_DIR="$HOME/go/src/github.com/gmlewis/go-reticulum"
NOMAD_DIR="$HOME/go/src/github.com/gmlewis/go-nomadnet"
HUB_HASH_FILE="$HOME/.rrcd/hub_identity"

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
export GOTRACEBACK=all

echo "== [1/5] installing tmux (for the gonomadnet TUI session) =="
if ! command -v tmux >/dev/null 2>&1; then
    sudo apt-get install -y tmux >/dev/null 2>&1 || {
        echo "WARN: tmux install failed; gonomadnet will run in daemon mode"
        echo "      (headless — it will NOT auto-join #test4 until the room is"
        echo "       selected through a connected client)."
    }
fi
TMUX_OK=$(command -v tmux >/dev/null 2>&1 && echo yes || echo no)

echo "== [2/5] killing existing gonomadnet / nomadnet / gorrcd / gornsd =="
# gonomadnet first (its name is a substring of nothing else), then the
# Python nomadnet, then the hub, then the shared instance. The gonomadnet.sh
# wrapper (bash -ex ./gonomadnet.sh) dies with its child.
pkill -x gonomadnet 2>/dev/null || true
pkill -f "gonomadnet.sh" 2>/dev/null || true
pkill -x nomadnet 2>/dev/null || true
pkill -f "python.*nomadnet" 2>/dev/null || true
pkill -x gorrcd 2>/dev/null || true
pkill -x gornsd 2>/dev/null || true
sleep 2

echo "== [3/5] building + installing the latest tools =="
git -C "$RETIC_DIR" pull --ff-only 2>/dev/null || echo "WARN: go-reticulum pull failed; building the local checkout"
( cd "$RETIC_DIR" && go install ./cmd/gornsd ./cmd/gorrcd ) || { echo "FATAL: go-reticulum build failed"; exit 1; }
git -C "$NOMAD_DIR" pull --ff-only 2>/dev/null || echo "WARN: go-nomadnet pull failed; building the local checkout"
( cd "$NOMAD_DIR" && go install ./cmd/gonomadnet ) || { echo "FATAL: go-nomadnet build failed"; exit 1; }

echo "== [4/5] starting gornsd -s (the shared instance; owns all interfaces) =="
nohup "$GORN" -s -pprof-addr 127.0.0.1:6062 \
    >"$LOGDIR/gornsd-$EPOCH.log" 2>&1 </dev/null &
GORN_PID=$!
disown "$GORN_PID" 2>/dev/null || true
echo "gornsd pid $GORN_PID, log $LOGDIR/gornsd-$EPOCH.log"

echo "== waiting for the shared-instance socket and the fleet port =="
SOCKET_UP=no
for _ in $(seq 1 30); do
    if ss -x 2>/dev/null | grep -q "rns/default"; then
        SOCKET_UP=yes
        break
    fi
    if ! kill -0 "$GORN_PID" 2>/dev/null; then
        echo "FATAL: gornsd exited during startup; check $LOGDIR/gornsd-$EPOCH.log"
        exit 1
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

echo "== [5/5] starting gorrcd (attached; serves the RRC chat hub) =="
nohup "$GORRCD" -pprof-addr 127.0.0.1:6061 \
    >"$LOGDIR/gorrcd-$EPOCH.log" 2>&1 </dev/null &
GORRCD_PID=$!
disown "$GORRCD_PID" 2>/dev/null || true
echo "gorrcd pid $GORRCD_PID, log $LOGDIR/gorrcd-$EPOCH.log"

if [ "$TMUX_OK" = yes ]; then
    echo "== starting gonomadnet (the TUI in a detached tmux session) =="
    tmux kill-session -t gonomadnet 2>/dev/null || true
    tmux new-session -d -s gonomadnet -e TERM=xterm-256color \
        "$GONOMADNET -t -pprof-addr 127.0.0.1:6060 2>$LOGDIR/gonomadnet-$EPOCH.log; \
         bash --norc --noprofile"
    echo "gonomadnet TUI running in tmux session 'gonomadnet'"
else
    echo "== starting gonomadnet (daemon mode; tmux unavailable) =="
    nohup "$GONOMADNET" -d -pprof-addr 127.0.0.1:6060 \
        >"$LOGDIR/gonomadnet-$EPOCH.log" 2>&1 </dev/null &
    GONOMADNET_PID=$!
    disown "$GONOMADNET_PID" 2>/dev/null || true
    echo "gonomadnet pid $GONOMADNET_PID (daemon mode — select the room from a"
    echo "connected client to bring this node into #test4)"
fi

echo
echo "== bootstrap complete =="
echo "logs : /tmp/gornsd-$EPOCH.log"
echo "       /tmp/gorrcd-$EPOCH.log"
echo "       /tmp/gonomadnet-$EPOCH.log"
echo "       ~/.reticulum/logfile (gornsd's RNS-level log)"
echo "pprof: gornsd 127.0.0.1:6062 · gorrcd 127.0.0.1:6061 · gonomadnet 127.0.0.1:6060"
echo
echo "watch the hub log : tail -f /tmp/gorrcd-$EPOCH.log"
echo "watch fleet paths : gornpath 28c7c1a68c735693aa8e6b8193ed44b2"
echo "stop everything   : pkill -x gornsd gorrcd; tmux kill-session -t gonomadnet"
echo

# Attach the gonomadnet TUI to THIS terminal so the user watches the room
# live. Detach with Ctrl-B D: the TUI keeps running in its tmux session, the
# ssh session closes cleanly, and everything else stays up.
if [ "$TMUX_OK" = yes ] && [ -t 0 ]; then
    echo "== attaching the gonomadnet TUI (detach: Ctrl-B then D) =="
    exec tmux attach-session -t gonomadnet
elif [ "$TMUX_OK" = yes ]; then
    echo "non-interactive session; attach the TUI later with: tmux attach -t gonomadnet"
fi
exit 0