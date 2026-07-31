#!/usr/bin/env python3
# summary.py — structural summary of a tmux capture for parity comparison.
#
# Copyright 2026 Glenn Lewis. All rights reserved.
# Licensed under the GPL v3 (see repo LICENSE / copyright_header.txt).
#
# Reduces a "capture-pane -e" frame to the handful of facts that matter for
# behavioral parity: what's on the menu bar, what's on the shortcut/footer bar,
# which box-drawing border style is used, which rows carry a focus highlight
# (a non-default background), and whether the body text is suspiciously all-bold.
#
# This is the quick "did the port regress?" check. Run it on a frame from the
# original and the same frame from the Go port and compare the summaries.
#
# Usage:
#   python3 summary.py captures/orig_guide_135x32_00_esc.txt
#   python3 summary.py --json-from ansiview.json
#
# Internally shells out to ansiview.py --json (same directory) unless --json-from
# is given (the output of `ansiview.py --json`).

import sys, os, json, subprocess, argparse, re

AP = argparse.ArgumentParser(description="Structural summary of a tmux capture.")
AP.add_argument("file", nargs="?", help="capture-pane -e file")
AP.add_argument("--json-from", help="precomputed ansiview.py --json output file")
args = AP.parse_args()

if args.json_from:
    rows = json.load(open(args.json_from))
else:
    if not args.file:
        AP.error("need a capture file or --json-from")
    here = os.path.dirname(os.path.abspath(__file__))
    av = os.path.join(here, "ansiview.py")
    rows = json.loads(subprocess.check_output([sys.executable, av, args.file, "--json"]))

# rows: list of {row, text, styles:[{text, style}]}

def has_bg(style): return "bg=" in style
def has_bold(style): return "B" in style and "bg=" not in style  # bold flag, not a bg color named B...

# First non-empty row = menu bar (both toolkits render the menu on row 0/1).
menu = next((r["text"] for r in rows if r["text"].strip()), "")

# Footer/shortcut bar = last non-empty row that is NOT a box border.
border_chars = set("┌┐└┘├┤┬┴┼─│╔╗╚╝╠╣╦╩╬═║╭╮╰╯")
def is_border_line(t):
    s = "".join(c for c in t if not c.isspace())
    return len(s) > 0 and all(c in border_chars for c in s)

# A row's bg-fill fraction: what fraction of its non-space cells are covered by a
# run that has a background color. The menu bar and shortcut/footer bar are both
# full-width bg-filled lines; body text is not.
def bg_fill_frac(row):
    total = 0; filled = 0
    for st in row["styles"]:
        cells = sum(1 for c in st["text"] if not c.isspace())
        if cells == 0:
            continue
        total += cells
        if has_bg(st["style"]):
            filled += cells
    return (filled / total) if total else 0.0

# Footer / shortcut bar = last non-empty, non-border row whose bg fill covers
# > 50% of its width (the shortcutbar style fills the whole line). Fall back to
# the last non-border non-empty row if none qualifies.
footer = ""
fallback_footer = ""
for r in reversed(rows):
    t = r["text"]
    if not t.strip() or is_border_line(t):
        continue
    if not fallback_footer:
        fallback_footer = t
    if bg_fill_frac(r) > 0.5:
        footer = t
        break
if not footer:
    footer = fallback_footer

# Detect dominant box border style.
borders = {"single": "┌", "double": "╔", "rounded": "╭"}
border_style = "none"
for name, ch in borders.items():
    if any(ch in r["text"] for r in rows):
        border_style = name
        break

# Menu items: prefer bracketed `[ Name ]` groups (original); else split on 2+
# spaces (Go port bare words). Drop a leading non-bracketed indicator glyph.
def parse_menu_items(text):
    bracketed = re.findall(r"\[\s*([^\[\]]+?)\s*\]", text)
    if bracketed:
        return bracketed
    return [m for m in re.split(r"\s{2,}", text.strip()) if m.strip()]

# Focus-highlight rows: rows where some run has a bg color (excluding pure border
# lines and the full-width menu/footer bars, which are styled but not "focus").
focus_rows = []
for r in rows:
    t = r["text"]
    if not t.strip() or is_border_line(t):
        continue
    if t == menu or t == footer:
        continue
    bgs = sorted({st["style"] for st in r["styles"] if has_bg(st["style"])})
    if bgs:
        focus_rows.append((r["row"], bgs, t.rstrip()))

# All-bold body heuristic: among content runs in body rows (excluding menu,
# footer, pure border rows, and runs that are only border glyphs/whitespace),
# what fraction are marked bold. The original uses normal body_text for
# paragraphs (bold only on headings/links), so a high fraction signals the
# "everything is bold" rendering bug. Run-level (not row-level) so it still
# catches two-column pages where one column is bold.
def is_content_run(text):
    return any((not c.isspace()) and (c not in border_chars) for c in text)

body_runs = []
for r in rows:
    t = r["text"]
    if not t.strip() or is_border_line(t):
        continue
    if t == menu or t == footer:
        continue
    for st in r["styles"]:
        if is_content_run(st["text"]):
            body_runs.append(st)
bold_frac = (sum(1 for st in body_runs if "B" in st["style"]) / len(body_runs)) if body_runs else 0.0

# Menu items: split on two-or-more spaces (both `[ Name ]` and bare `Name` styles).
menu_items = parse_menu_items(menu)

print(f"rows={len(rows)}")
print(f"border_style={border_style}")
print(f"menu_items({len(menu_items)})={menu_items}")
print(f"menu_raw={menu.rstrip()}")
print(f"footer={footer.rstrip()}")
print(f"focus_rows({len(focus_rows)}):")
for idx, bgs, t in focus_rows:
    print(f"  row {idx:3d}  {bgs}  {t[:90]}")
print(f"body_bold_run_fraction={bold_frac:.2f}  ({'SUSPICIOUS: body looks all-bold' if bold_frac > 0.6 else 'ok'})")