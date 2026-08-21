#!/usr/bin/env python3
"""Capture AnnounceStreamEntry row text + style golden values from the real
Python nomadnet Network.py, by stubbing urwid (GLib broken on this host)."""
import sys, types, json

# --- stub gi ---
gi = types.ModuleType("gi"); girepo = types.ModuleType("gi.repository")
glib = types.ModuleType("gi.repository.GLib")
for n in ["MainLoop","MainContext"]: setattr(glib, n, object)
glib.PRIORITY_HIGH = glib.PRIORITY_DEFAULT = 0
glib.timeout_add_seconds = glib.timeout_add = glib.idle_add = glib.source_remove = lambda *a, **k: 0
glib.IOCondition = object; glib.IO_IN = 1
gio = types.ModuleType("gi.repository.Gio")
for n in ["UnixInputStream","Socket"]: setattr(gio, n, object)
girepo.GLib = glib; girepo.Gio = gio; gi.repository = girepo
gi.require_version = lambda *a, **k: None
sys.modules["gi"]=gi; sys.modules["gi.repository"]=girepo
sys.modules["gi.repository.GLib"]=glib; sys.modules["gi.repository.Gio"]=gio

# --- stub urwid ---
urwid = types.ModuleType("urwid")
class _Widget:
    def __init__(self, *a, **k): pass
    def set_text(self, t): self._t = t
    def get_text(self): return getattr(self, "_t", "")
class _Text(_Widget):
    def __init__(self, text, *a, **k): self._t = text
class _AttrMap(_Widget):
    captured = []
    def __init__(self, w, attr, focus_attr=None, *a, **k):
        self._w=w; self._attr=attr; self._fattr=focus_attr
        txt = w.get_text() if hasattr(w,"get_text") else ""
        _AttrMap.captured.append((txt, attr, focus_attr))
class _Pile(_Widget):
    def __init__(self, items): self._items=items
class _Columns(_Widget):
    def __init__(self, items): self._items=items
class _WidgetWrap(_Widget):
    def __init__(self, w): self._w=w
urwid.Text=_Text; urwid.AttrMap=_AttrMap; urwid.Pile=_Pile; urwid.Columns=_Columns
urwid.WidgetWrap=_WidgetWrap; urwid.Button=_Widget; urwid.Padding=_Widget; urwid.Filler=_Widget
urwid.Frame=_Widget; urwid.ListBox=_Widget; urwid.SimpleListWalker=_Widget
urwid.BoxAdapter=_Widget; urwid.Pile=_Pile; urwid.Columns=_Columns
urwid.connect_signal = lambda *a, **k: None
urwid.set_encoding = lambda *a, **k: None
urwid.WEIGHT = 1; urwid.CENTER = "center"; urwid.RELATIVE_100 = ("relative", 100)
urwid.MIDDLE = "middle"; urwid.PACK = "pack"; urwid.OFFIX = "offix"; urwid.GIVEN = "given"; urwid.LEFT="left"; urwid.RIGHT="right"; urwid.TOP="top"; urwid.BOTTOM="bottom"; urwid.CLIP="clip"
urwid.Overlay = _Widget; urwid.Filler = _Widget; urwid.LineBox = _Widget; urwid.SolidFill = _Widget
class _AttrSpec:
    def __init__(self, *a, **k): pass
urwid.AttrSpec = _AttrSpec
ACTIVATE = object()
class _CommandMap(dict):
    def __getitem__(self, k): return None
urwid.ACTIVATE = ACTIVATE
urwid.command_map = _CommandMap()
class _RawDisplay:
    @staticmethod
    def Screen(): return _Widget()
urwid.raw_display = _RawDisplay()
urwid.MainLoop = _Widget
def _urwid_getattr(name): return _Widget
urwid.__getattr__ = _urwid_getattr
urwid.__path__ = []
sys.modules["urwid"] = urwid
import importlib.abc, importlib.machinery
class _UrwidSubFinder(importlib.abc.MetaPathFinder):
    def find_spec(self, name, path, target=None):
        if name.startswith("urwid."):
            return importlib.machinery.ModuleSpec(name, _UrwidSubLoader(name))
        return None
class _UrwidSubLoader(importlib.abc.Loader):
    def __init__(self, name): self._name=name
    def create_module(self, spec):
        m = types.ModuleType(self._name); m.__path__=[]; m.__getattr__ = lambda nm: _Widget
        return m
    def exec_module(self, module): pass
sys.meta_path.insert(0, _UrwidSubFinder())

import time as _time
from nomadnet.ui import TextUI as _TextUIMod
from nomadnet.ui.textui import Network as _Net
from nomadnet.Directory import DirectoryEntry
import RNS

GLYPHS = _TextUIMod.GLYPHS; GLYPHSETS = _TextUIMod.GLYPHSETS
def glyph_dict(gs):
    g={}; idx=GLYPHSETS[gs]
    for tup in GLYPHS: g[tup[0]]=tup[idx]
    return g

# Fixed now so the same-day check is deterministic.
_fixed_now = 1_700_000_000.0  # 2023-11-14 22:13:20 UTC
_time.time = lambda: _fixed_now
_Net.time.time = lambda: _fixed_now

class FakeDir:
    def __init__(self, trust_map, display_map): self._t=trust_map; self._d=display_map
    def trust_level(self, h): return self._t.get(bytes(h), DirectoryEntry.UNKNOWN)
    def simplest_display_str(self, h): return self._d.get(bytes(h), RNS.prettyhexrep(h))
class FakeUI:
    def __init__(self, g): self.glyphs=g; self.colormode=256
    # main_display attribute access tolerated
    def __getattr__(self, n): return None
class FakeApp:
    def __init__(self, g, trust_map, display_map, sanitize):
        self.ui=FakeUI(g); self.time_format="%Y-%m-%d %H:%M:%S"
        self.config={"textui":{"sanitize_names": sanitize}}
        self.directory=FakeDir(trust_map, display_map)
    def __getattr__(self, n): return None

nomadnet = sys.modules["nomadnet"]
class FakeNomadNetApp:
    _shared=None
    @classmethod
    def get_shared_instance(cls): return cls._shared
nomadnet.NomadNetworkApp = FakeNomadNetApp

SH = bytes(range(32))  # source hash 32 bytes
PEER = bytes([0xaa]*32)

def capture(ts, name, atype, trust=DirectoryEntry.TRUSTED, show_dest=False, sanitize=False, display_map=None):
    g = glyph_dict("unicode")
    trust_map = {bytes(SH): trust}
    dm = display_map or {}
    app = FakeApp(g, trust_map, dm, sanitize)
    FakeNomadNetApp._shared = app
    _AttrMap.captured = []
    announce = (ts, SH, name, atype)
    try:
        e = _Net.AnnounceStreamEntry(app, announce, delegate=None, show_destination=show_dest)
    except Exception as ex:
        return {"error": str(ex), "captured": _AttrMap.captured}
    txt, style, fstyle = _AttrMap.captured[0] if _AttrMap.captured else (None,None,None)
    return {"row": txt, "style": style, "focus_style": fstyle}

# now = 2023-11-14 22:13:20 UTC. same-day ts -> time format; other-day -> date.
TODAY = _fixed_now - 3600       # same day, 21:13:20
OTHERDAY = _fixed_now - 7*86400 # 2023-11-07, date format

cases = [
    {"case":"node_trusted_today",        "ts":TODAY,    "name":b"Alice",         "atype":"node",  "trust":DirectoryEntry.TRUSTED},
    {"case":"peer_untrusted_today",      "ts":TODAY,    "name":b"Bob",           "atype":"peer",  "trust":DirectoryEntry.UNTRUSTED},
    {"case":"pn_unknown_today",          "ts":TODAY,    "name":b"PN-1",          "atype":"pn",    "trust":DirectoryEntry.UNKNOWN},
    {"case":"node_warning_otherday",     "ts":OTHERDAY, "name":b"Carol",         "atype":"node",  "trust":DirectoryEntry.WARNING},
    {"case":"long_name_truncate",        "ts":TODAY,    "name":b"A"+b"x"*40,     "atype":"node",  "trust":DirectoryEntry.TRUSTED},
    {"case":"show_destination_hexrep",   "ts":TODAY,    "name":b"Alice",         "atype":"node",  "trust":DirectoryEntry.TRUSTED, "show_dest":True},
    {"case":"sanitize_on",               "ts":TODAY,    "name":b"Hello > world", "atype":"node",  "trust":DirectoryEntry.TRUSTED, "sanitize":True},
    {"case":"sanitize_off_strip_mods",   "ts":TODAY,    "name":b"Hello > world", "atype":"node",  "trust":DirectoryEntry.TRUSTED, "sanitize":False},
    {"case":"empty_name_prettyhex",      "ts":TODAY,    "name":b"",              "atype":"node",  "trust":DirectoryEntry.TRUSTED},
]

out=[]
for c in cases:
    r = capture(c["ts"], c["name"], c["atype"], trust=c.get("trust",DirectoryEntry.TRUSTED),
                show_dest=c.get("show_dest",False), sanitize=c.get("sanitize",False))
    r["case"]=c["case"]
    r["input_name"]=c["name"].decode("utf-8","replace")
    out.append(r)
print(json.dumps(out, ensure_ascii=False, indent=2))
