#!/usr/bin/env python3
"""Extract per-guide-topic reader frames from a .cast.

For the Guide page, detect the topic being viewed:
  - Go: the right-pane border title (e.g. "Concepts & Terminology")
  - Python: the reader's first heading line (the styled topic-title row),
    since Python's reader pane has no border title.
Then save, per topic, the longest-lived clean reader frame (plain + annotated).

Usage: python3 guidetopics.py <file.cast> <out_dir>
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

TOPIC_NAMES = [
    "Introduction", "Concepts & Terminology", "Channels & RRC", "Interfaces",
    "Hosting a Node", "Configuration Options", "Keyboard Shortcuts", "Markup",
    "First Run", "Network Configuration", "Display Test", "Credits & Licenses",
]


def right_pane_title(text):
    """The right pane's border title, if any (Go puts the topic name there)."""
    lines = text.split('\n')
    if len(lines) < 3:
        return None
    # right pane top border is on line 2 (index 1); find the second ┌...┐
    line = lines[1]
    matches = list(re.finditer(r'┌\s*─*([^─┌┐]*?)─*\s*┐', line))
    if len(matches) >= 2:
        t = matches[1].group(1).strip()
        return t or None
    return None


def reader_first_heading(text):
    """Python: the reader's first content line that is a topic title (styled).
    Heuristic: a line in the right pane matching a known topic name."""
    for ln in text.split('\n')[2:8]:
        for tn in TOPIC_NAMES:
            if tn in ln:
                return tn
        # also accept the "and" variant Go uses inside content
        if "Concepts and Terminology" in ln:
            return "Concepts & Terminology"
    return None


def detect_guide_topic(text):
    t = right_pane_title(text)
    if t in TOPIC_NAMES:
        return t
    h = reader_first_heading(text)
    return h


def main():
    fn, out = sys.argv[1], sys.argv[2]
    os.makedirs(out, exist_ok=True)
    events = []
    with open(fn, 'r', encoding='utf-8', errors='replace') as f:
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
    term = pcs.Term(232, 53)
    topic_runs = {}
    last_text = None
    for t, chunk in events:
        term.feed(chunk)
        text = term.screen_text()
        if text == last_text:
            continue
        last_text = text
        page = pcs.detect_page(text)
        if page != "Guide":
            continue
        topic = detect_guide_topic(text)
        if not topic:
            continue
        topic_runs.setdefault(topic, []).append((t, text, term.screen_annotated()))
    base = os.path.basename(fn).replace('.cast', '')
    idx = []
    for topic, runs in sorted(topic_runs.items()):
        # longest-lived
        best_i, best_gap = 0, 0.0
        for i in range(len(runs) - 1):
            gap = runs[i + 1][0] - runs[i][0]
            if gap > best_gap:
                best_gap = gap
                best_i = i
        if runs:
            t, text, ann = runs[max(best_i, 0)]
        else:
            continue
        safe = re.sub(r'[^A-Za-z0-9]+', '_', topic)
        stem = f"{base}__guide__{safe}"
        with open(os.path.join(out, stem + ".txt"), 'w') as f:
            f.write(text)
        with open(os.path.join(out, stem + ".ansi.txt"), 'w') as f:
            f.write(ann)
        idx.append(f"{topic}: {len(runs)} frames -> {stem}")
    with open(os.path.join(out, f"{base}_guide_topics.txt"), 'w') as f:
        f.write('\n'.join(idx))
    print(f"{base}: {len(topic_runs)} guide topics")


if __name__ == '__main__':
    main()