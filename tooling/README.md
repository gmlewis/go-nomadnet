# Parity & screencast-comparison tooling

This directory holds the tools used to compare the Go port of nomadnet against
the source-of-truth Python original. They work on asciinema `.cast` recordings
(JSONL: one `[time, "o"|"i", data]` event per line).

The older subdirectories have their own READMEs:

- `tui-parity/` — live (`asciinema`/`tmux`) capture of the running Python and Go
  binaries plus `summary.py`/`ansiview.py` decoders. See `tui-parity/README.md`.
- `micron-parity/` — golden-value extractors for the micron *parser* (no urwid
  needed). See `micron-parity/README.md`.

The three scripts below (`parse_screencast.py`, `substates.py`, `guidetopics.py`)
are the **color-aware cast comparison** suite. They were written because the
`tui-parity` summary strips color and the original `parse_screencast.py` did not
decode ANSI escapes correctly. They compare **color, underline, italic, bold,
reverse, strike, and blink** — not just text — because rendering behavior
depends on all of these.

## ⚠️ Colormode caveat (read before trusting any color diff)

**The colormode of each `.cast` must match before palette *color values* can be
compared.** asciinema records exactly the bytes the program emits:

- Python `nomadnet` (urwid) emits the colormode selected by its `colormode`
  config / the terminal's advertised color capability. A recording made in
  **256-color mode** emits `\x1b[38;5;N;48;5;M` palette-index SGR; a **truecolor**
  recording emits `\x1b[38;2;r;g;b;48;2;r;g;b`.
- The Go port (tview/tcell) currently always emits **truecolor**.

So a Python cast recorded in 256-color and a Go cast recorded in truecolor will
differ in *every* color cell even when both correctly implement the **same**
palette — urwid approximates the truecolor RGB to the nearest 256-palette index,
while tcell emits the exact RGB. **Such color differences are colormode artifacts,
not port bugs.**

How to tell which colormode a cast used (look at the raw menubar SGR):

```bash
python3 - <<'PY'
import json
fn="python_session.cast"
with open(fn, errors="replace") as f:
    for line in f:
        d=json.loads(line.strip())
        if isinstance(d,list) and d[1]=='o' and "Conversations ]" in d[2] and "Network ]" in d[2]:
            print(repr(d[2][:200])); break
PY
# \x1b[...38;5;16;48;5;145...   -> 256-color   (palette index)
# \x1b[...38;2;17;17;17;48;2;187;187;187... -> truecolor (RGB)
```

To make a valid color comparison, **re-record the Python session in truecolor**
(set `colormode = 24bit` in `~/.reticulum/...` / the nomadnet config and run in a
truecolor-capable terminal, e.g. `COLORTERM=truecolor` set). Then the Go
truecolor RGB values can be diffed against the Python truecolor RGB values
directly.

**What IS comparable across colormodes** (and is what these tools reliably find):
**structural** differences and **text-attribute** differences that are not
color-index approximations — e.g. a border that exists in one and not the other,
an underline attribute that leaks, a menu item that is bold/highlighted in one
and uniform in the other, a footer that is present/absent, a forced background
where the other uses terminal default.

## `parse_screencast.py` — color-aware terminal emulator + page detector

A minimal VT100/ANSI terminal emulator that feeds a `.cast`'s output chunks and
reconstructs the final screen grid of `Cell`s, each carrying a character **and a
`Style`** (fg hex, bg hex, bold/dim/italic/underline/reverse/strike/blink). It
handles:

- CSI SGR: truecolor `38;2;/48;2;`, 256-palette `38;5;/48;5;`, 16-color
  `30-37`/`40-47`, and resets (`39`/`49`/`0`).
- Private modes (`\x1b[?25l` hide cursor), cursor positioning (`\x1b[H`,
  `\x1b[ROW;COLH`), erase (`\x1b[K`, `\x1b[J`).
- G0 charset designation (`\x1b(B` US-ASCII, `\x1b(0` DEC special graphics) with
  ACS box-drawing translation (`q`→`─`, `x`→`│`, etc.).
- **Cross-chunk escape splicing**: `.cast` output is split at arbitrary byte
  boundaries, so an escape sequence can span two chunks. A `pending` buffer
  carries the incomplete tail into the next `feed()`; `_handle_esc` returns
  `incomplete=True` to signal this. Without it, parser artifacts like
  `Peer2HInfo` or `│` mid-row appear and read as false Go bugs.
- Color normalization to `#rrggbb`; reverse swaps fg/bg in the style key.

Output formats:

- `screen_text()` — plain text (no style).
- `screen_annotated()` — the same grid with inline style tags:
  `⟦#fg,#bg,ATTRS⟧text⟦⟧` where `ATTRS` is a subset of `B`/`D`/`I`/`U`/`S`/`K`/`R`
  (`d` in a field means terminal default). This is what you diff to see color and
  effects.

Page detection uses the **left-pane border title** (every menu bar contains
every page name, so substring detection is wrong) plus a right-pane `<hash>` regex
for the Network/Browser pages.

### CLI

```bash
# Per-page: for each .cast, save the longest-lived settled frame per detected
# page as both plain text and annotated (style-tagged) text.
python3 tooling/parse_screencast.py --pages <out_dir> <a.cast> [b.cast ...]

# Side-by-side diff of matching pages across a Python and a Go cast
# (prints annotated lines that differ).
python3 tooling/parse_screencast.py --diff <py.cast> <go.cast>

# Dump EVERY distinct frame (plain text) — large; for deep debugging.
python3 tooling/parse_screencast.py --dump <out_dir> <a.cast> [b.cast ...]

# No mode flag: print a one-line per-page frame-count summary per cast.
python3 tooling/parse_screencast.py <a.cast>
```

`Term` is constructed at 232×53 (the recorded terminal size); change the
constants in `main`/the helper scripts if your cast used a different size.

## `substates.py` — per sub-state (page × left-pane-title) frames + chrome

The whole-screen "longest frame per page" frame is often a poor unit of
comparison because urwid/tview render **diff-style**: only changed cells are
re-emitted, so stale cells from earlier pages (boot log, splash) persist in
undrawn areas and read as false bugs. `substates.py` narrows the comparison to
stable sub-states.

For each `(page, left_pane_title)` sub-state it saves:

- `<base>__<Page>__<LeftPaneTitle>.txt` / `.ansi.txt` — the **fullest** left-pane
  content frame.
- `<base>__<Page>__<LeftPaneTitle>.last.txt` / `.longest.txt` / `.longest.ansi.txt`
  — the **settled** frame (biggest time gap to the next change; preferred for
  comparison because it has stopped mid-render churn).
- `<base>_chrome.txt` — every distinct (menu, footer) pair the session ever
  showed, annotated, in time order. Chrome (menu bar + shortcut bar) is redrawn
  on every frame, so it is the most reliable cross-session comparison surface.

Usage:

```bash
python3 tooling/substates.py <file.cast> <out_dir>
```

`pick_best` chooses the fullest left-pane content; `_pick_longest` chooses the
frame with the biggest time gap (settled). **Prefer `.longest.ansi.txt`** for
body comparison and `_chrome.txt` for menu/footer comparison.

## `guidetopics.py` — per-guide-topic reader frames

The Guide page has 12 embedded micron topics; comparing "the Guide page" as one
frame mixes the topic list with whatever reader content happened to be showing.
`guidetopics.py` detects the topic being viewed (Go: the right-pane border title;
Python: the reader's first content heading matching a known topic name) and
saves, per topic, the longest-lived clean reader frame:

- `<base>__guide__<Topic>.txt` / `.ansi.txt`
- `<base>_guide_topics.txt` — index of captured topics + frame counts.

Usage:

```bash
python3 tooling/guidetopics.py <file.cast> <out_dir>
```

**Known limitation:** urwid diff-rendering means a freshly-switched Python reader
frame can show stale body rows from the *previous* topic (only changed rows are
redrawn). If a Python topic frame's body looks like the wrong topic, it is a
frame-selection artifact, not a port bug — cross-check against the Go frame
(which renders the full topic) and against the Python source `.mu` content.

## Known tooling limitations

- **UTF-8 multibyte glyphs are not decoded.** `Term` treats each input byte as
  one cell, so Nerd Font icons and other multibyte characters appear as mojibake
  (`â`, `ï`, `ó°`) in both Python and Go output. This is a *tool* limitation,
  symmetrical across both casts, so it does not by itself indicate a port bug —
  but it prevents reliable comparison of glyph *content*. Fixing it (decode the
  cast as UTF-8 and advance the cursor by display width) is a pending task.
- **No OSC/DCS/SO-SI handling beyond cursor hide.** Hyperlink OSC 8
  (`\x1b]8;;url\x1b\\`) markers emitted by tview are skipped but the link target
  is not captured.
- **No italic-under-256-color round-trip verification.** Attribute flags
  (bold/italic/underline/…) are tracked from SGR and are reliable, but 256-color
  approximations of *dim* (urwid `gNN`) can be confused with truecolor grays.
  Always confirm a suspected color bug against the Python source palette (see
  the colormode caveat) before treating it as real.