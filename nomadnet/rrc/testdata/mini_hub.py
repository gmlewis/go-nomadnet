#!/usr/bin/env python3
# A minimal RRC hub for CROSS-IMPLEMENTATION integration tests: hosts an
# rrc.hub destination over a loopback TCP interface using the INSTALLED
# nomadnet's RRC module (the same constants, envelope builder, and vendored
# cbor the real Python ecosystem uses), and speaks the protocol to any client
# that connects - Go or Python.
#
# Events are appended as JSON lines to --log so the Go test can assert on the
# hub's view of the exchange (the raw hello bytes, the decoded fields, the
# received messages).
#
# Run via the parity interpreter: python3 mini_hub.py --port N --log FILE

import argparse
import json
import os
import sys
import time

import RNS

sys_path = os.path.dirname(os.path.abspath(__file__))
sys_path_up = os.path.join(os.path.dirname(sys_path), "..", "..", "..")
for p in (sys_path, os.path.join(sys_path, "..", "..", "..", "..", "nomadnet")):
    if os.path.isdir(p):
        sys.path.insert(0, os.path.join(p, "..", "..", "..", ".."))

from nomadnet.vendor import cbor  # noqa: E402

# RRC protocol constants (RRC.py:47-56,68-90) - imported from the installed
# module so the hub speaks the exact wire format.
from nomadnet.RRC import (  # noqa: E402
    K_V, K_T, K_ID, K_TS, K_SRC, K_ROOM, K_BODY, K_NICK,
    T_HELLO, T_WELCOME, T_JOIN, T_JOINED, T_PART, T_PARTED,
    T_MSG, T_NOTICE, T_ACTION, T_PING, T_PONG, T_ERROR,
    B_WELCOME_HUB, B_WELCOME_VER, B_WELCOME_CAPS, B_WELCOME_LIMITS,
    L_MAX_NICK_BYTES, L_MAX_ROOM_NAME_BYTES, L_MAX_MSG_BODY_BYTES,
    L_MAX_ROOMS_PER_SESSION, L_RATE_LIMIT_MSGS_PER_MINUTE,
    CAP_RESOURCE_ENVELOPE, CAP_ACTION,
    RRC_VERSION,
)

ap = argparse.ArgumentParser()
ap.add_argument("--port", type=int, required=True)
ap.add_argument("--log", required=True)
ap.add_argument("--name", default="MiniHub")
args = ap.parse_args()

LOG = args.log


def emit(event):
    with open(LOG, "a") as f:
        f.write(json.dumps(event) + "\n")


os.makedirs(os.path.dirname(LOG), exist_ok=True)
cfgdir = os.path.join(os.path.dirname(LOG), "rns")
os.makedirs(cfgdir, exist_ok=True)
with open(os.path.join(cfgdir, "config"), "w") as f:
    f.write("[reticulum]\n  share_instance = No\n  enable_transport = No\n\n"
            "[logging]\n  loglevel = 4\n\n[interfaces]\n"
            "  [[interfaces/tcp]]\n    type = TCPServerInterface\n"
            "    enabled = yes\n    listen_ip = 127.0.0.1\n"
            "    listen_port = %d\n" % args.port)

reticulum = RNS.Reticulum(cfgdir)
identity = RNS.Identity()
dest = RNS.Destination(identity, RNS.Destination.IN, RNS.Destination.SINGLE, "rrc", "hub")


def make_env(msg_type, body=None, room=None):
    import time
    env = {K_V: 1, K_T: int(msg_type), K_ID: os.urandom(8),
           K_TS: int(round(time.time() * 1000)), K_SRC: identity.hash}
    if room is not None:
        env[K_ROOM] = room
    if body is not None:
        env[K_BODY] = body
    return env


def send_env(link, env):
    payload = cbor.encode(env)
    RNS.Packet(link, payload).send()
    with open(LOG, "a") as f:
        f.write(json.dumps({"event": "sent", "type": env.get(K_T),
                            "bytes": payload.hex()}) + "\n")


def on_packet(data, packet):
    try:
        return _on_packet(data, packet)
    except Exception as e:
        import traceback
        with open(LOG, "a") as f:
            f.write(json.dumps({"event": "handler-error", "error": str(e),
                                "trace": traceback.format_exc()}) + "\n")


def _on_packet(data, packet):
    try:
        env = cbor.decode(data)
    except Exception as e:
        with open(LOG, "a") as f:
            f.write(json.dumps({"event": "decode-failed", "error": str(e)}) + "\n")
        return
    with open(LOG, "a") as f:
        f.write(json.dumps({"event": "packet", "type": env.get(K_T),
                            "src-len": len(env.get(K_SRC, b"")) if isinstance(env.get(K_SRC), bytes) else None,
                            "body-types": {k: type(x).__name__ for k, x in (env.get(K_BODY) or {}).items()}
                            if isinstance(env.get(K_BODY), dict) else None}) + "\n")

    if env.get(K_T) == T_HELLO:
        body = env.get(K_BODY) or {}
        nick = env.get(K_NICK)
        if isinstance(nick, bytes):
            nick = nick.decode("utf-8", "replace")
        with open(LOG, "a") as f:
            f.write(json.dumps({"event": "hello", "name-type": type(body.get(0)).__name__,
                                "ver-type": type(body.get(1)).__name__,
                                "nick": nick}) + "\n")
        # The WELCOME reply, mirroring rrcd: hub name/version, capabilities,
        # and the protocol limits the client applies.
        welcome_body = {
            B_WELCOME_HUB: args.name,
            B_WELCOME_VER: "0.1",
            B_WELCOME_CAPS: {CAP_RESOURCE_ENVELOPE: True, CAP_ACTION: True},
            B_WELCOME_LIMITS: {L_MAX_NICK_BYTES: 32, L_MAX_ROOM_NAME_BYTES: 32,
                               L_MAX_MSG_BODY_BYTES: 1000, L_MAX_ROOMS_PER_SESSION: 16,
                               L_RATE_LIMIT_MSGS_PER_MINUTE: 30},
        }
        send_env(packet.link, make_env(T_WELCOME, welcome_body))
    elif env.get(K_T) == T_MSG:
        # Echo the message back as a NOTICE so the client can assert the
        # cross-implementation message delivery.
        room = env.get(K_ROOM)
        nick = env.get(K_NICK)
        text = env.get(K_BODY) or ""
        if isinstance(text, bytes):
            text = text.decode("utf-8", "replace")
        if isinstance(nick, bytes):
            nick = nick.decode("utf-8", "replace")
        with open(LOG, "a") as f:
            f.write(json.dumps({"event": "msg", "room": room, "nick": nick,
                                "text": text}) + "\n")
        notice = "echo: " + text
        packet_env = make_env(T_NOTICE, body=notice.encode("utf-8"), room=room)
        packet_env[K_NICK] = "MiniHub"
        send_env(packet.link, packet_env)


def link_established(link):
    with open(LOG, "a") as f:
        f.write(json.dumps({"event": "link-established"}) + "\n")
    link.set_packet_callback(on_packet)


dest.set_link_established_callback(link_established)

with open(LOG, "a") as f:
    f.write(json.dumps({"event": "hub", "hash": RNS.hexrep(dest.hash, delimit=False)}) + "\n")

while True:
    with open(LOG, "a") as f:
        f.write(json.dumps({"event": "announce", "dest-hash": RNS.hexrep(dest.hash, delimit=False),
                            "id-hash": RNS.hexrep(identity.hash, delimit=False)}) + "\n")
    print("ANNOUNCE dest-hash=" + RNS.hexrep(dest.hash, delimit=False), flush=True)
    dest.announce()
    time.sleep(5)
