#!/usr/bin/env python3
"""Capture KnownNodes NodeEntry row text + style golden values from the real
Python nomadnet Network.py, using the REAL Directory.simplest_display_str /
trust_level methods bound to a fake directory instance (so the display logic is
independent), with urwid stubbed (GLib broken on this host)."""
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

# --- stub urwid (same pattern as announce_entry_golden.py) ---
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
urwid.Frame=_Widget; urwid.ListBox=_Widget; urwid.SimpleListWalker=_Widget; urwid.BoxAdapter=_Widget
urwid.connect_signal = lambda *a, **k: None
urwid.set_encoding = lambda *a, **k: None
urwid.WEIGHT=1; urwid.CENTER="center"; urwid.RELATIVE_100=("relative",100); urwid.MIDDLE="middle"
urwid.PACK="pack"; urwid.GIVEN="given"; urwid.LEFT="left"; urwid.RIGHT="right"; urwid.TOP="top"; urwid.BOTTOM="bottom"; urwid.CLIP="clip"
urwid.Overlay=_Widget; urwid.LineBox=_Widget; urwid.SolidFill=_Widget
class _AttrSpec:
    def __init__(self,*a,**k): pass
urwid.AttrSpec=_AttrSpec
urwid.ACTIVATE=object()
class _CmdMap(dict):
    def __getitem__(self,k): return None
urwid.command_map=_CmdMap()
class _RawDisplay:
    @staticmethod
    def Screen(): return _Widget()
urwid.raw_display=_RawDisplay()
urwid.MainLoop=_Widget
def _ug(n): return _Widget
urwid.__getattr__=_ug
urwid.__path__=[]
sys.modules["urwid"]=urwid
import importlib.abc, importlib.machinery
class _F(importlib.abc.MetaPathFinder):
    def find_spec(self,name,path,target=None):
        if name.startswith("urwid."): return importlib.machinery.ModuleSpec(name,_L(name))
        return None
class _L(importlib.abc.Loader):
    def __init__(self,name): self.n=name
    def create_module(self,spec):
        m=types.ModuleType(self.n); m.__path__=[]; m.__getattr__=lambda nm:_Widget; return m
    def exec_module(self,module): pass
sys.meta_path.insert(0,_F())

from nomadnet.ui import TextUI as _T
from nomadnet.ui.textui import Network as _Net
from nomadnet.Directory import Directory, DirectoryEntry
import RNS

GLYPHS=_T.GLYPHS; GLYPHSETS=_T.GLYPHSETS
def glyph_dict(gs):
    g={}; idx=GLYPHSETS[gs]
    for tup in GLYPHS: g[tup[0]]=tup[idx]
    return g

class FakeUI:
    def __init__(self,g): self.glyphs=g; self.colormode=256
class FakeApp:
    def __init__(self,g,sanitize): self.ui=FakeUI(g); self.config={"textui":{"sanitize_names":sanitize}}
    def __getattr__(self,n): return None
nomadnet=sys.modules["nomadnet"]
class FNN:
    _shared=None
    @classmethod
    def get_shared_instance(cls): return cls._shared
nomadnet.NomadNetworkApp=FNN

SH=bytes(range(16))
HEX=RNS.hexrep(SH, delimit=False)

def fake_dir(entries, sanitize):
    app=FakeApp(glyph_dict("unicode"), sanitize)
    FNN._shared=app
    # entries: list of (source_hash_bytes, display_name, trust_level)
    de={}
    for sh,name,trust in entries:
        de[bytes(sh)]=DirectoryEntry(bytes(sh), display_name=name, trust_level=trust)
    d=types.SimpleNamespace(directory_entries=de, app=app)
    d.trust_level=Directory.trust_level.__get__(d)
    d.simplest_display_str=Directory.simplest_display_str.__get__(d)
    app.directory=d
    return app,d

class FakeDelegate:
    def connect_node(self, *a, **k): pass

def capture(entries, sanitize=False, target=bytes(SH)):
    app,d=fake_dir(entries, sanitize)
    _AttrMap.captured=[]
    node=types.SimpleNamespace(source_hash=bytes(target), trust_level=d.trust_level(bytes(target)))
    try:
        e=_Net.NodeEntry(app, node, FakeDelegate())
    except Exception as ex:
        return {"error":str(ex),"captured":_AttrMap.captured}
    txt,style,fstyle=_AttrMap.captured[0] if _AttrMap.captured else (None,None,None)
    return {"row":txt,"style":style,"focus_style":fstyle}

T=DirectoryEntry.TRUSTED; U=DirectoryEntry.UNTRUSTED; W=DirectoryEntry.WARNING; UNK=DirectoryEntry.UNKNOWN
cases=[
    {"name":"trusted_named",        "entries":[(SH,"Alice",T)]},
    {"name":"untrusted_named",      "entries":[(SH,"Alice",U)]},
    {"name":"warning_named",        "entries":[(SH,"Alice",W)]},
    {"name":"unknown_in_dir_named", "entries":[(SH,"Alice",UNK)]},
    {"name":"trusted_empty_name",   "entries":[(SH,"",T)]},
    {"name":"unknown_not_in_dir",   "entries":[]},
    {"name":"trusted_not_in_dir",   "entries":[]},
    {"name":"trusted_sanitize_on",  "entries":[(SH,"Hello > world",T)], "sanitize":True},
    {"name":"trusted_sanitize_off", "entries":[(SH,"Hello > world",T)], "sanitize":False},
]
out=[]
for c in cases:
    r=capture(c["entries"], sanitize=c.get("sanitize",False))
    r["name"]=c["name"]
    out.append(r)
print(json.dumps(out, ensure_ascii=False, indent=2))
