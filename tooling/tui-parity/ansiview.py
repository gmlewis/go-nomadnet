#!/usr/bin/env python3
# ansiview.py — render a tmux "capture-pane -e" file so TUI styling is visible.
#
# Copyright 2026 Glenn Lewis. All rights reserved.
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# tmux `capture-pane -p -e` embeds ANSI SGR escape sequences (colors, bold,
# reverse-video, ...). A plain `cat` of that file shows the text but hides the
# styling — which is exactly where urwid/tview express focus highlights, trust
# colors, headings, links, etc. This decoder walks the SGR state machine and
# annotates styled runs so you can see, in plain text, WHAT style each cell has
# and WHERE the focus highlight is — without a headed terminal.
#
# Modes:
#   default        wrap each styled run as  {style}text   (style omitted if default)
#   --plain        strip all styling, print just the text (trimmed)
#   --focus        print only rows that contain a non-default background color
#                  (i.e. likely focused/highlighted list rows or menu items),
#                  prefixed with the row number and the bg color found
#   --json         emit one JSON object per row: {row, text, styles:[{text,fg,bg,...}]}
#
# Style marker format: fg=... bg=... plus flags B(bold) I(italic) U(underline) R(reverse).
# Colors print as ('c',r,g,b) for 24-bit, ('i',n) for 256-palette index, or int for 16-color.
#
# Usage:
#   python3 ansiview.py captures/guide_135x32_00_esc.txt
#   python3 ansiview.py --focus captures/network_135x32_03_esc.txt
#   python3 ansiview.py --plain captures/go_135x32_00_esc.txt

import sys, re, json, argparse

AP = argparse.ArgumentParser(description="Decode tmux capture-pane -e styling.")
AP.add_argument("file")
AP.add_argument("--plain", action="store_true", help="strip styles, print text only")
AP.add_argument("--focus", action="store_true", help="print only rows with a background color")
AP.add_argument("--json", action="store_true", help="emit JSON per row")
args = AP.parse_args()

data = open(args.file, "r", errors="replace").read()

def parse_sgr_params(s):
    out = []
    for part in s.split(";"):
        if part == "":
            out.append(None)
        else:
            try:
                out.append(int(part))
            except ValueError:
                out.append(None)
    return out

tokens = re.findall(r"\x1b\[[0-9;]*m|\x1b\][^\x07\x1b]*\x07|[^\x1b]+", data)

fg = None; bg = None; bold = False; reverse = False
italic = False; underline = False

def apply(params):
    global fg, bg, bold, reverse, italic, underline
    if not params:
        fg = None; bg = None; bold = False; reverse = False
        italic = False; underline = False
        return
    i = 0
    while i < len(params):
        p = params[i]
        if p is None or p == 0:
            fg = None; bg = None; bold = False; reverse = False
            italic = False; underline = False
        elif p == 1: bold = True
        elif p == 3: italic = True
        elif p == 4: underline = True
        elif p == 7: reverse = True
        elif p == 22: bold = False
        elif p == 23: italic = False
        elif p == 24: underline = False
        elif p == 27: reverse = False
        elif 30 <= p <= 37: fg = p - 30
        elif p == 38:
            if i+1 < len(params) and params[i+1] == 2:
                fg = ("c", params[i+2], params[i+3], params[i+4]); i += 4
            elif i+1 < len(params) and params[i+1] == 5:
                fg = ("i", params[i+2]); i += 2
        elif p == 39: fg = None
        elif 40 <= p <= 47: bg = p - 40
        elif p == 48:
            if i+1 < len(params) and params[i+1] == 2:
                bg = ("c", params[i+2], params[i+3], params[i+4]); i += 4
            elif i+1 < len(params) and params[i+1] == 5:
                bg = ("i", params[i+2]); i += 2
        elif p == 49: bg = None
        elif 90 <= p <= 97: fg = p - 90 + 8
        elif 100 <= p <= 107: bg = p - 100 + 8
        i += 1

def marker():
    m = ""
    if reverse: m += "R"
    if bold: m += "B"
    if italic: m += "I"
    if underline: m += "U"
    if fg is not None: m += "fg=" + repr(fg)
    if bg is not None: m += "bg=" + repr(bg)
    return m

# Build rows of (char, marker).
rows = []; row = []
for tok in tokens:
    if tok.startswith("\x1b]"):
        continue  # OSC
    if tok.startswith("\x1b[") and tok.endswith("m"):
        apply(parse_sgr_params(tok[2:-1])); continue
    if tok.startswith("\x1b["):
        continue
    for ch in tok:
        if ch == "\n":
            rows.append(row); row = []
        else:
            row.append((ch, marker()))
rows.append(row)

def render_default(row):
    s = ""; cur_m = None; run = ""
    for ch, m in row:
        if m != cur_m:
            if run:
                s += ("{" + cur_m + "}" + run) if cur_m else run
            run = ""; cur_m = m if m else None
        run += ch
    if run:
        s += ("{" + cur_m + "}" + run) if cur_m else run
    return s

def render_plain(row):
    return "".join(ch for ch, _ in row).rstrip()

if args.json:
    out = []
    for idx, row in enumerate(rows):
        runs = []; cur_m = None; run = ""
        for ch, m in row:
            if m != cur_m:
                if run:
                    runs.append({"text": run, "style": cur_m or ""})
                run = ""; cur_m = m if m else None
            run += ch
        if run:
            runs.append({"text": run, "style": cur_m or ""})
        out.append({"row": idx, "text": render_plain(row), "styles": runs})
    print(json.dumps(out, indent=1))
elif args.focus:
    for idx, row in enumerate(rows):
        bgs = sorted({m for _, m in row if "bg=" in m})
        if bgs:
            txt = render_plain(row)
            if txt.strip() == "":
                continue
            print(f"{idx:3d} bg={bgs}  {txt}")
elif args.plain:
    for row in rows:
        print(render_plain(row))
else:
    for row in rows:
        print(render_default(row))