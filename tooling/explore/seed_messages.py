#!/usr/bin/env python3
# Seed deterministic LXMF conversations into a nomadnet / gonomadnet storage
# tree so the differential explorer can drive message flows OFFLINE. Uses the
# installed Python RNS/LXMF to build REAL encrypted messages: each seeded
# message is encrypted to the target's own identity, so each target's seed
# carries its own ciphertext of the SAME plaintext — both implementations
# render identical content, and hash/timing normalization absorbs the
# per-target peer identities.
#
# Seeded state (count = 3, the default):
#   conv 1: one unread received message from an unknown peer (+ "unread" flag)
#   conv 2: one read received message from a TRUSTED, NAMED peer — the trusted
#           entry (written straight to the msgpack directory file, the format
#           both implementations read) also makes the Trusted tab non-empty so
#           keyboard tab navigation aligns across implementations
#   conv 3: one sent (originator) message to an unknown peer (+ "failed" flag)
#
# Run under the parity interpreter (the one that can import RNS and LXMF).
#
# Usage (from explore.py, not by hand):
#   python3 seed_messages.py --identity <seed>/storage/identity \
#       --conversations <seed>/storage/conversations \
#       --directory <seed>/storage/directory --count 3

import argparse
import os
import sys

import RNS
import LXMF
# The vendored u-msgpack the Python stack itself uses (Directory.py:7).
import RNS.vendor.umsgpack as msgpack

# Python DirectoryEntry trust levels (Directory.py:410-413).
TRUSTED = 0xFF


def hexrep(data):
    return RNS.hexrep(data, delimit=False)


def write_conv_flags(conv_dir, flags):
    for name, value in flags.items():
        with open(os.path.join(conv_dir, name), "w") as f:
            f.write(str(value))


def write_directory(directory_path, entries):
    """Write the msgpack directory file in Python's layout (Directory.py
    save_to_disk): {"entry_list": [(source_hash, display_name, trust_level,
    hosts_node, preferred_delivery, identify, sort_rank, notes)],
    "announce_stream": [...]}. The Go port reads the same file."""
    packed = [(h, name, trust, False, 0, False, None, "")
              for (h, name, trust) in entries]
    with open(directory_path, "wb") as f:
        f.write(msgpack.packb({"entry_list": packed, "announce_stream": []}))


def main():
    ap = argparse.ArgumentParser(description="Seed LXMF conversations for the differential explorer")
    ap.add_argument("--identity", required=True, help="the seed's storage/identity file")
    ap.add_argument("--conversations", required=True, help="the seed's storage/conversations dir")
    ap.add_argument("--directory", default="", help="the seed's storage/directory file (optional)")
    ap.add_argument("--count", type=int, default=3, help="number of conversations to seed (max 3 shapes)")
    args = ap.parse_args()

    own = RNS.Identity.from_file(args.identity)
    if own is None:
        sys.exit("could not load identity from %s" % args.identity)
    own_delivery = RNS.Destination(own, RNS.Destination.OUT, RNS.Destination.SINGLE,
                                   "lxmf", "delivery")

    def received_message(text):
        peer = RNS.Identity()
        peer_delivery = RNS.Destination(peer, RNS.Destination.OUT,
                                        RNS.Destination.SINGLE, "lxmf", "delivery")
        lxm = LXMF.LXMessage(own_delivery, peer_delivery, text)
        lxm.pack()
        conv_dir = os.path.join(args.conversations, hexrep(peer_delivery.hash))
        os.makedirs(conv_dir, exist_ok=True)
        lxm.write_to_directory(conv_dir)
        return conv_dir, peer_delivery

    def sent_message(text):
        peer = RNS.Identity()
        peer_delivery = RNS.Destination(peer, RNS.Destination.OUT,
                                        RNS.Destination.SINGLE, "lxmf", "delivery")
        lxm = LXMF.LXMessage(peer_delivery, own_delivery, text)
        lxm.pack()
        conv_dir = os.path.join(args.conversations, hexrep(peer_delivery.hash))
        os.makedirs(conv_dir, exist_ok=True)
        lxm.write_to_directory(conv_dir)
        return conv_dir, peer_delivery

    shapes = min(args.count, 3)
    directory_entries = []
    seeded = 0
    for i in range(shapes):
        if i == 0:
            conv, _peer = received_message("Unread seed message from the differential explorer")
            write_conv_flags(conv, {"unread": 1})
            seeded += 1
        elif i == 1:
            conv, peer = received_message("Read seed message from the trusted seed peer")
            if args.directory:
                # The trusted, named peer: makes the Trusted tab non-empty
                # (keyboard tab navigation aligns) and exercises name
                # rendering in the conversation rows.
                directory_entries.append((peer.hash, "Trusted Seed Peer", TRUSTED))
            seeded += 1
        else:
            conv, _peer = sent_message("Sent seed message from the differential explorer")
            write_conv_flags(conv, {"failed": 1})
            seeded += 1

    if args.directory and directory_entries:
        write_directory(args.directory, directory_entries)

    print("seeded %d conversation(s)" % seeded)


if __name__ == "__main__":
    main()