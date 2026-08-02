#!/usr/bin/env python3
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Reusable screencast analysis tool for parsing asciinema (.cast) files,
# extracting terminal screen snapshots, analyzing network logs, and
# identifying visual/behavioral discrepancies between Python NomadNet and Go NomadNet.

import json
import os
import re
import sys

ANSI_ESCAPE = re.compile(r"\x1b\[[0-9;]*[a-zA-Z]|\x1b\)[0-9A-Z]|\x1b\][0-9;]*[^\a\x1b]*[\a\x1b]")

def clean_ansi(text):
    return ANSI_ESCAPE.sub("", text)

def parse_cast(filename, cols=232, rows=53):
    if not os.path.exists(filename):
        print(f"File not found: {filename}")
        return None

    events = []
    inputs = []
    with open(filename, "r", encoding="utf-8", errors="replace") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                data = json.loads(line)
                if isinstance(data, list) and len(data) >= 3:
                    if data[1] == "o":
                        events.append((data[0], data[2]))
                    elif data[1] == "i":
                        inputs.append((data[0], repr(data[2])))
            except Exception:
                pass

    print(f"Loaded {len(events)} output chunks and {len(inputs)} input events from {filename}")

    grid = [[" " for _ in range(cols)] for _ in range(rows)]
    cx, cy = 0, 0
    snapshots = []
    log_highlights = []

    pattern = re.compile(r"\x1b\[([0-9;]*)([a-zA-Z])|\x1b\)[0-9A-Z]")

    for t, chunk in events:
        # Check text for log messages
        clean_chunk = clean_ansi(chunk)
        for line in clean_chunk.splitlines():
            line_s = line.strip()
            if any(kw in line_s for kw in ["[Error]", "[Warning]", "Failed", "closed", "reconnect", "Learned path"]):
                if not log_highlights or log_highlights[-1][1] != line_s:
                    log_highlights.append((t, line_s))

        pos = 0
        for m in pattern.finditer(chunk):
            text = chunk[pos:m.start()]
            for ch in text:
                if ch == "\r":
                    cx = 0
                elif ch == "\n":
                    cy = min(rows - 1, cy + 1)
                elif ch >= " ":
                    if 0 <= cy < rows and 0 <= cx < cols:
                        grid[cy][cx] = ch
                    cx += 1
            pos = m.end()

            params = m.group(1).split(";") if m.group(1) else []
            cmd = m.group(2)
            if cmd == "H" or cmd == "f":
                r = int(params[0]) - 1 if params and params[0] else 0
                c = int(params[1]) - 1 if len(params) > 1 and params[1] else 0
                cy, cx = max(0, min(rows - 1, r)), max(0, min(cols - 1, c))
            elif cmd == "K":
                mode = int(params[0]) if params and params[0] else 0
                if mode == 0 and 0 <= cy < rows:
                    for x in range(cx, cols):
                        grid[cy][x] = " "
            elif cmd == "J":
                mode = int(params[0]) if params and params[0] else 0
                if mode == 2:
                    grid = [[" " for _ in range(cols)] for _ in range(rows)]
                    cx, cy = 0, 0

        text = chunk[pos:]
        for ch in text:
            if ch == "\r":
                cx = 0
            elif ch == "\n":
                cy = min(rows - 1, cy + 1)
            elif ch >= " ":
                if 0 <= cy < rows and 0 <= cx < cols:
                    grid[cy][cx] = ch
                cx += 1

        screen_lines = ["".join(row).rstrip() for row in grid if "".join(row).strip()]
        screen_text = "\n".join(screen_lines)
        if any(kw in screen_text for kw in ["Conversations", "Network", "Remote Node", "Saved Nodes", "Announce Stream"]):
            if not snapshots or snapshots[-1][1] != screen_text:
                snapshots.append((t, screen_text))

    return {
        "filename": filename,
        "inputs": inputs,
        "log_highlights": log_highlights,
        "snapshots": snapshots,
    }

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 tooling/parse_screencast.py <cast_file1> [cast_file2...]")
        sys.exit(1)

    for filename in sys.argv[1:]:
        res = parse_cast(filename)
        if not res:
            continue
        print(f"\n=======================================================")
        print(f"ANALYSIS REPORT: {res['filename']}")
        print(f"=======================================================")

        if res["inputs"]:
            print(f"\n--- KEY INPUT SEQUENCE ({len(res['inputs'])} events) ---")
            for t, inp in res["inputs"][:30]:
                print(f"  [{t:6.2f}s] {inp}")
            if len(res["inputs"]) > 30:
                print(f"  ... ({len(res['inputs']) - 30} more input events)")

        if res["log_highlights"]:
            print(f"\n--- LOG HIGHLIGHTS & NETWORK ERRORS ({len(res['log_highlights'])} items) ---")
            for t, log in res["log_highlights"][:30]:
                print(f"  [{t:6.2f}s] {log[:120]}")

        if res["snapshots"]:
            print(f"\n--- UNIQUE SCREEN SNAPSHOTS ({len(res['snapshots'])} frames) ---")
            print(f"Sample of last rendered screen snapshot:")
            last_snap = res["snapshots"][-1][1]
            for l in last_snap.splitlines()[:20]:
                print(f"  | {l}")

if __name__ == "__main__":
    main()
