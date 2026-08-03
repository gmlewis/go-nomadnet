#!/usr/bin/env python3
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Reusable screencast analysis tool for parsing asciinema (.cast) files,
# capturing complete 2D terminal screen buffer snapshots across ALL pages
# (Guide, Conversations, Network, Channels, Log, Interfaces, Config),
# exporting frame dumps, and performing full diff analysis between sessions.

import difflib
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

    pattern = re.compile(r"\x1b\[([0-9;]*)([a-zA-Z])|\x1b\)[0-9A-Z]|\x1b\][0-9;]*[^\a\x1b]*[\a\x1b]")

    for t, chunk in events:
        # Check text for log messages / errors
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

            seq = m.group(0)
            if seq.startswith("\x1b["):
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

        screen_lines = ["".join(row).rstrip() for row in grid]
        while screen_lines and not screen_lines[-1].strip():
            screen_lines.pop()
        screen_text = "\n".join(screen_lines)

        # Capture ALL unique screen states (no skipping!)
        if screen_text and (not snapshots or snapshots[-1][1] != screen_text):
            snapshots.append((t, screen_text))

    return {
        "filename": filename,
        "inputs": inputs,
        "log_highlights": log_highlights,
        "snapshots": snapshots,
    }

def dump_snapshots(res, out_dir):
    os.makedirs(out_dir, exist_ok=True)
    snaps = res["snapshots"]
    print(f"Dumping {len(snaps)} screen frame snapshots to {out_dir}...")
    for idx, (t, screen) in enumerate(snaps):
        path = os.path.join(out_dir, f"frame_{idx:04d}_{t:06.2f}s.txt")
        with open(path, "w", encoding="utf-8") as f:
            f.write(screen)

def diff_casts(res1, res2):
    print(f"\n=======================================================")
    print(f"DIFF ANALYSIS: {res1['filename']} (Original) vs {res2['filename']} (Go Port)")
    print(f"=======================================================")

    pages = ["Guide", "Conversations", "Network", "Channels", "Log", "Interfaces", "Config"]
    for page in pages:
        frames1 = [s for t, s in res1["snapshots"] if page in s]
        frames2 = [s for t, s in res2["snapshots"] if page in s]
        print(f"\n--- Page [{page}]: Python frames={len(frames1)}, Go frames={len(frames2)} ---")
        if frames1 and frames2:
            snap1 = frames1[-1].splitlines()
            snap2 = frames2[-1].splitlines()
            diff = list(difflib.unified_diff(snap1, snap2, fromfile="Python", tofile="Go", lineterm=""))
            if diff:
                print(f"Differences in [{page}] last frame:")
                for d in diff[:25]:
                    print("  ", d)
                if len(diff) > 25:
                    print(f"   ... ({len(diff) - 25} more diff lines)")
            else:
                print(f"  Exact visual match on [{page}]!")

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 tooling/parse_screencast.py [--dump <out_dir>] [--diff <file1> <file2>] <file1> [file2...]")
        sys.exit(1)

    dump_dir = None
    diff_mode = False
    files = []
    args = sys.argv[1:]
    idx = 0
    while idx < len(args):
        arg = args[idx]
        if arg == "--dump" and idx + 1 < len(args):
            dump_dir = args[idx + 1]
            idx += 2
        elif arg == "--diff":
            diff_mode = True
            idx += 1
        else:
            files.append(arg)
            idx += 1

    parsed = []
    for filename in files:
        res = parse_cast(filename)
        if res:
            parsed.append(res)
            print(f"\n=======================================================")
            print(f"ANALYSIS REPORT: {res['filename']}")
            print(f"=======================================================")

            if res["inputs"]:
                print(f"\n--- INPUT EVENT STREAM ({len(res['inputs'])} events) ---")
                for t, inp in res["inputs"][:15]:
                    print(f"  [{t:6.2f}s] {inp}")
                if len(res["inputs"]) > 15:
                    print(f"  ... ({len(res['inputs']) - 15} more input events)")

            if res["log_highlights"]:
                print(f"\n--- LOG HIGHLIGHTS & NETWORK ERRORS ({len(res['log_highlights'])} items) ---")
                for t, log in res["log_highlights"][:15]:
                    print(f"  [{t:6.2f}s] {log[:120]}")

            snaps = res["snapshots"]
            print(f"\n--- ALL CAPTURED UNIQUE SCREEN FRAMES ({len(snaps)} frames) ---")
            for i, (t, screen) in enumerate(snaps[:10]):
                first_line = screen.splitlines()[0] if screen.splitlines() else ""
                second_line = screen.splitlines()[1] if len(screen.splitlines()) > 1 else ""
                print(f"  Frame {i:03d} [{t:6.2f}s]: Header: {first_line[:80]} | Body: {second_line[:60]}")
            if len(snaps) > 10:
                print(f"  ... ({len(snaps) - 10} more frames)")

            if dump_dir:
                out = os.path.join(dump_dir, os.path.basename(filename).replace(".", "_"))
                dump_snapshots(res, out)

    if diff_mode and len(parsed) >= 2:
        diff_casts(parsed[0], parsed[1])

if __name__ == "__main__":
    main()
