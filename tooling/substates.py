#!/usr/bin/env python3
"""Extract per sub-state frames + chrome (menu/footer) from a .cast.

A sub-state is (page, left_pane_title). For each sub-state we save the
longest-lived frame (plain + color-annotated). We also dump every distinct
(menu_row, footer_row) chrome pair observed, annotated, since the menu/footer
appear on every frame and are the most reliable cross-session comparable
chrome.

Usage: python3 substates.py <file.cast> <out_dir>
"""
import importlib.util
import json
import os
import re
import sys

_HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location(
    'pcs', os.path.join(_HERE, 'parse_screencast.py'))
pcs = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(pcs)

BORDER_RE = re.compile(r'┌\s*─*([^─┌┐]*?)─*\s*┐')


def first_left_title(text):
    for ln in text.split('\n')[:6]:
        for m in BORDER_RE.finditer(ln):
            t = m.group(1).strip()
            if t:
                return t
    return None


def parse(filename, cols=232, rows=53):
    events = []
    with open(filename, 'r', encoding='utf-8', errors='replace') as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                d = json.loads(line)
                if isinstance(d, list) and len(d) >= 3 and d[1] == 'o':
                    events.append((d[0], d[2]))
            except Exception:
                pass
    term = pcs.Term(cols, rows)
    sub_runs = {}   # (page,left_title) -> list[(t, text, annotated)]
    chrome = {}     # (menu_annotated, footer_annotated) -> first seen t
    last_text = None
    for t, chunk in events:
        term.feed(chunk)
        text = term.screen_text()
        if text == last_text:
            continue
        last_text = text
        page = pcs.detect_page(text)
        if page is None:
            continue
        lt = first_left_title(text)
        key = (page, lt)
        # chrome: menu = row 0, footer = last non-empty row
        ann = term.screen_annotated()
        lines = ann.split('\n')
        menu = lines[0] if lines else ""
        footer = ""
        for ln in reversed(lines):
            if ln.strip():
                footer = ln
                break
        ck = (menu, footer)
        if ck not in chrome:
            chrome[ck] = (t, page)
        sub_runs.setdefault(key, []).append((t, text, ann))
    return sub_runs, chrome


def _left_content(text):
    """Count non-space, non-border chars in the left pane (cols 1..40)."""
    n = 0
    for ln in text.split('\n')[:48]:
        seg = ln[1:40]
        for ch in seg:
            if ch not in (' ', '│', '─', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼'):
                n += 1
    return n


def _pick_longest(runs):
    best_i, best_gap = 0, 0.0
    for i in range(len(runs) - 1):
        gap = runs[i + 1][0] - runs[i][0]
        if gap > best_gap:
            best_gap = gap
            best_i = i
    return runs[best_i]


def pick_best(runs):
    """Pick the frame with the fullest left-pane content (avoids picking the
    intro splash or an empty/transient frame). Falls back to longest-lived."""
    best_i, best_n = 0, -1
    for i, (t, text, ann) in enumerate(runs):
        n = _left_content(text)
        if n > best_n:
            best_n = n
            best_i = i
    return runs[best_i]


def main():
    fn, out = sys.argv[1], sys.argv[2]
    os.makedirs(out, exist_ok=True)
    sub_runs, chrome = parse(fn)
    base = os.path.basename(fn).replace('.cast', '')
    idx = []
    for (page, lt), runs in sorted(sub_runs.items()):
        t, text, ann = pick_best(runs)
        lt_text, lt_ann = runs[-1][1], runs[-1][2]
        lo_t, lo_text, lo_ann = _pick_longest(runs)
        safe = re.sub(r'[^A-Za-z0-9]+', '_', lt or "none")
        stem = f"{base}__{page}__{safe}"
        with open(os.path.join(out, stem + ".txt"), 'w') as f:
            f.write(text)
        with open(os.path.join(out, stem + ".ansi.txt"), 'w') as f:
            f.write(ann)
        with open(os.path.join(out, stem + ".last.txt"), 'w') as f:
            f.write(lt_text)
        with open(os.path.join(out, stem + ".longest.txt"), 'w') as f:
            f.write(lo_text)
        with open(os.path.join(out, stem + ".longest.ansi.txt"), 'w') as f:
            f.write(lo_ann)
        idx.append(f"{page} | {lt} | {len(runs)} frames -> {stem}")
    with open(os.path.join(out, f"{base}_substates.txt"), 'w') as f:
        f.write('\n'.join(idx))
    # chrome
    with open(os.path.join(out, f"{base}_chrome.txt"), 'w') as f:
        for (menu, footer), (t, page) in sorted(chrome.items(), key=lambda x: x[1][0]):
            f.write(f"=== t={t:.2f} page={page} ===\nMENU: {menu}\nFOOTER: {footer}\n\n")
    print(f"{base}: {len(sub_runs)} sub-states, {len(chrome)} chrome states")


if __name__ == '__main__':
    main()