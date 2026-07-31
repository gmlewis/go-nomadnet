#!/usr/bin/env python3
# micron_parseline.py — extract MicronParser.parse_line structural output.
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# Copies parse_line verbatim from nomadnet/ui/textui/MicronParser.py:220-416
# and stubs the urwid widget constructors + render_table to structural
# markers, reusing the stubbed make_output from micron_inline.py. Captures
# the line-level classification (heading level, divider char, comment,
# section-reset, table start config, partial) as JSON golden values.
#
# Reads fixture lines from stdin (one micron markup line per line), emits one
# JSON object per line: {"input": ..., "output": [ {kind,...} ]}.
import json
import re
import sys

import micron_inline as M

SECTION_INDENT = 2
MAX_TABLE_WIDTH = 100
SELECTED_STYLES = M.STYLES_DARK

_MICRON_STRIP_RE = re.compile(
    r"`[FB]T[0-9a-fA-F]{6}"
    r"|`[FB][0-9a-fA-F]{3}"
    r"|`:[A-Za-z0-9_\-]*"
    r"|`[!*_=fbacrl`<>{]"
)


def slugify_micron(text):
    if text is None:
        return ""
    stripped = _MICRON_STRIP_RE.sub("", text)
    s = re.sub(r"[^A-Za-z0-9]+", "-", stripped).strip("-").lower()
    return s


def left_indent(state):
    return (state["depth"] - 1) * SECTION_INDENT


def right_indent(state):
    return (state["depth"] - 1) * SECTION_INDENT


def state_to_style(state):
    return {"fg": state["fg_color"], "bg": state["bg_color"], "bold": state["formatting"]["bold"],
            "underline": state["formatting"]["underline"], "italic": state["formatting"]["italic"]}


def style_to_state(style, state):
    if style["fg"] is not None: state["fg_color"] = style["fg"]
    if style["bg"] is not None: state["bg_color"] = style["bg"]
    if style["bold"] is not None: state["formatting"]["bold"] = style["bold"]
    if style["underline"] is not None: state["formatting"]["underline"] = style["underline"]
    if style["italic"] is not None: state["formatting"]["italic"] = style["italic"]


# --- urwid stubs ---------------------------------------------------------

class _Urwid(object):
    PACK = "PACK"

    def Text(self, parts, align=None):
        return {"_w": "text", "parts": parts, "align": align}

    def Divider(self, char):
        return {"_w": "divider", "char": char}

    def Padding(self, inner, left=None, right=None):
        return {"_w": "padding", "inner": inner, "left": left, "right": right}

    def AttrMap(self, inner, style):
        return {"_w": "attrmap", "inner": inner, "style": style}

    def Columns(self, widgets, dividechars=0):
        return {"_w": "columns", "widgets": widgets, "dividechars": dividechars}


urwid = _Urwid()


class _LinkClass(object):
    def __init__(self, parts, align=None, delegate=None):
        self.parts = parts
        self.align = align
        self.delegate = delegate
        self.in_columns = False


LINK_CLASS = _LinkClass


def render_table(lines, state, url_delegate):
    return {"_w": "table_render", "rows": list(lines),
            "align": state.get("table_align"), "maxwidth": state.get("table_maxwidth")}


def parse_partial(line):
    try:
        endpos = line.find("}")
        if endpos == -1:
            return None
        partial_data = line[0:endpos]
        partial_id = None
        partial_components = partial_data.split("`")
        if len(partial_components) == 1:
            partial_url = partial_components[0]
            partial_refresh = None
            partial_fields = ""
        elif len(partial_components) == 2:
            partial_url = partial_components[0]
            partial_refresh = float(partial_components[1])
            partial_fields = ""
        elif len(partial_components) == 3:
            partial_url = partial_components[0]
            partial_refresh = float(partial_components[1])
            partial_fields = partial_components[2]
        else:
            partial_url = ""
            partial_fields = ""
            partial_refresh = None
        if partial_refresh is not None and partial_refresh < 1:
            partial_refresh = None
        pf = partial_fields.split("|")
        if len(pf) > 0:
            partial_fields = pf
            for f in pf:
                if f.startswith("pid="):
                    pcs = f.split("=")
                    partial_id = pcs[1]
        else:
            partial_fields = [partial_fields]
        return {
            "type": "partial",
            "url": partial_url,
            "refresh": partial_refresh,
            "fields": partial_fields,
            "id": partial_id,
        }
    except Exception:
        return None


def make_output(state, line, url_delegate, pre_escape=False):
    return M.make_output(state, line, url_delegate, pre_escape)


def make_style(state):
    return M.make_style(state)


# --- parse_line verbatim from MicronParser.py:220-416 -------------------

def parse_line(line, state, url_delegate):
    pre_escape = False
    if len(line) > 0:
        first_char = line[0]

        if len(line) == 2 and line == "`=":
            state["literal"] ^= True
            return None

        if not state["literal"]:
            if first_char == ">" and "`<" in line:
                line = line.lstrip(">")
                first_char = line[0]

            if first_char == "\\":
                line = line[1:]
                pre_escape = True

            elif first_char == "#":
                return None

            if line.startswith("`t"):
                line = line[2:]
                align = line[0] if len(line) and line[0] in ["l", "c", "r"] else None
                max_width = None
                if align: line = line[1:]
                if len(line):
                    try: max_width = int(line)
                    except: pass

                if state["table_mode"]:
                    widgets = render_table(state["table_buffer"], state, url_delegate)
                    state["table_mode"] = False
                    state["table_buffer"] = []
                    state["table_align"] = None
                    state["table_maxwidth"] = MAX_TABLE_WIDTH
                    return widgets

                else:
                    state["table_mode"] = True
                    state["table_buffer"] = []
                    state["table_align"] = align
                    state["table_maxwidth"] = max_width
                    return None

            if state["table_mode"]:
                state["table_buffer"].append(line)
                return None

            elif line.startswith("`{"):
                return parse_partial(line[2:])

            elif first_char == "<":
                state["depth"] = 0
                return parse_line(line[1:], state, url_delegate)

            elif first_char == ">":
                i = 0
                while i < len(line) and line[i] == ">":
                    i += 1
                    state["depth"] = i

                    for j in range(1, i + 1):
                        wanted_style = "heading" + str(i)
                        if wanted_style in SELECTED_STYLES:
                            style = SELECTED_STYLES[wanted_style]

                line = line[state["depth"]:]
                if len(line) > 0:
                    latched_style = state_to_style(state)
                    style_to_state(style, state)

                    heading_style = make_style(state)
                    output = make_output(state, line, url_delegate)

                    style_to_state(latched_style, state)

                    slug = slugify_micron(line)
                    if slug:
                        state.setdefault("pending_anchors", []).append(slug)
                    state["_header_pending"] = True

                    if len(output) > 0:
                        first_style = output[0][0]

                        heading_style = first_style
                        output.insert(0, " " * left_indent(state))
                        return [urwid.AttrMap(urwid.Text(output, align=state["align"]), heading_style)]
                    else:
                        return None
                else:
                    return None

            elif first_char == "-":
                if len(line) == 2:
                    divider_char = line[1]
                    if ord(divider_char) < 32:
                        divider_char = "─"
                else:
                    divider_char = "─"
                if state["depth"] == 0:
                    return [urwid.Divider(divider_char)]
                else:
                    return [urwid.Padding(urwid.Divider(divider_char), left=left_indent(state), right=right_indent(state))]

        output = make_output(state, line, url_delegate, pre_escape)

        if output != None:
            text_only = True
            for o in output:
                if not isinstance(o, tuple):
                    text_only = False
                    break

            if not text_only:
                widgets = []
                for o in output:
                    if isinstance(o, tuple):
                        if url_delegate != None:
                            tw = LINK_CLASS(o, align=state["align"], delegate=url_delegate)
                            tw.in_columns = True
                        else:
                            tw = urwid.Text(o, align=state["align"])
                        widgets.append((urwid.PACK, tw))
                    else:
                        # Field/checkbox/radio: capture as a marker carrying
                        # the dict and a fixed width for fields.
                        if o["type"] == "field":
                            widgets.append((o["width"], {"_w": "field", "data": o}))
                        elif o["type"] in ("checkbox", "radio"):
                            widgets.append((None, {"_w": o["type"], "data": o}))
                columns_widget = urwid.Columns(widgets, dividechars=0)
                text_widget = columns_widget
            else:
                if url_delegate != None:
                    text_widget = LINK_CLASS(output, align=state["align"], delegate=url_delegate)
                else:
                    text_widget = urwid.Text(output, align=state["align"])

            if state["depth"] == 0:
                return [text_widget]
            else:
                return [urwid.Padding(text_widget, left=left_indent(state), right=right_indent(state))]
        else:
            return None
    else:
        return None


def simplify(widgets, state):
    """Reduce stubbed widgets to a comparable structural description."""
    if widgets is None:
        return None
    out = []
    for w in widgets:
        if not isinstance(w, dict):
            out.append({"kind": "raw", "value": str(w)})
            continue
        kind = w.get("_w")
        if kind == "divider":
            out.append({"kind": "divider", "char": w["char"], "depth": state["depth"]})
        elif kind == "padding":
            inner = w["inner"]
            if isinstance(inner, dict) and inner.get("_w") == "divider":
                out.append({"kind": "divider", "char": inner["char"], "depth": state["depth"], "padded": True})
            elif isinstance(inner, dict) and inner.get("_w") == "text":
                out.append({"kind": "inline", "depth": state["depth"],
                            "content": M.convert(inner["parts"])})
            elif isinstance(inner, dict) and inner.get("_w") == "columns":
                out.append({"kind": "columns", "depth": state["depth"], "widgets": inner["widgets"]})
            else:
                out.append({"kind": "padding", "inner": str(inner), "depth": state["depth"]})
        elif kind == "text":
            out.append({"kind": "inline", "depth": state["depth"], "content": M.convert(w["parts"])})
        elif kind == "attrmap":
            inner = w["inner"]
            if isinstance(inner, dict) and inner.get("_w") == "text":
                # Heading: content is the make_output parts (with a leading
                # indent string inserted). Level = state depth.
                content = M.convert([p for p in inner["parts"] if not (isinstance(p, str))])
                out.append({"kind": "heading", "level": state["depth"], "content": content})
            else:
                out.append({"kind": "attrmap", "inner": str(inner)})
        elif kind == "linkclass":
            out.append({"kind": "inline", "depth": state["depth"], "content": M.convert(w.parts)})
        elif kind == "table_render":
            out.append({"kind": "table", "rows": w["rows"], "align": w["align"], "maxwidth": w["maxwidth"]})
        elif kind == "columns":
            out.append({"kind": "columns", "depth": state["depth"], "widgets": w["widgets"]})
        else:
            out.append({"kind": "raw", "value": str(w)})
    return out


def run(line):
    state = M.default_state()
    widgets = parse_line(line, state, url_delegate=True)
    if isinstance(widgets, dict) and widgets.get("type") == "partial":
        return {"kind": "partial", "url": widgets["url"], "refresh": widgets["refresh"],
                "fields": widgets["fields"], "id": widgets["id"]}
    return simplify(widgets, state)


if __name__ == "__main__":
    lines = sys.stdin.read().split("\n")
    results = []
    for ln in lines:
        if ln == "":
            continue
        results.append({"input": ln, "output": run(ln)})
    print(json.dumps(results, indent=2))