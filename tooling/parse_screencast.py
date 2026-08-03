#!/usr/bin/env python3
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Color-aware asciinema (.cast) analyzer for nomadnet parity work.
#
# Replays the output stream of a .cast through a proper terminal model so the
# resulting grid is clean, human-readable text WITH preserved, normalized color:
#   - CSI sequences (cursor move, erase, SGR) applied or stripped
#   - private modes (\x1b[?25l etc.) stripped
#   - G0/G1 charset designation (\x1b(B USASCII, \x1b(0 DEC special graphics)
#     tracked, with ACS graphics chars translated to unicode box drawing
#   - SGR color resolved to a normalized hex fg/bg + attribute string for every
#     cell, so color differences (menubar fill, list_focus bg, trust rows,
#     message header states) are diffable alongside text differences
#   - OSC/DSR/other escapes stripped
#
# It detects the active page from the left-pane border title and emits, for
# each page, the most stable (longest-lived) frame in two forms:
#   <out>/page_<NAME>.txt        plain text (layout/structure)
#   <out>/page_<NAME>.ansi.txt   inline color tags: styled runs are wrapped
#                                ⟦#fg,#bg,attrs⟧text⟦⟧  (default = bare text)
#
# With --diff, two casts are compared page-by-page on the annotated form, plus a
# per-region color summary (the distinct styles used in the menu row, footer
# row, and list body).
#
# Usage:
#   python3 parse_screencast.py --pages <out> <file.cast> [<file.cast>...]
#   python3 parse_screencast.py --diff <py.cast> <go.cast>
#   python3 parse_screencast.py --dump <out_dir> <file.cast>...

import difflib
import json
import os
import re
import sys

# DEC Special Graphics -> unicode (active when G0/G1 = '0')
ACS = {
    'j': '┘', 'k': '┐', 'l': '┌', 'm': '└', 'n': '┼',
    'q': '─', 't': '├', 'u': '┤', 'v': '┴', 'w': '┬', 'x': '│',
    'h': '▀', 'i': '▄', '0': '▇', 'A': '▁', 'B': '▂', 'C': '▃',
    'D': '▄', 'E': '▅', 'F': '▆', 'G': '▇', 'H': '█',
    '`': '◆', 'a': '▒', 'f': '°', 'g': '±', 'o': '⎺', 'p': '⎻',
    'r': '⎼', 's': '⎽', '~': '·', 'y': '≤', 'z': '≥', '|': '→',
    '}': '←', '{': 'π', '\\': '»', '_': ' ',
}

# Standard 16-color palette (xterm) for resolving SGR 30-37/40-47/90-97/100-107.
PAL16 = [
    "#000000", "#800000", "#008000", "#808000", "#000080", "#800080",
    "#008080", "#c0c0c0", "#808080", "#ff0000", "#00ff00", "#ffff00",
    "#0000ff", "#ff00ff", "#00ffff", "#ffffff",
]


def _cube(n):
    # 6x6x6 cube color (16..231)
    if n < 0:
        return 0
    levels = [0, 95, 135, 175, 215, 255]
    return levels[n]


def _gray(n):
    # grayscale (232..255): 8..238 step 10
    return 8 + (n * 10)


def color256_to_hex(n):
    if n < 16:
        return PAL16[n]
    if n < 232:
        n -= 16
        r = _cube(n // 36)
        g = _cube((n // 6) % 6)
        b = _cube(n % 6)
    else:
        v = _gray(n - 232)
        r = g = b = v
    return "#%02x%02x%02x" % (r, g, b)


class Style:
    __slots__ = ("fg", "bg", "bold", "dim", "italic", "underline",
                "reverse", "strike", "blink")

    def __init__(self):
        self.fg = None     # None = default, else "#rrggbb"
        self.bg = None
        self.bold = False
        self.dim = False
        self.italic = False
        self.underline = False
        self.reverse = False
        self.strike = False
        self.blink = False

    def copy(self):
        s = Style()
        for a in self.__slots__:
            setattr(s, a, getattr(self, a))
        return s

    def reset(self):
        self.fg = None
        self.bg = None
        self.bold = False
        self.dim = False
        self.italic = False
        self.underline = False
        self.reverse = False
        self.strike = False
        self.blink = False

    def key(self):
        # normalized key for run grouping & diffing. Apply reverse by swapping
        # fg/bg so equivalent reverse styles collapse together.
        fg, bg = self.fg, self.bg
        if self.reverse:
            fg, bg = bg, fg
        attrs = ""
        if self.bold:
            attrs += "B"
        if self.dim:
            attrs += "D"
        if self.italic:
            attrs += "I"
        if self.underline:
            attrs += "U"
        if self.strike:
            attrs += "S"
        if self.blink:
            attrs += "K"
        fg = fg if fg else "d"
        bg = bg if bg else "d"
        return "%s,%s,%s" % (fg, bg, attrs)

    def is_default(self):
        return (self.fg is None and self.bg is None and not self.bold
                and not self.dim and not self.italic and not self.underline
                and not self.reverse and not self.strike and not self.blink)


def apply_sgr(style, params):
    # params: list of int|None (raw SGR numbers)
    if not params or (len(params) == 1 and (params[0] is None or params[0] == 0)):
        style.reset()
        return
    i = 0
    while i < len(params):
        n = params[i]
        if n is None or n == 0:
            style.reset()
        elif n == 1:
            style.bold = True
        elif n == 2:
            style.dim = True
        elif n == 3:
            style.italic = True
        elif n == 4:
            style.underline = True
        elif n == 5:
            style.blink = True
        elif n == 7:
            style.reverse = True
        elif n == 9:
            style.strike = True
        elif n == 22:
            style.bold = False
            style.dim = False
        elif n == 23:
            style.italic = False
        elif n == 24:
            style.underline = False
        elif n == 25:
            style.blink = False
        elif n == 27:
            style.reverse = False
        elif n == 29:
            style.strike = False
        elif 30 <= n <= 37:
            style.fg = PAL16[n - 30]
        elif n == 38:
            # extended fg
            if i + 1 < len(params):
                mode = params[i + 1]
                if mode == 5 and i + 2 < len(params):
                    style.fg = color256_to_hex(params[i + 2])
                    i += 2
                elif mode == 2 and i + 4 < len(params):
                    style.fg = "#%02x%02x%02x" % (
                        params[i + 2] & 0xff, params[i + 3] & 0xff,
                        params[i + 4] & 0xff)
                    i += 4
        elif n == 39:
            style.fg = None
        elif 40 <= n <= 47:
            style.bg = PAL16[n - 40]
        elif n == 48:
            if i + 1 < len(params):
                mode = params[i + 1]
                if mode == 5 and i + 2 < len(params):
                    style.bg = color256_to_hex(params[i + 2])
                    i += 2
                elif mode == 2 and i + 4 < len(params):
                    style.bg = "#%02x%02x%02x" % (
                        params[i + 2] & 0xff, params[i + 3] & 0xff,
                        params[i + 4] & 0xff)
                    i += 4
        elif n == 49:
            style.bg = None
        elif 90 <= n <= 97:
            style.fg = PAL16[n - 90 + 8]
        elif 100 <= n <= 107:
            style.bg = PAL16[n - 100 + 8]
        i += 1


class Cell:
    __slots__ = ("ch", "style")

    def __init__(self):
        self.ch = ' '
        self.style = Style()


class Term:
    def __init__(self, cols, rows):
        self.cols = cols
        self.rows = rows
        self.grid = [[Cell() for _ in range(cols)] for _ in range(rows)]
        self.cx = 0
        self.cy = 0
        self.g0 = 'B'
        self.g1 = 'B'
        self.cur = 'g0'
        self.cur_style = Style()
        self.pending = ""  # carry incomplete escapes across chunk boundaries

    def _putc(self, ch):
        if self.cur == 'g0' and self.g0 == '0' and ch in ACS:
            ch = ACS[ch]
        elif self.cur == 'g1' and self.g1 == '0' and ch in ACS:
            ch = ACS[ch]
        if 0 <= self.cy < self.rows and 0 <= self.cx < self.cols:
            c = self.grid[self.cy][self.cx]
            c.ch = ch
            c.style = self.cur_style.copy()
        self.cx += 1
        if self.cx >= self.cols:
            self.cx = self.cols - 1  # clamp; no autowrap

    def _erase_row(self, y, x0=0, x1=None):
        x1 = self.cols if x1 is None else x1
        for x in range(x0, x1):
            self.grid[y][x].ch = ' '
            self.grid[y][x].style = self.cur_style.copy()

    def _clear(self):
        for y in range(self.rows):
            for x in range(self.cols):
                self.grid[y][x].ch = ' '
                self.grid[y][x].style = self.cur_style.copy()

    def feed(self, data):
        # asciinema splits output at arbitrary byte boundaries, so an escape
        # sequence can be split across two chunks. Carry any trailing partial
        # escape to the next call via self.pending.
        if self.pending:
            data = self.pending + data
            self.pending = ""
        i = 0
        n = len(data)
        while i < n:
            c = data[i]
            if c == '\x1b':
                consumed, incomplete = self._handle_esc(data, i)
                if incomplete:
                    self.pending = data[i:]
                    return
                i += consumed
                continue
            if c == '\r':
                self.cx = 0
                i += 1
                continue
            if c == '\n':
                self.cy = min(self.rows - 1, self.cy + 1)
                i += 1
                continue
            if c == '\b':
                self.cx = max(0, self.cx - 1)
                i += 1
                continue
            if c == '\t':
                self.cx = min(self.cols - 1, (self.cx // 8 + 1) * 8)
                i += 1
                continue
            if c >= ' ':
                self._putc(c)
            i += 1

    def _handle_esc(self, data, i):
        """Returns (consumed, incomplete). When incomplete, the caller saves
        data[i:] to self.pending for the next chunk."""
        if i + 1 >= len(data):
            return 0, True
        nxt = data[i + 1]
        if nxt == '[':
            j = i + 2
            while j < len(data) and not (0x40 <= ord(data[j]) <= 0x7e):
                j += 1
            if j >= len(data):
                return 0, True  # CSI not terminated yet
            self._apply_csi(data[i + 2:j + 1])  # include the final byte
            return (j + 1) - i, False
        if nxt == ']':
            j = i + 2
            while j < len(data):
                if data[j] == '\x07':
                    j += 1
                    break
                if data[j] == '\x1b' and j + 1 < len(data) and data[j + 1] == '\\':
                    j += 2
                    break
                j += 1
            if j >= len(data):
                return 0, True  # OSC not terminated yet
            return j - i, False
        if nxt in '()*+':
            if i + 2 >= len(data):
                return 0, True
            ch = data[i + 2]
            if nxt == '(':
                self.g0 = ch
            elif nxt == ')':
                self.g1 = ch
            return 3, False
        if nxt in 'OE':
            if i + 2 >= len(data):
                return 0, True
            return 3, False
        if nxt == 'N':
            if i + 2 >= len(data):
                return 0, True
            return 3, False
        if nxt == 'D':
            self.cy = min(self.rows - 1, self.cy + 1)
            return 2, False
        if nxt == 'M':
            self.cy = max(0, self.cy - 1)
            return 2, False
        if nxt == 'E':
            self.cy = min(self.rows - 1, self.cy + 1)
            self.cx = 0
            return 2, False
        if nxt in ('7', '8', '=', '>', 'c'):
            if nxt == 'c':
                self._clear()
                self.cx = self.cy = 0
            return 2, False
        return 2, False

    def _apply_csi(self, body):
        if not body:
            self._apply_csi_final('', [])
            return
        final = body[-1]
        params = body[:-1]
        priv = params.startswith('?')
        if priv:
            params = params[1:]
        nums = []
        for p in params.split(';'):
            if p == '':
                nums.append(None)
            else:
                try:
                    nums.append(int(p))
                except ValueError:
                    nums.append(None)
        if priv:
            return  # private modes (cursor show/hide, mouse, alt screen) ignored
        self._apply_csi_final(final, nums)

    def _apply_csi_final(self, final, nums):
        def arg(idx, default):
            if idx < len(nums) and nums[idx] is not None:
                return nums[idx]
            return default
        if final == 'H' or final == 'f':
            self.cy = max(0, min(self.rows - 1, arg(0, 1) - 1))
            self.cx = max(0, min(self.cols - 1, arg(1, 1) - 1))
        elif final == 'A':
            self.cy = max(0, self.cy - arg(0, 1))
        elif final == 'B':
            self.cy = min(self.rows - 1, self.cy + arg(0, 1))
        elif final == 'C':
            self.cx = min(self.cols - 1, self.cx + arg(0, 1))
        elif final == 'D':
            self.cx = max(0, self.cx - arg(0, 1))
        elif final == 'G':
            self.cx = max(0, min(self.cols - 1, arg(0, 1) - 1))
        elif final == 'd':
            self.cy = max(0, min(self.rows - 1, arg(0, 1) - 1))
        elif final == 'J':
            mode = arg(0, 0)
            if mode == 0:
                self._erase_row(self.cy, self.cx)
                for y in range(self.cy + 1, self.rows):
                    self._erase_row(y)
            elif mode == 1:
                self._erase_row(self.cy, 0, self.cx + 1)
                for y in range(0, self.cy):
                    self._erase_row(y)
            elif mode in (2, 3):
                self._clear()
        elif final == 'K':
            mode = arg(0, 0)
            if mode == 0:
                self._erase_row(self.cy, self.cx)
            elif mode == 1:
                self._erase_row(self.cy, 0, self.cx + 1)
            elif mode == 2:
                self._erase_row(self.cy)
        elif final == 'm':
            apply_sgr(self.cur_style, nums)
        elif final == 'S':
            for _ in range(arg(0, 1)):
                self.grid.pop(0)
                self.grid.append([Cell() for _ in range(self.cols)])
        elif final == 'T':
            for _ in range(arg(0, 1)):
                self.grid.pop()
                self.grid.insert(0, [Cell() for _ in range(self.cols)])
        elif final == 'L':
            n = arg(0, 1)
            for _ in range(n):
                if 0 <= self.cy < self.rows:
                    self.grid.insert(self.cy, [Cell() for _ in range(self.cols)])
                    self.grid.pop()
        elif final == 'M':
            n = arg(0, 1)
            for _ in range(n):
                if 0 <= self.cy < self.rows:
                    self.grid.pop(self.cy)
                    self.grid.append([Cell() for _ in range(self.cols)])
        elif final == 'P':
            n = arg(0, 1)
            if 0 <= self.cy < self.rows:
                row = self.grid[self.cy]
                del row[self.cx:min(self.cols, self.cx + n)]
                while len(row) < self.cols:
                    row.append(Cell())
        elif final == '@':
            n = arg(0, 1)
            if 0 <= self.cy < self.rows:
                row = self.grid[self.cy]
                for _ in range(n):
                    row.insert(self.cx, Cell())
                while len(row) > self.cols:
                    row.pop()
        elif final == 'X':
            n = arg(0, 1)
            self._erase_row(self.cy, self.cx, min(self.cols, self.cx + n))
        # other finals ignored

    # --- output forms ---
    def screen_text(self):
        lines = []
        for row in self.grid:
            s = ''.join(c.ch for c in row).rstrip()
            lines.append(s)
        while lines and not lines[-1].strip():
            lines.pop()
        return '\n'.join(lines)

    def screen_annotated(self):
        lines = []
        for row in self.grid:
            out = []
            prev_key = None
            in_tag = False
            for c in row:
                k = c.style.key()
                if k != prev_key:
                    if in_tag:
                        out.append('⟦⟧')
                        in_tag = False
                    if not c.style.is_default():
                        out.append('⟦%s⟧' % k)
                        in_tag = True
                    prev_key = k
                out.append(c.ch)
            if in_tag:
                out.append('⟦⟧')
            line = ''.join(out).rstrip()
            # trim trailing default spaces (but keep tag structure up to last non-space)
            lines.append(line)
        while lines and not lines[-1].strip():
            lines.pop()
        return '\n'.join(lines)


# --- page detection ---
BORDER_RE = re.compile(r'┌\s*─*([^─┌┐]*?)─*\s*┐')

TITLE_TO_PAGE = {
    "Conversations": "Conversations",
    "Announce Stream": "Network",
    "Announce Info": "Network",
    "Saved Nodes": "Network",
    "Known Nodes": "Network",
    "Channels": "Channels",
    "Topics": "Guide",
    "Interfaces": "Interfaces",
    "Log": "Log",
    "Configuration": "Config",
    "Config": "Config",
}
GUIDE_TOPICS = {
    "Introduction", "Concepts & Terminology", "Channels & RRC", "Interfaces",
    "Hosting a Node", "Configuration Options", "Keyboard Shortcuts", "Markup",
    "First Run", "Network Configuration", "Display Test", "Credits & Licenses",
}


def _left_titles(text):
    titles = []
    for ln in text.split('\n')[:6]:
        for m in BORDER_RE.finditer(ln):
            t = m.group(1).strip()
            if t:
                titles.append(t)
    return titles


def detect_page(text):
    titles = _left_titles(text)
    for t in titles:
        if t in TITLE_TO_PAGE:
            return TITLE_TO_PAGE[t]
        if t in GUIDE_TOPICS:
            return "Guide"
    # right-pane node hash title => Network detail
    for t in titles:
        if re.fullmatch(r'<[0-9a-f]{32}>', t):
            return "Network"
    return None


def parse_cast(filename, cols=232, rows=53):
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
    term = Term(cols, rows)
    page_frames = {}
    last_text = None
    for t, chunk in events:
        term.feed(chunk)
        text = term.screen_text()
        if text == last_text:
            continue
        last_text = text
        page = detect_page(text)
        if page is None:
            continue
        page_frames.setdefault(page, []).append((t, text))
    # attach the term? No -- we need annotated for chosen frames only.
    # Re-replay to capture annotated for selected frames is expensive; instead
    # keep term state by re-running and snapshotting annotated at the chosen
    # timestamps. We return plain frames + a re-play capability.
    return {"filename": filename, "page_frames": page_frames}


def replay_annotated(filename, want_pages, cols=232, rows=53):
    """Replay cast, yielding (page, text, annotated) for the longest-lived
    frame of each page in want_pages (set). Returns dict page -> annotated str.
    """
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
    # first pass: find, per page, the timestamp range of the longest-lived frame
    term = Term(cols, rows)
    last_text = None
    # list of (page, start_t) while active; we track the chosen frame per page
    # by gap analysis in a single pass.
    page_runs = {}  # page -> list of (start_t, text)
    for t, chunk in events:
        term.feed(chunk)
        text = term.screen_text()
        if text == last_text:
            continue
        last_text = text
        page = detect_page(text)
        if page is None:
            continue
        page_runs.setdefault(page, []).append((t, text))
    # choose per page the frame with the largest gap to the next frame
    chosen = {}
    for page, runs in page_runs.items():
        if page not in want_pages:
            continue
        best_i, best_gap = 0, 0.0
        for i in range(len(runs) - 1):
            gap = runs[i + 1][0] - runs[i][0]
            if gap > best_gap:
                best_gap = gap
                best_i = i
        # if only one frame, use it; if last frame is the end, also keep best
        chosen[page] = runs[best_i][1]
    # second pass: replay to capture annotated text at the chosen frame texts.
    # Match by text equality (the chosen plain text).
    wanted_texts = {page: txt for page, txt in chosen.items()}
    found = {}
    term = Term(cols, rows)
    last_text = None
    for t, chunk in events:
        term.feed(chunk)
        text = term.screen_text()
        if text == last_text:
            continue
        last_text = text
        page = detect_page(text)
        if page is None or page not in wanted_texts:
            continue
        if page in found:
            continue
        if text == wanted_texts[page]:
            found[page] = term.screen_annotated()
    # fallback: if a page's chosen frame text never re-matched (shouldn't
    # happen), capture the first frame of that page.
    term = Term(cols, rows)
    last_text = None
    for t, chunk in events:
        term.feed(chunk)
        text = term.screen_text()
        if text == last_text:
            continue
        last_text = text
        page = detect_page(text)
        if page is None or page not in wanted_texts:
            continue
        if page in found:
            continue
        found[page] = term.screen_annotated()
    return found, chosen


def write_pages(filename, out_dir):
    os.makedirs(out_dir, exist_ok=True)
    res = parse_cast(filename)
    want = set(res["page_frames"].keys())
    annotated, plain = replay_annotated(filename, want)
    base = os.path.basename(filename).replace('.cast', '')
    idx_lines = []
    for page in sorted(want):
        ptxt = plain.get(page, "")
        atxt = annotated.get(page, "")
        ppath = os.path.join(out_dir, f"{base}_page_{page}.txt")
        apath = os.path.join(out_dir, f"{base}_page_{page}.ansi.txt")
        with open(ppath, 'w', encoding='utf-8') as f:
            f.write(ptxt)
        with open(apath, 'w', encoding='utf-8') as f:
            f.write(atxt)
        idx_lines.append(f"{page}: {len(res['page_frames'][page])} frames -> {ppath} + {apath}")
        print(f"{page}: {len(res['page_frames'][page])} frames")
    with open(os.path.join(out_dir, f"{base}_index.txt"), 'w') as f:
        f.write('\n'.join(idx_lines))
    return want


def diff_pages(f1, f2, out_dir=None):
    res1 = parse_cast(f1)
    res2 = parse_cast(f2)
    pages = sorted(set(res1["page_frames"]) | set(res2["page_frames"]))
    ann1, _ = replay_annotated(f1, set(pages))
    ann2, _ = replay_annotated(f2, set(pages))
    print("\n=======================================================")
    print("DIFF (color-annotated): %s vs %s" % (f1, f2))
    print("=======================================================")
    for page in pages:
        n1 = len(res1["page_frames"].get(page, []))
        n2 = len(res2["page_frames"].get(page, []))
        print("\n--- Page [%s]: Python frames=%d, Go frames=%d ---" % (page, n1, n2))
        a1 = ann1.get(page)
        a2 = ann2.get(page)
        if a1 is None:
            print("  (not visited in Python)")
            continue
        if a2 is None:
            print("  (not visited in Go)")
            continue
        d = list(difflib.unified_diff(
            a1.splitlines(), a2.splitlines(),
            fromfile="Python", tofile="Go", lineterm=""))
        if not d:
            print("  Exact color+text match!")
        else:
            for line in d[:60]:
                print("  " + line)
            if len(d) > 60:
                print("  ... (%d more diff lines)" % (len(d) - 60))


def main():
    if len(sys.argv) < 2:
        print("Usage:")
        print("  parse_screencast.py --pages <out_dir> <f.cast> [...]")
        print("  parse_screencast.py --diff <py.cast> <go.cast>")
        print("  parse_screencast.py --dump <out_dir> <f.cast> [...]")
        sys.exit(1)
    args = sys.argv[1:]
    mode = None
    rest = []
    i = 0
    while i < len(args):
        a = args[i]
        if a == '--pages':
            mode = 'pages'
            i += 1
            out = args[i]
            i += 1
            for f in args[i:]:
                print("\n=== %s ===" % f)
                write_pages(f, out)
            return
        elif a == '--diff':
            mode = 'diff'
            i += 1
            f1, f2 = args[i], args[i + 1]
            diff_pages(f1, f2)
            return
        elif a == '--dump':
            i += 1
            out = args[i]
            i += 1
            for f in args[i:]:
                _dump(f, out)
            return
        else:
            rest.append(a)
            i += 1
    if rest:
        # default: analysis report per file
        for f in rest:
            print("\n=== %s ===" % f)
            res = parse_cast(f)
            for page, frames in sorted(res["page_frames"].items()):
                print("  %s: %d frames" % (page, len(frames)))


def _dump(filename, out_dir):
    """Dump every distinct frame as plain text (legacy behavior)."""
    os.makedirs(out_dir, exist_ok=True)
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
    term = Term(232, 53)
    base = os.path.basename(filename).replace('.', '_')
    od = os.path.join(out_dir, base)
    os.makedirs(od, exist_ok=True)
    last = None
    idx = 0
    for t, chunk in events:
        term.feed(chunk)
        text = term.screen_text()
        if text == last:
            continue
        last = text
        with open(os.path.join(od, f"frame_{idx:05d}_{t:06.2f}s.txt"), 'w',
                  encoding='utf-8') as f:
            f.write(text)
        idx += 1
    print("Dumped %d frames to %s" % (idx, od))


if __name__ == '__main__':
    main()
