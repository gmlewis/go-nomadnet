#!/usr/bin/env python3
# micron_inline.py — extract MicronParser.make_output structural output.
#
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# Copied verbatim from nomadnet/ui/textui/MicronParser.py (make_output, lines
# 593-855) and default_state (39-68), with the urwid/app-dependent helpers
# make_style, make_part, LinkSpec, colormode and SYNTH_SPECS stubbed so the
# pure parsing structure (text spans, field dicts, link specs) can be captured
# as JSON golden values for Go parity tests — without importing urwid (which
# does not import on hosts missing GLib).
#
# Reads fixture lines from stdin (one micron markup line per line), emits one
# JSON object per line: {"input": ..., "output": [ {kind,text|link|field...} ]}.
import json
import sys

DEFAULT_FG_DARK = "ddd"
DEFAULT_FG = DEFAULT_FG_DARK
DEFAULT_BG = "default"

STYLES_DARK = {
    "plain":    {"fg": DEFAULT_FG_DARK, "bg": DEFAULT_BG, "bold": False, "underline": False, "italic": False},
    "heading1": {"fg": "222", "bg": "bbb", "bold": False, "underline": False, "italic": False},
    "heading2": {"fg": "111", "bg": "999", "bold": False, "underline": False, "italic": False},
    "heading3": {"fg": "000", "bg": "777", "bold": False, "underline": False, "italic": False},
}

SELECTED_STYLES = STYLES_DARK
SYNTH_SPECS = {}


def ensure_selected_styles():
    pass


def default_state(fg=None, bg=None):
    global SELECTED_STYLES
    ensure_selected_styles()
    if fg is None:
        fg = SELECTED_STYLES["plain"]["fg"]
    if bg is None:
        bg = DEFAULT_BG
    state = {
        "literal": False,
        "table_mode": False,
        "table_buffer": [],
        "table_align": None,
        "table_maxwidth": 100,
        "depth": 0,
        "fg_color": fg,
        "bg_color": bg,
        "formatting": {"bold": False, "underline": False, "italic": False, "strikethrough": False, "blink": False},
        "default_align": "left",
        "align": "left",
        "default_fg": fg,
        "default_bg": bg,
        "anchors": {},
        "pending_anchors": [],
        "header_rows": [],
    }
    return state


# --- Stubs for urwid/app-dependent helpers -------------------------------

class LinkSpec(object):
    def __init__(self, link_target, orig_spec, cm=256):
        self.link_target = link_target
        self.link_fields = None


class _App(object):
    class _UI(object):
        colormode = 256
    ui = _UI()


def _get_shared_instance():
    return _App()


def make_style(state):
    # Return a deterministic placeholder encoding the relevant state so the
    # structure can be compared without urwid. Format: fg|bg|b|u|i.
    fg = state["fg_color"]
    bg = state["bg_color"]
    fmt = state["formatting"]
    return "fg=%s|bg=%s|b=%s|u=%s|i=%s" % (fg, bg, fmt["bold"], fmt["underline"], fmt["italic"])


def make_part(state, part):
    return (make_style(state), part)


# Patch nomadnet reference used inside make_output's link branch.
import types
nomadnet = types.SimpleNamespace(
    NomadNetworkApp=types.SimpleNamespace(get_shared_instance=_get_shared_instance)
)


# --- make_output verbatim from MicronParser.py:593-855 -------------------

def make_output(state, line, url_delegate, pre_escape=False):
    output = []
    if state["literal"]:
        if line == "\\`=":
            line = "`="
        output.append(make_part(state, line))
    else:
        part = ""
        mode = "text"
        escape = pre_escape
        skip = 0

        for i in range(0, len(line)):
            c = line[i]
            if skip > 0:
                skip -= 1
            else:
                if mode == "formatting":
                    if c == "_":
                        state["formatting"]["underline"] ^= True
                    elif c == "!":
                        state["formatting"]["bold"] ^= True
                    elif c == "*":
                        state["formatting"]["italic"] ^= True
                    elif c == "F":
                        if len(line) >= i+4:
                            if line[i+1] == "T" and len(line) >= i+8:
                                color = line[i+2:i+8]
                                state["fg_color"] = color
                                skip = 7
                            else:
                                color = line[i+1:i+4]
                                state["fg_color"] = color
                                skip = 3
                    elif c == "f":
                        state["fg_color"] = state["default_fg"]
                    elif c == "B":
                        if len(line) >= i+4:
                            if line[i+1] == "T" and len(line) >= i+8:
                                color = line[i+2:i+8]
                                state["bg_color"] = color
                                skip = 7
                            else:
                                color = line[i+1:i+4]
                                state["bg_color"] = color
                                skip = 3
                    elif c == "b":
                        state["bg_color"] = state["default_bg"]
                    elif c == "`":
                        state["formatting"]["bold"]      = False
                        state["formatting"]["underline"] = False
                        state["formatting"]["italic"]    = False
                        state["fg_color"] = state["default_fg"]
                        state["bg_color"] = state["default_bg"]
                        state["align"] = state["default_align"]
                    elif c == "c":
                        if state["align"] != "center": state["align"] = "center"
                    elif c == "l":
                        if state["align"] != "left": state["align"] = "left"
                    elif c == "r":
                        if state["align"] != "right": state["align"] = "right"
                    elif c == "a":
                        state["align"] = state["default_align"]

                    elif c == ":":
                        name_start = i + 1
                        name_end = name_start
                        while name_end < len(line) and (line[name_end].isalnum() or line[name_end] in "_-"):
                            name_end += 1
                        anchor_name = line[name_start:name_end]
                        if anchor_name:
                            state.setdefault("pending_anchors", []).append(anchor_name)
                        skip = (name_end - i) - 1
                        if skip < 0: skip = 0

                    elif c == '<':
                        if len(part) > 0:
                            output.append(make_part(state, part))
                            part = ""
                        try:
                            field_start = i + 1
                            backtick_pos = line.find('`', field_start)
                            if backtick_pos == -1:
                                pass
                            else:
                                field_content = line[field_start:backtick_pos]
                                field_masked = False
                                field_width = 24
                                field_type = "field"
                                field_name = field_content
                                field_value = ""
                                field_data = ""
                                field_prechecked = False

                                if '|' in field_content:
                                    f_components = field_content.split('|')
                                    field_flags = f_components[0]
                                    field_name = f_components[1]

                                    if '^' in field_flags:
                                        field_type = "radio"
                                        field_flags = field_flags.replace("^", "")
                                    elif '?' in field_flags:
                                        field_type = "checkbox"
                                        field_flags = field_flags.replace("?", "")
                                    elif '!' in field_flags:
                                        field_flags = field_flags.replace("!", "")
                                        field_masked = True

                                    if len(field_flags) > 0:
                                        try:
                                            field_width = min(int(field_flags), 256)
                                        except ValueError:
                                            pass

                                    if len(f_components) > 2:
                                        field_value = f_components[2]
                                    else:
                                        field_value = ""
                                    if len(f_components) > 3:
                                        if f_components[3] == '*':
                                            field_prechecked = True

                                else:
                                    field_name = field_content
                                    field_type = "field"
                                    field_masked = False
                                    field_width = 24
                                    field_value = ""
                                    field_prechecked = False

                                field_end = line.find('>', backtick_pos)
                                if field_end == -1:
                                    pass
                                else:
                                    field_data = line[backtick_pos+1:field_end]

                                    if field_type in ["checkbox", "radio"]:
                                        output.append({
                                            "type": field_type,
                                            "name": field_name,
                                            "value": field_value if field_value else field_data,
                                            "label": field_data,
                                            "prechecked": field_prechecked,
                                            "style": make_style(state)
                                        })
                                    else:
                                        output.append({
                                            "type": "field",
                                            "name": field_name,
                                            "width": field_width,
                                            "masked": field_masked,
                                            "data": field_data,
                                            "style": make_style(state)
                                        })
                                    skip = field_end - i
                        except Exception as e:
                            pass

                    elif c == "[":
                        endpos = line[i:].find("]")
                        if endpos == -1:
                            pass
                        else:
                            link_data = line[i+1:i+endpos]
                            skip = endpos

                            link_components = link_data.split("`")
                            if len(link_components) == 1:
                                link_label = ""
                                link_fields = ""
                                link_url = link_data
                            elif len(link_components) == 2:
                                link_label = link_components[0]
                                link_url = link_components[1]
                                link_fields = ""
                            elif len(link_components) == 3:
                                link_label = link_components[0]
                                link_url = link_components[1]
                                link_fields = link_components[2]
                            else:
                                link_url = ""
                                link_label = ""
                                link_fields = ""

                            if len(link_url) != 0:
                                if link_label == "":
                                    link_label = link_url

                                if len(part) > 0:
                                    output.append(make_part(state, part))

                                cm = nomadnet.NomadNetworkApp.get_shared_instance().ui.colormode

                                specname = make_style(state)
                                speclist = SYNTH_SPECS.get(specname)
                                if speclist is None:
                                    speclist = ["S0", "S1", "S2", "S3", "S4"]
                                    SYNTH_SPECS[specname] = speclist

                                if cm == 1:
                                    orig_spec = speclist[0]
                                elif cm == 16:
                                    orig_spec = speclist[1]
                                elif cm == 88:
                                    orig_spec = speclist[2]
                                elif cm == 256:
                                    orig_spec = speclist[3]
                                elif cm == 2**24:
                                    orig_spec = speclist[4]

                                if url_delegate != None:
                                    linkspec = LinkSpec(link_url, orig_spec, cm=cm)
                                    if link_fields != "":
                                        lf = link_fields.split("|")
                                        if len(lf) > 0:
                                            linkspec.link_fields = lf

                                    output.append((linkspec, link_label))
                                else:
                                    output.append(make_part(state, link_label))

                    mode = "text"
                    if len(part) > 0:
                        output.append(make_part(state, part))

                elif mode == "text":
                    if c == "\\":
                        if escape:
                            part += c
                            escape = False
                        else:
                            escape = True
                    elif c == "`":
                        if escape:
                            part += c
                            escape = False
                        else:
                            mode = "formatting"
                            if len(part) > 0:
                                output.append(make_part(state, part))
                                part = ""
                    else:
                        part += c
                        escape = False

            if i == len(line)-1:
                if len(part) > 0:
                    output.append(make_part(state, part))

    if len(output) > 0:
        return output
    else:
        return None


def serialize(part):
    style, text = part
    return {"kind": "text", "style": style, "text": text}


def convert(output):
    if output is None:
        return None
    result = []
    for o in output:
        if isinstance(o, tuple):
            # Could be a text part (style, str) or a link spec (LinkSpec, label).
            if isinstance(o[0], LinkSpec):
                ls = o[0]
                result.append({
                    "kind": "link",
                    "url": ls.link_target,
                    "label": o[1],
                    "fields": ls.link_fields,
                })
            else:
                style, text = o
                result.append({"kind": "text", "style": style, "text": text})
        elif isinstance(o, dict):
            d = {"kind": o["type"]}
            if "name" in o:
                d["name"] = o["name"]
            if "value" in o:
                d["value"] = o["value"]
            if "label" in o:
                d["label"] = o["label"]
            if "prechecked" in o:
                d["prechecked"] = o["prechecked"]
            if "width" in o:
                d["width"] = o["width"]
            if "masked" in o:
                d["masked"] = o["masked"]
            if "data" in o:
                d["data"] = o["data"]
            result.append(d)
    return result


def run(line, url_delegate=True):
    state = default_state()
    out = make_output(state, line, url_delegate=url_delegate)
    return convert(out)


if __name__ == "__main__":
    lines = sys.stdin.read().split("\n")
    results = []
    for ln in lines:
        if ln == "":
            continue
        # split off an optional leading marker; we pass the raw line.
        results.append({"input": ln, "output": run(ln)})
    print(json.dumps(results, indent=2))