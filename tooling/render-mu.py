#!/usr/bin/env python3
# render-mu.py — headless Python nomadnet micron renderer (logical-line parity).
#
# Renders a .mu file through the source-of-truth Python nomadnet MicronParser
# (urwid stubbed so it runs headlessly on macOS without GLib) and emits the same
# structured logical-line JSON schema as `cmd/view-mu -json`. Paired with
# view-mu, `tooling/diff-mu.py` diffs the two to find rendering parity bugs
# (headers, colors, attributes, links, fields, dividers) deterministically,
# with no network and no tmux.
#
# This is PRE-WRAP (one entry per micron source line, faithful to the parser's
# segmentation). Wrapping/layout parity is compared by the live loopback
# comparator, not here.
#
# Usage:
#   python3 tooling/render-mu.py file.mu [--theme dark|light] [--json]
#   cat file.mu | python3 tooling/render-mu.py - [--json]
#
# Diagnostics go to stderr; the JSON document goes to stdout.

import argparse
import json
import os
import sys
import types
import importlib.abc
import importlib.machinery

# ---------------------------------------------------------------------------
# 1. Stub gi / GLib / Gio / GObject so urwid's GLib import doesn't fail headless.
# ---------------------------------------------------------------------------
def _stub_gi():
    gi = types.ModuleType("gi")
    gi.require_version = lambda *a, **k: None
    sys.modules["gi"] = gi
    for sub in ("gi.repository", "gi.repository.GLib", "gi.repository.Gio",
                "gi.repository.GObject"):
        m = types.ModuleType(sub)
        m.__getattr__ = lambda *a: None
        sys.modules[sub] = m

_stub_gi()

# ---------------------------------------------------------------------------
# 2. Stub urwid with distinct widget classes so we can read back the markup the
#    parser builds. Based on tooling/tui-parity/*_golden.py.
# ---------------------------------------------------------------------------
urwid = types.ModuleType("urwid")

class _Widget:
    def __init__(self, *a, **k):
        self._args = a
        self._kwargs = k
    def __getattr__(self, name):
        # Some urwid internals probe attributes at import/class-def time.
        raise AttributeError(name)

class _Text(_Widget):
    def __init__(self, text, *a, **k):
        self._t = text
        self._align = k.get("align")
        self._args = (text,) + a
        self._kwargs = k
    def get_text(self):
        return (self._t, [])

class _AttrMap(_Widget):
    def __init__(self, w, attr, focus_attr=None, *a, **k):
        self._w = w
        self._attr = attr
        self._fattr = focus_attr
        self._args = (w, attr)
        self._kwargs = k

class _Pile(_Widget):
    def __init__(self, items, *a, **k):
        self._items = items
        self._args = (items,)
        self._kwargs = k

class _Columns(_Widget):
    def __init__(self, items, *a, **k):
        self._items = items
        self._args = (items,)
        self._kwargs = k

class _Padding(_Widget):
    def __init__(self, w, *a, **k):
        self._w = w
        self._left = k.get("left", 0)
        self._right = k.get("right", 0)
        self._args = (w,)
        self._kwargs = k

class _Divider(_Widget):
    def __init__(self, char="─", *a, **k):
        self._char = char
        self._args = (char,)
        self._kwargs = k

class _AttrSpec(_Widget):
    def __init__(self, fg="default", bg="default", *a, **k):
        self.foreground = fg
        self.background = bg
        self._args = (fg, bg)
        self._kwargs = k

class _CheckBox(_Widget):
    pass

class _RadioButton(_Widget):
    pass

class _Filler(_Widget):
    pass

urwid.Text = _Text
urwid.AttrMap = _AttrMap
urwid.Pile = _Pile
urwid.Columns = _Columns
urwid.Padding = _Padding
urwid.Divider = _Divider
urwid.AttrSpec = _AttrSpec
urwid.CheckBox = _CheckBox
urwid.RadioButton = _RadioButton
urwid.Filler = _Filler
urwid.Button = _Widget
urwid.Frame = _Widget
urwid.ListBox = _Widget
urwid.SimpleListWalker = _Widget
urwid.BoxAdapter = _Widget
urwid.WidgetWrap = _Widget
urwid.SolidFill = _Widget
urwid.Overlay = _Widget
urwid.LineBox = _Widget
urwid.Edit = _Widget
urwid.connect_signal = lambda *a, **k: None
urwid.set_encoding = lambda *a, **k: None
urwid.WEIGHT = 1
urwid.CENTER = "center"
urwid.RELATIVE_100 = ("relative", 100)
urwid.MIDDLE = "middle"
urwid.PACK = "pack"
urwid.OFFIX = "offix"
urwid.GIVEN = "given"
urwid.LEFT = "left"
urwid.RIGHT = "right"
urwid.TOP = "top"
urwid.BOTTOM = "bottom"
urwid.CLIP = "clip"
urwid.AttrSpec = _AttrSpec

def _urwid_getattr(name):
    return _Widget
urwid.__getattr__ = _urwid_getattr
urwid.__path__ = []
sys.modules["urwid"] = urwid

class _UrwidSubFinder(importlib.abc.MetaPathFinder):
    def find_spec(self, name, path, target=None):
        if name.startswith("urwid."):
            return importlib.machinery.ModuleSpec(name, _UrwidSubLoader(name))
        return None

class _UrwidSubLoader(importlib.abc.Loader):
    def __init__(self, name):
        self._name = name
    def create_module(self, spec):
        m = types.ModuleType(self._name)
        m.__path__ = []
        m.__getattr__ = lambda nm: _Widget
        return m
    def exec_module(self, module):
        pass

sys.meta_path.insert(0, _UrwidSubFinder())

# ---------------------------------------------------------------------------
# 3. Import nomadnet + inject a FakeNomadNetApp (ensure_selected_styles and the
#    link branch read app.config["textui"]["theme"] and app.ui.colormode).
# ---------------------------------------------------------------------------
try:
    import nomadnet  # noqa: F401
except ImportError:
    # Fall back to the nomadnet source checkout, which also carries the RNS and
    # LXMF symlinks. The canonical environment is the homebrew py3.14 install
    # (/opt/homebrew/bin/python3), where nomadnet is already importable.
    _checkout = os.path.expanduser("~/src/github.com/markqvist/nomadnet")
    if _checkout not in sys.path:
        sys.path.insert(0, _checkout)
    import nomadnet  # noqa: F401

class _FakeUI:
    def __init__(self):
        self.colormode = 2 ** 24
        self.screen = None

class _DummyDelegate:
    # Stands in for the Browser so links become LinkSpecs. LinkableText sets
    # last_keypress on it; we never call back into it.
    last_keypress = 0
    def handle_link(self, *a, **k):
        pass
    def marked_link(self, *a, **k):
        pass

class _FakeApp:
    _shared = None
    def __init__(self, theme):
        self.config = {"textui": {"theme": theme}}
        self.ui = _FakeUI()
    @classmethod
    def get_shared_instance(cls):
        return cls._shared

# nomadnet themes are integer constants (THEME_DARK=1, THEME_LIGHT=2), NOT
# strings — ensure_selected_styles compares config["textui"]["theme"] against
# them, so a "dark" string would silently select the LIGHT palette.
from nomadnet.ui import TextUI as _TextUI  # noqa: E402
_THEME_DARK = _TextUI.THEME_DARK
_THEME_LIGHT = _TextUI.THEME_LIGHT

_FAKE_APP = _FakeApp(_THEME_DARK)
_FakeApp._shared = _FAKE_APP
nomadnet.NomadNetworkApp = _FakeApp  # ensure_selected_styles resolves this

from nomadnet.ui.textui import MicronParser as M  # noqa: E402
from nomadnet.util import strip_modifiers  # noqa: E402

# The real ReadlineEdit class (subclass of the stubbed urwid.Edit). Used only to
# detect field widgets; field extraction is best-effort.
_ReadlineEditCls = getattr(M, "ReadlineEdit", None)

# ---------------------------------------------------------------------------
# 4. Patch make_style to return a hashable frozen-style key (with a side cache
#    and a fake SYNTH_SPECS entry), and LinkSpec to a stub carrying the target.
#    This captures per-run colors directly from parser state, with no screen.
# ---------------------------------------------------------------------------
STYLE_CACHE = {}  # style key -> {fg,bg,bold,underline,italic}

class _FakeSpec:
    def __init__(self, key):
        self.key = key
        self.foreground = "default"
        self.background = "default"

class _LinkStub:
    def __init__(self, link_target, orig_spec, cm=256):
        self.link_target = link_target
        self.link_fields = None
        self.style_key = getattr(orig_spec, "key", None)

def _my_make_style(state):
    s = dict(M.state_to_style(state))
    key = (s["fg"], s["bg"], s["bold"], s["underline"], s["italic"])
    STYLE_CACHE[key] = s
    if key not in M.SYNTH_SPECS:
        # A 5-list of fake specs (one per color depth) so the link branch's
        # `orig_spec = speclist[cm_index]` resolves for any colormode.
        M.SYNTH_SPECS[key] = [_FakeSpec(key)] * 5
    return key

M.make_style = _my_make_style
M.LinkSpec = _LinkStub

# Headings: map the SELECTED heading styles' fg/bg (normalized) to a level, so
# we can report heading_level like the Go renderer. Uses M.SELECTED_STYLES (the
# palette ensure_selected_styles actually committed to) rather than a hardcoded
# dark/light table, so the color->level map always matches the emitted spans.
def _heading_levels():
    levels = {}
    styles = M.SELECTED_STYLES
    for lvl in (1, 2, 3):
        key = "heading%d" % lvl
        if key not in styles:
            continue
        st = styles[key]
        levels[(_norm_color(st["fg"]), _norm_color(st["bg"]))] = lvl
    return levels

# ---------------------------------------------------------------------------
# 5. Color normalization: Python micron state colors are raw ("ddd", "ffffff",
#    "g56", "default"); normalize to the Go StyledSpan forms ("#rrggbb",
#    "default", "gNN") so the two JSON documents diff directly.
# ---------------------------------------------------------------------------
def _norm_color(c):
    if c is None or c == "default":
        return "default"
    c = c.lower()
    if c.startswith("g") and len(c) == 3:
        return c
    if len(c) == 3:
        return "#" + c[0] * 2 + c[1] * 2 + c[2] * 2
    if len(c) == 6:
        return "#" + c
    return c

def _style_from_key(key):
    s = STYLE_CACHE.get(key)
    if not s:
        return {"fg": "default", "bg": "default", "bold": False,
                "underline": False, "italic": False}
    return {"fg": _norm_color(s["fg"]), "bg": _norm_color(s["bg"]),
            "bold": bool(s["bold"]), "underline": bool(s["underline"]),
            "italic": bool(s["italic"])}

# ---------------------------------------------------------------------------
# 6. Page-level #!fg= / #!bg= extraction, matching Browser.load_page.
# ---------------------------------------------------------------------------
def _page_colors(markup):
    fg = bg = None
    pos = markup.find("#!fg=")
    if pos >= 0:
        end = markup.find("\n", pos)
        if end in (pos + 8, pos + 11):  # 3 or 6 hex digits
            fg = markup[pos + 5:end]
    pos = markup.find("#!bg=")
    if pos >= 0:
        end = markup.find("\n", pos)
        if end in (pos + 8, pos + 11):
            bg = markup[pos + 5:end]
    return fg, bg

# ---------------------------------------------------------------------------
# 7. Walk the AttrMapList and build the JSON document.
# ---------------------------------------------------------------------------
def _align_name(a):
    if a == "center":
        return "center"
    if a == "right":
        return "right"
    return "left"

def _unwrap(widget):
    """Walk AttrMap/Padding wrappers to the core widget.

    Returns (core, inner_attr, pad_left) where inner_attr is the innermost
    AttrMap's style key (the one that actually styles the text, e.g. a heading
    style on the nested AttrMap a heading builds) and pad_left is the sum of
    Padding left offsets. Headings build AttrMap(AttrMap(Text(...), heading),
    plain), so the heading style lives one level in — the outer line_attr is the
    plain style and must not be used for the text.
    """
    inner_attr = None
    pad_left = 0
    while True:
        if isinstance(widget, _AttrMap):
            inner_attr = widget._attr
            widget = widget._w
            continue
        if isinstance(widget, _Padding):
            pad_left += widget._left or 0
            widget = widget._w
            continue
        break
    return widget, inner_attr, pad_left


def _extract_spans(widget, line_attr):
    """Return (spans, divider_char_or_None, indent, inner_attr, align) from a
    line's (possibly wrapped) inner widget. indent folds the heading's baked-in
    leading spaces into the indent field (matching the Go renderer, which does
    not put indent spaces in spans)."""
    core, inner_attr, pad_left = _unwrap(widget)
    bare_attr = inner_attr if isinstance(inner_attr, tuple) else line_attr

    if isinstance(core, _Divider):
        return [], core._char, pad_left, inner_attr, None
    if isinstance(core, _Columns):
        spans = []
        for item in core._items:
            if not isinstance(item, tuple) or len(item) < 2:
                continue
            child = item[1]
            if isinstance(child, _AttrMap):
                spans += _field_spans(child)
        return spans, None, pad_left, inner_attr, None
    if isinstance(core, _Text):
        parts = core._t
        align = getattr(core, "_align", None)
        # Headings prepend a bare all-space string (" "*left_indent) to the text
        # run list. Treat it as indent, not a span, so the JSON matches the Go
        # renderer's Indent field.
        baked_indent = 0
        if isinstance(parts, list) and parts and isinstance(parts[0], str) \
                and parts[0].strip() == "":
            baked_indent = len(parts[0])
            parts = parts[1:]
        spans = _text_spans(parts, bare_attr)
        return spans, None, pad_left + baked_indent, inner_attr, align
    # Anything else: best-effort.
    if hasattr(core, "_t"):
        spans = _text_spans(core._t, bare_attr)
        return spans, None, pad_left, inner_attr, getattr(core, "_align", None)
    return [], None, pad_left, inner_attr, None

def _text_spans(parts, line_attr):
    spans = []
    if isinstance(parts, str):
        parts = [(line_attr, parts)]
    if not isinstance(parts, list):
        return spans
    for p in parts:
        if isinstance(p, str):
            # Bare string (e.g. the heading indent) uses the line's attr.
            st = _style_from_key(line_attr) if isinstance(line_attr, tuple) else \
                 {"fg": "default", "bg": "default", "bold": False,
                  "underline": False, "italic": False}
            spans.append({"text": p, **st, "link": None, "field": None})
        elif isinstance(p, tuple) and len(p) == 2:
            attr, text = p
            if isinstance(attr, _LinkStub):
                st = _style_from_key(attr.style_key)
                spans.append({"text": text, **st,
                              "link": {"label": text,
                                       "url": attr.link_target or "",
                                       "fields": _link_fields_str(attr.link_fields)},
                              "field": None})
            elif isinstance(attr, tuple):
                st = _style_from_key(attr)
                spans.append({"text": text, **st, "link": None, "field": None})
            else:
                spans.append({"text": text, "fg": "default", "bg": "default",
                              "bold": False, "underline": False, "italic": False,
                              "link": None, "field": None})
    return spans

def _link_fields_str(lf):
    if not lf:
        return ""
    if isinstance(lf, list):
        return "|".join(lf)
    return str(lf)

def _field_spans(am):
    w = am._w
    style = _style_from_key(am._attr) if isinstance(am._attr, tuple) else \
            {"fg": "default", "bg": "default", "bold": False,
             "underline": False, "italic": False}
    try:
        name = getattr(w, "field_name", None)
        value = getattr(w, "field_value", None)
        is_readline = isinstance(w, _ReadlineEditCls) if _ReadlineEditCls else False
        if is_readline:
            data = getattr(w, "edit_text", None)
            if data is None:
                data = getattr(w, "_edit_text", "") or ""
            masked = getattr(w, "mask", None) is not None
            f = {"name": name or "", "type": "field", "data": data or "",
                 "value": "", "width": 0, "masked": masked, "prechecked": False}
            return [{"text": data or "", **style, "link": None, "field": f}]
        if isinstance(w, _CheckBox):
            label = w._args[0] if w._args else ""
            f = {"name": name or "", "type": "checkbox", "data": label,
                 "value": value or "", "width": 0, "masked": False,
                 "prechecked": bool(w._kwargs.get("state", False))}
            return [{"text": label, **style, "link": None, "field": f}]
        if isinstance(w, _RadioButton):
            label = w._args[1] if len(w._args) > 1 else ""
            f = {"name": name or "", "type": "radio", "data": label,
                 "value": value or "", "width": 0, "masked": False,
                 "prechecked": bool(w._kwargs.get("state", False))}
            return [{"text": label, **style, "link": None, "field": f}]
    except Exception:
        pass
    return []

def render(markup, theme="dark", source="-"):
    # theme arrives as the string "dark"/"light"; convert to the integer
    # constant ensure_selected_styles compares against, and seed SELECTED_STYLES
    # from the matching table. markup_to_attrmaps re-runs ensure_selected_styles
    # (which reads config["textui"]["theme"]), so the integer must be set first.
    theme_int = _THEME_DARK if theme == "dark" else _THEME_LIGHT
    _FAKE_APP.config["textui"]["theme"] = theme_int
    M.SELECTED_STYLES = M.STYLES_DARK if theme == "dark" else M.STYLES_LIGHT
    heading_levels = _heading_levels()

    fg, bg = _page_colors(markup)
    am_list = M.markup_to_attrmaps(strip_modifiers(markup),
                                   url_delegate=_DummyDelegate(),  # non-None => links
                                   fg_color=fg, bg_color=bg)
    header_rows = set(getattr(am_list, "header_rows", []) or [])
    # anchors is {slug: row_index}; invert to per-line slug for the JSON schema.
    anchors = getattr(am_list, "anchors", {}) or {}
    anchor_for_line = {}
    for slug, row_idx in anchors.items():
        anchor_for_line.setdefault(row_idx, slug)

    lines = []
    for idx, am in enumerate(am_list):
        if not isinstance(am, _AttrMap):
            lines.append({"index": idx, "align": "left", "indent": 0,
                          "heading_level": 0, "divider": False, "spans": []})
            continue
        spans, divchar, indent, inner_attr, align_raw = _extract_spans(
            am._w, am._attr)
        align = _align_name(align_raw) if align_raw else "left"
        heading_level = 0
        if idx in header_rows:
            # The heading style is the innermost AttrMap's style key (set by the
            # heading branch's style_to_state(headingN)). Map its normalized
            # fg/bg to a level; fall back to 1 only if unrecognized.
            heading_level = 1
            if isinstance(inner_attr, tuple) and len(inner_attr) >= 2:
                key = (_norm_color(inner_attr[0]), _norm_color(inner_attr[1]))
                heading_level = heading_levels.get(key, 1)
        line = {"index": idx, "align": align, "indent": indent,
                "heading_level": heading_level, "divider": divchar is not None,
                "spans": spans}
        if divchar is not None:
            line["divider_char"] = divchar
        anchor = anchor_for_line.get(idx)
        if anchor:
            line["anchor"] = anchor
        lines.append(line)
    return {"source": source, "theme": theme, "lines": lines}

def main():
    ap = argparse.ArgumentParser(description="Headless Python micron renderer (parity).")
    ap.add_argument("file", help=".mu file, or '-' for stdin")
    ap.add_argument("--theme", default="dark", choices=["dark", "light"])
    ap.add_argument("--json", action="store_true", help="emit structured JSON (default)")
    ap.add_argument("--source", default=None, help="source label for the JSON doc")
    args = ap.parse_args()

    if args.file == "-":
        markup = sys.stdin.read()
        src = args.source or "-"
    else:
        with open(args.file, "r", encoding="utf-8") as f:
            markup = f.read()
        src = args.source or args.file

    try:
        doc = render(markup, theme=args.theme, source=src)
    except Exception as e:
        sys.stderr.write("render-mu: %s\n" % e)
        raise
    json.dump(doc, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")

if __name__ == "__main__":
    main()