#!/usr/bin/env python3
"""Capture urwid calc_coords golden (x,y) values for a known set of wrapped
strings. Output is JSON on stdout, consumed by the Go golden test
tui/cursor-coords_test.go.

urwid 4.x pulls in a GLib event loop on import, which is broken on this host.
We stub out `gi.repository.GLib`/`Gio` so the import succeeds — we only need
the pure-Python text_layout machinery.
"""
import sys, types, json

# --- stub gi so urwid.text_layout imports without GLib ----------------------
gi = types.ModuleType("gi")
girepo = types.ModuleType("gi.repository")
glib = types.ModuleType("gi.repository.GLib")
for n in ["MainLoop", "MainContext"]:
    setattr(glib, n, object)
glib.PRIORITY_HIGH = glib.PRIORITY_DEFAULT = 0
glib.timeout_add_seconds = glib.timeout_add = glib.idle_add = glib.source_remove = (
    lambda *a, **k: 0
)
glib.IOCondition = object
glib.IO_IN = 1
gio = types.ModuleType("gi.repository.Gio")
for n in ["UnixInputStream", "Socket"]:
    setattr(gio, n, object)
girepo.GLib = glib
girepo.Gio = gio
gi.repository = girepo
gi.require_version = lambda *a, **k: None
sys.modules["gi"] = gi
sys.modules["gi.repository"] = girepo
sys.modules["gi.repository.GLib"] = glib
sys.modules["gi.repository.Gio"] = gio
# -----------------------------------------------------------------------------

import urwid  # noqa: E402
from urwid.text_layout import calc_coords  # noqa: E402

CASES = [
    # (text, maxcol, [positions to probe])
    ("The quick brown fox jumps over the lazy dog", 20, [0, 1, 5, 10, 18, 19, 20, 21, 25, 30, 39, 41, 43, 44]),
    ("hello", 20, [0, 3, 5]),
    ("abcdefghijklmnopqrstuvwxyz", 10, [0, 5, 9, 10, 11, 20, 25, 26]),
    ("one two three four", 8, [0, 3, 4, 7, 8, 11, 12, 17, 18]),
    ("  leading spaces here", 12, [0, 2, 9, 12, 13, 21, 22]),
    ("word", 4, [0, 2, 4]),
    ("a b c d e f", 3, [0, 1, 2, 3, 4, 5, 6, 10, 11]),
    ("", 20, [0]),
]

out = []
for text, maxcol, positions in CASES:
    t = urwid.Text(text)
    trans = t.get_line_translation(maxcol)
    coords = {}
    for pos in positions:
        coords[pos] = list(calc_coords(text, trans, pos))
    out.append({"text": text, "maxcol": maxcol, "coords": coords})

print(json.dumps(out, indent=2))