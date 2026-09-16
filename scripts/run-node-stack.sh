#!/bin/bash -eu
#
# scripts/run-node-stack.sh — the one-stop launcher for a full Reticulum node
#
# Runs ONE shared go-reticulum transport (gornsd -s) for the whole stack,
# then launches the LXMF propagation daemon (golxmd), the RRC chat hub (gorrcd),
# the RRC bot client (gorrcbot), and finally runs the gonomadnet TUI in the
# FOREGROUND of this terminal.
#
# Stack Architecture:
#   1. gornsd -s   (Layer 3: Shared instance; owns ALL network interfaces:
#                   TCPServerInterface on port 4242, AutoInterface, LoRa, relays)
#   2. golxmd -p   (Layer 4: LXMF propagation node; attached to shared instance;
#                   stores & forwards offline LXMF messages across the mesh)
#   3. gorrcd      (Layer 4: RRC chat hub; attached to shared instance;
#                   serves persistent rooms and chat channels)
#   4. gorrcbot    (Layer 4: Autonomous RRC client; attached to shared instance;
#                   joins #general to provide offline field tools & diagnostics)
#   5. gonomadnet  (Layer 7: Nomad Network browser & node; runs in foreground)
#
# Usage:
#   ./scripts/run-node-stack.sh [options]
#
# Options:
#   --no-build     Skip rebuilding Go binaries; use existing binaries on PATH
#   --headless     Run gonomadnet as a daemon (-d) instead of interactive TUI
#   -h, --help     Show this help message
#
# Stop:
#   pkill -x gornsd gorrcd gorrcbot golxmd gonomadnet
#

EPOCH="$(date +%s)"
LOGDIR="${TMPDIR:-/tmp}"
NO_BUILD=0
HEADLESS=0

for arg in "$@"; do
    case "$arg" in
        --no-build)
            NO_BUILD=1
            ;;
        --headless)
            HEADLESS=1
            ;;
        -h|--help)
            sed -ne '/^#/!q;s/^# //;2,$p' "$0"
            exit 0
            ;;
        *)
            echo "Unknown option: $arg" >&2
            echo "Run '$0 --help' for usage." >&2
            exit 1
            ;;
    esac
done

# Resolve paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NOMAD_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
RETIC_DIR="$(cd "$NOMAD_DIR/../go-reticulum" 2>/dev/null && pwd || true)"

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
export GOTRACEBACK=all

find_bin() {
    local name="$1"
    if command -v "$name" >/dev/null 2>&1; then
        command -v "$name"
    elif [ -x "$HOME/go/bin/$name" ]; then
        echo "$HOME/go/bin/$name"
    else
        echo ""
    fi
}

echo "== [1/6] Stopping existing stack instances =="
pkill -x gonomadnet 2>/dev/null || true
pkill -f "gonomadnet.sh" 2>/dev/null || true
pkill -x nomadnet 2>/dev/null || true
pkill -f "python.*nomadnet" 2>/dev/null || true
pkill -x gorrcbot 2>/dev/null || true
pkill -x gorrcd 2>/dev/null || true
pkill -x golxmd 2>/dev/null || true
pkill -x gornsd 2>/dev/null || true

# Wait until every target process has exited
for _ in $(seq 1 30); do
    if ! pgrep -x "gornsd|gorrcd|gorrcbot|golxmd|gonomadnet|nomadnet" >/dev/null 2>&1; then
        break
    fi
    sleep 0.5
done

# Force-kill anything remaining
pkill -9 -x gonomadnet 2>/dev/null || true
pkill -9 -x nomadnet 2>/dev/null || true
pkill -9 -x gorrcbot 2>/dev/null || true
pkill -9 -x gorrcd 2>/dev/null || true
pkill -9 -x golxmd 2>/dev/null || true
pkill -9 -x gornsd 2>/dev/null || true

if pgrep -x "gornsd|gorrcd|gorrcbot|golxmd|gonomadnet|nomadnet" >/dev/null 2>&1; then
    echo "FATAL: some processes survived SIGKILL; inspect with: pgrep -af 'gornsd|gorrcd|gorrcbot|golxmd|gonomadnet'" >&2
    exit 1
fi

if [ "$NO_BUILD" -eq 0 ]; then
    echo "== [2/6] Building tools =="
    if [ -n "$RETIC_DIR" ] && [ -d "$RETIC_DIR/cmd" ]; then
        echo "Building go-reticulum daemons (gornsd, gorrcd, golxmd, gorrcbot)..."
        ( cd "$RETIC_DIR" && go install -tags=wago ./cmd/gornsd ./cmd/gorrcd ./cmd/golxmd ./cmd/gornstatus ) || {
            echo "FATAL: go-reticulum build failed" >&2
            exit 1
        }
        ( cd "$RETIC_DIR" && go install ./cmd/gorrcbot ) || {
            echo "FATAL: gorrcbot build failed" >&2
            exit 1
        }
    fi
    echo "Building gonomadnet..."
    ( cd "$NOMAD_DIR" && go install -tags=wago ./cmd/gonomadnet ) || {
        echo "FATAL: go-nomadnet build failed" >&2
        exit 1
    }
else
    echo "== [2/6] Skipping build (--no-build specified) =="
fi

GORN="$(find_bin gornsd)"
GOLXMD="$(find_bin golxmd)"
GORRCD="$(find_bin gorrcd)"
GORRCBOT="$(find_bin gorrcbot)"
GONOMADNET="$(find_bin gonomadnet)"
GORNSTATUS="$(find_bin gornstatus)"

if [ -z "$GORN" ] || [ -z "$GORRCD" ] || [ -z "$GONOMADNET" ]; then
    echo "FATAL: Required binaries missing. Ensure go-reticulum and go-nomadnet are built." >&2
    exit 1
fi

echo "== [3/6] Starting gornsd -s (shared instance; owns all interfaces) =="
nohup "$GORN" -s -v -v -pprof-addr 127.0.0.1:6062 \
    >"$LOGDIR/gornsd-$EPOCH.log" 2>&1 </dev/null &
GORN_PID=$!
disown "$GORN_PID" 2>/dev/null || true
echo "gornsd pid $GORN_PID, log $LOGDIR/gornsd-$EPOCH.log"

echo "Waiting for shared-instance socket..."
SOCKET_UP=no
for _ in $(seq 1 60); do
    if ! kill -0 "$GORN_PID" 2>/dev/null; then
        echo "FATAL: gornsd exited during startup; check $LOGDIR/gornsd-$EPOCH.log" >&2
        exit 1
    fi
    # Portable check: ss (Linux), lsof (macOS/Linux), or gornstatus query
    if command -v ss >/dev/null 2>&1 && ss -xa 2>/dev/null | grep -q "rns/default"; then
        SOCKET_UP=yes
        break
    elif [ -n "$GORNSTATUS" ] && "$GORNSTATUS" 2>/dev/null | grep -q "Shared Instance"; then
        SOCKET_UP=yes
        break
    elif command -v lsof >/dev/null 2>&1 && lsof -U 2>/dev/null | grep -q "rns/default"; then
        SOCKET_UP=yes
        break
    fi
    sleep 0.5
done

if [ "$SOCKET_UP" != yes ]; then
    echo "FATAL: the shared-instance socket never appeared; check $LOGDIR/gornsd-$EPOCH.log" >&2
    exit 1
fi

# Check fleet interface listening port (4242)
PORT_LISTENING=no
if command -v ss >/dev/null 2>&1 && ss -tln 2>/dev/null | grep -q ":4242"; then
    PORT_LISTENING=yes
elif command -v lsof >/dev/null 2>&1 && lsof -i :4242 2>/dev/null | grep -q LISTEN; then
    PORT_LISTENING=yes
elif command -v netstat >/dev/null 2>&1 && netstat -an 2>/dev/null | grep -q "4242.*LISTEN"; then
    PORT_LISTENING=yes
fi

if [ "$PORT_LISTENING" = yes ]; then
    echo "Fleet interface is listening on standard RNS port 4242"
else
    echo "NOTE: port 4242 not listening (may be client-only or using non-default interfaces in ~/.reticulum/config)"
fi

echo "== [4/6] Starting golxmd -p (LXMF propagation daemon) =="
if [ -n "$GOLXMD" ]; then
    nohup "$GOLXMD" -p -v \
        >"$LOGDIR/golxmd-$EPOCH.log" 2>&1 </dev/null &
    GOLXMD_PID=$!
    disown "$GOLXMD_PID" 2>/dev/null || true
    echo "golxmd pid $GOLXMD_PID, log $LOGDIR/golxmd-$EPOCH.log"
else
    echo "WARN: golxmd binary not found; skipping LXMF propagation daemon"
fi

echo "== [5/6] Starting gorrcd & gorrcbot (RRC chat hub and assistant) =="
nohup "$GORRCD" -pprof-addr 127.0.0.1:6061 -log-level DEBUG \
    >"$LOGDIR/gorrcd-$EPOCH.log" 2>&1 </dev/null &
GORRCD_PID=$!
disown "$GORRCD_PID" 2>/dev/null || true
echo "gorrcd pid $GORRCD_PID, log $LOGDIR/gorrcd-$EPOCH.log"

if [ -n "$GORRCBOT" ]; then
    if [ -f "$HOME/.gorrcbot/config.toml" ]; then
        nohup "$GORRCBOT" -pprof-addr 127.0.0.1:6063 -log-level NOTICE \
            >"$LOGDIR/gorrcbot-$EPOCH.log" 2>&1 </dev/null &
        GORRCBOT_PID=$!
        disown "$GORRCBOT_PID" 2>/dev/null || true
        echo "gorrcbot pid $GORRCBOT_PID, log $LOGDIR/gorrcbot-$EPOCH.log"
    else
        "$GORRCBOT" || true
        echo "gorrcbot initialized configuration in ~/.gorrcbot/config.toml (start manually or re-run)"
    fi
fi

echo
echo "== Reticulum Node Bootstrap Complete =="
echo "Logs : $LOGDIR/gornsd-$EPOCH.log"
[ -n "$GOLXMD" ] && echo "       $LOGDIR/golxmd-$EPOCH.log"
echo "       $LOGDIR/gorrcd-$EPOCH.log"
[ -n "$GORRCBOT" ] && [ -f "$HOME/.gorrcbot/config.toml" ] && echo "       $LOGDIR/gorrcbot-$EPOCH.log"
echo "       ~/.reticulum/logfile"
echo "pprof: gornsd 127.0.0.1:6062 · gorrcd 127.0.0.1:6061 · gorrcbot 127.0.0.1:6063 · gonomadnet 127.0.0.1:6060"
echo "Stop : pkill -x gornsd gorrcd gorrcbot golxmd gonomadnet"
echo

if [ "$HEADLESS" -eq 1 ]; then
    echo "== Starting gonomadnet in daemon mode (-d) =="
    nohup "$GONOMADNET" -d -pprof-addr 127.0.0.1:6060 \
        >"$LOGDIR/gonomadnet-$EPOCH.log" 2>&1 </dev/null &
    GONOMADNET_PID=$!
    disown "$GONOMADNET_PID" 2>/dev/null || true
    echo "gonomadnet daemon pid $GONOMADNET_PID, log $LOGDIR/gonomadnet-$EPOCH.log"
    echo "All daemons are running in the background."
    exit 0
fi

echo "== Starting gonomadnet in foreground (exit: Ctrl-Q or Esc) =="
exec "$GONOMADNET" -t -pprof-addr 127.0.0.1:6060 2>"$LOGDIR/gonomadnet-$EPOCH.log"
