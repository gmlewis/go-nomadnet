#!/usr/bin/env python3
"""Capture LXMessageWidget title_string + header_style golden values from the
real Python nomadnet, by stubbing urwid (GLib broken on this host) and
instantiating LXMessageWidget with a fake app + fake message."""
import sys, types, json

# --- stub gi so urwid imports -----------------------------------------------
gi = types.ModuleType("gi")
girepo = types.ModuleType("gi.repository")
glib = types.ModuleType("gi.repository.GLib")
for n in ["MainLoop", "MainContext"]:
    setattr(glib, n, object)
glib.PRIORITY_HIGH = glib.PRIORITY_DEFAULT = 0
glib.timeout_add_seconds = glib.timeout_add = glib.idle_add = glib.source_remove = lambda *a, **k: 0
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

# --- stub urwid with fake classes sufficient to construct LXMessageWidget ---
urwid = types.ModuleType("urwid")
class _Widget:
    def __init__(self, *a, **k): pass
    def set_text(self, t): self._t = t
    def get_text(self): return getattr(self, "_t", "")
class _Text(_Widget):
    def __init__(self, text, *a, **k): self._t = text
class _AttrMap(_Widget):
    captured = []
    def __init__(self, w, attr, *a, **k):
        self._w = w; self._attr = attr
        # record (text, style) for every AttrMap so we can pick the msg_header one
        txt = w.get_text() if hasattr(w, "get_text") else ""
        _AttrMap.captured.append((txt, attr))
class _Pile(_Widget):
    def __init__(self, items): self._items = items
class _Columns(_Widget):
    def __init__(self, items): self._items = items
class _Padding(_Widget):
    def __init__(self, w, *a, **k): self._w = w
class _WidgetWrap(_Widget):
    def __init__(self, w): self._w = w
class _Screen:
    pass
class _MainLoop:
    def __init__(self, *a, **k): pass
    def set_alarm_in(self, *a, **k): pass
urwid.Text = _Text
urwid.AttrMap = _AttrMap
urwid.Pile = _Pile
urwid.Columns = _Columns
urwid.Padding = _Padding
urwid.WidgetWrap = _WidgetWrap
urwid.MainLoop = _MainLoop
urwid.set_encoding = lambda *a, **k: None
class _RawDisplay:
    @staticmethod
    def Screen(): return _Screen()
urwid.raw_display = _RawDisplay()
urwid.BoxAdapter = _Widget
urwid.Filler = _Widget
urwid.Frame = _Widget
urwid.Pile = _Pile
urwid.Columns = _Columns
urwid.ListBox = _Widget
urwid.SimpleListWalker = _Widget
urwid.Text = _Text
def _urwid_getattr(name):
    return _Widget
urwid.__getattr__ = _urwid_getattr
urwid.__path__ = []  # mark as package so submodule imports resolve
sys.modules["urwid"] = urwid

# Meta finder: any urwid.<sub> import resolves to a stub module whose every
# attribute is _Widget (so `from urwid.util import is_mouse_press` etc. work).
import importlib.abc, importlib.machinery
class _UrwidSubFinder(importlib.abc.MetaPathFinder):
    def find_spec(self, name, path, target=None):
        if name.startswith("urwid."):
            loader = _UrwidSubLoader(name)
            return importlib.machinery.ModuleSpec(name, loader)
        return None
class _UrwidSubLoader(importlib.abc.Loader):
    def __init__(self, name): self._name = name
    def create_module(self, spec):
        m = types.ModuleType(self._name)
        m.__path__ = []
        m.__getattr__ = lambda nm: _Widget
        return m
    def exec_module(self, module): pass
sys.meta_path.insert(0, _UrwidSubFinder())
# -----------------------------------------------------------------------------

import time as _time
import LXMF
from nomadnet.ui import TextUI as _TextUIMod
from nomadnet.ui.textui import Conversations as _Conv
from nomadnet.Directory import DirectoryEntry

GLYPHS = _TextUIMod.GLYPHS
GLYPHSETS = _TextUIMod.GLYPHSETS

def glyph_dict(glyphset):
    g = {}
    idx = GLYPHSETS[glyphset]
    for tup in GLYPHS:
        g[tup[0]] = tup[idx]
    return g

LXMFMessage = LXMF.LXMessage
# LXMF state constants
S_DELIVERED = LXMFMessage.DELIVERED
S_FAILED    = LXMFMessage.FAILED
S_REJECTED  = LXMFMessage.REJECTED
S_SENT      = LXMFMessage.SENT
S_PAPER     = LXMFMessage.PAPER
M_PROPAGATED = LXMFMessage.PROPAGATED
M_PAPER      = LXMFMessage.PAPER

OWN_HASH = b"\x11\x22\x33\x44\x55\x66\x77\x88\x99\xaa\xbb\xcc\xdd\xee\xff\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10"
PEER_HASH = b"\xaa\xbb\xcc\xdd\xee\xff\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x22\x33\x44\x55\x66\x77\x88\x99"

class FakeLoop:
    def set_alarm_in(self, *a, **k): pass

class FakeDir:
    def trust_level(self, h):
        return DirectoryEntry.TRUSTED

class FakeUI:
    def __init__(self, g):
        self.glyphs = g
        self.colormode = 256
        self.loop = FakeLoop()

class FakeApp:
    def __init__(self, g):
        self.ui = FakeUI(g)
        self.lxmf_destination = types.SimpleNamespace(hash=OWN_HASH)
        self.time_format = "%Y-%m-%d %H:%M:%S"
        self.config = {"textui": {"clipboard_copy": False}}
        self.directory = FakeDir()
        self.message_router = types.SimpleNamespace(pending_outbound=[])

class FakeMessage:
    def __init__(self, **kw):
        self.timestamp = kw.get("timestamp", _time.time() - 3600)  # 1h ago -> "1h ago"
        self.sort_timestamp = self.timestamp
        self._cached_source_hash = kw.get("source_hash", OWN_HASH)
        self._cached_method = kw.get("method", M_PROPAGATED)
        self._state = kw.get("state", S_SENT)
        self._transport_encrypted = kw.get("transport_encrypted", True)
        self._title = kw.get("title", "")
        self._content = kw.get("content", "Hello world")
        self._renderer = kw.get("renderer", LXMF.RENDERER_MICRON)
        self._sig_validated = kw.get("sig_validated", True)
        self._sig_desc = kw.get("sig_desc", "Signature correct")
        self._has_attachments = kw.get("has_attachments", False)
        self._cached_attachment_names = kw.get("attachment_names", [])
    def get_timestamp(self): return self.timestamp
    def get_hash(self): return b"\x00"*32
    def get_state(self): return self._state
    def get_transport_encrypted(self): return self._transport_encrypted
    def get_title(self): return self._title
    def get_content(self): return self._content
    def content_renderer(self): return self._renderer
    def signature_validated(self): return self._sig_validated
    def get_signature_description(self): return self._sig_desc
    def has_attachments(self): return self._has_attachments

nomadnet = sys.modules.get("nomadnet")
class FakeNomadNetApp:
    _shared = None
    @classmethod
    def get_shared_instance(cls):
        return cls._shared
nomadnet.NomadNetworkApp = FakeNomadNetApp

def capture(msg_kwargs, glyphset="unicode"):
    g = glyph_dict(glyphset)
    app = FakeApp(g)
    FakeNomadNetApp._shared = app
    _AttrMap.captured = []
    msg = FakeMessage(**msg_kwargs)
    try:
        w = _Conv.LXMessageWidget(msg, theme=_TextUIMod.THEME_DARK if False else __import__("nomadnet.ui", fromlist=["THEME_DARK"]).THEME_DARK, conversation_widget=None)
    except Exception as e:
        return {"error": str(e), "captured": _AttrMap.captured}
    # find the msg_header AttrMap
    title = None; style = None
    for txt, attr in _AttrMap.captured:
        if attr.startswith("msg_header"):
            title = txt; style = attr
            break
    return {"title": title, "style": style, "all": _AttrMap.captured}

# Timestamp fixed in the past so relative_time is deterministic.
# relative_time: delta<60 "just now", <3600 "Nm ago", <86400 "Nh ago", <172800 "yesterday"
# Use a fixed NOW by monkeypatching time.time inside Conversations.
import time as _t
_fixed_now = 1_700_000_000.0
_orig_time = _t.time
_t.time = lambda: _fixed_now
try:
    _Conv.time.time = lambda: _fixed_now
except Exception:
    pass

TS = _fixed_now - 3600  # exactly 1h ago -> "1h ago"

CASES = [
    {"name":"outbound_sent",        "source_hash":OWN_HASH,  "state":S_SENT,      "method":M_PROPAGATED},
    {"name":"outbound_delivered",   "source_hash":OWN_HASH,  "state":S_DELIVERED, "method":M_PROPAGATED},
    {"name":"outbound_failed",      "source_hash":OWN_HASH,  "state":S_FAILED,    "method":M_PROPAGATED},
    {"name":"outbound_rejected",    "source_hash":OWN_HASH,  "state":S_REJECTED,  "method":M_PROPAGATED},
    {"name":"outbound_propagated",  "source_hash":OWN_HASH,  "state":S_SENT,      "method":M_PROPAGATED, "_note":"state==SENT & method==PROPAGATED"},
    {"name":"outbound_paper",       "source_hash":OWN_HASH,  "state":S_PAPER,     "method":M_PAPER},
    {"name":"outbound_pending",     "source_hash":OWN_HASH,  "state":2,           "method":M_PROPAGATED, "_note":"state<SENT -> default branch arrow_r only"},
    {"name":"inbound_trusted_sig",  "source_hash":PEER_HASH, "state":S_SENT,      "method":M_PROPAGATED, "sig_validated":True},
    {"name":"inbound_untrusted_sig","source_hash":PEER_HASH, "state":S_SENT,      "method":M_PROPAGATED, "sig_validated":False, "sig_desc":"Signature could not be verified"},
    {"name":"failed_no_source",     "source_hash":None,      "state":S_FAILED,    "method":M_PROPAGATED},
    {"name":"with_title",           "source_hash":OWN_HASH,  "state":S_SENT,      "method":M_PROPAGATED, "title":"My Subject"},
    {"name":"plaintext",            "source_hash":OWN_HASH,  "state":S_SENT,      "method":M_PROPAGATED, "transport_encrypted":False},
    {"name":"with_attachment",      "source_hash":OWN_HASH,  "state":S_SENT,      "method":M_PROPAGATED, "has_attachments":True, "attachment_names":[("file","report.pdf",2048)]},
]

out = []
for c in CASES:
    kw = {k:v for k,v in c.items() if not k.startswith("_") and k!="name"}
    kw["timestamp"] = TS
    res = capture(kw)
    res["name"] = c["name"]
    if "_note" in c: res["note"] = c["_note"]
    out.append(res)

print(json.dumps(out, ensure_ascii=False, indent=2))
