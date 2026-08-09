# Parity & screencast-comparison tooling

This directory holds the tools used to compare the Go port of nomadnet against
the source-of-truth Python original. They work on asciinema `.cast` recordings
(JSONL: one `[time, "o"|"i", data]` event per line).

The older subdirectories have their own READMEs:

- `tui-parity/` — live (`asciinema`/`tmux`) capture of the running Python and Go
  binaries plus `summary.py`/`ansiview.py` decoders. See `tui-parity/README.md`.
- `micron-parity/` — golden-value extractors for the micron *parser* (no urwid
  needed). See `micron-parity/README.md`.

The scripts below are the **color-aware cast comparison** suite. The three
decoders (`parse_screencast.py`, `substates.py`, `guidetopics.py`) were written
because the `tui-parity` summary strips color and the original
`parse_screencast.py` did not decode ANSI escapes correctly. They compare
**color, underline, italic, bold, reverse, strike, and blink** — not just text
— because rendering behavior depends on all of these. `record-cast.sh` is the
recorder that produces the `.cast` files those decoders consume, forcing 24-bit
truecolor for both targets so the casts are directly comparable.

## ⚠️ Colormode caveat (read before trusting any color diff)

**The colormode of each `.cast` must match before palette *color values* can be
compared.** asciinema records exactly the bytes the program emits:

- Python `nomadnet` (urwid) emits the colormode selected by its `colormode`
  config. urwid's `set_terminal_properties` is **authoritative** — it does *not*
  consult the terminal's advertised color capability — so a config with
  `colormode = 256` emits `\x1b[38;5;N;48;5;M` palette-index SGR **even inside a
  truecolor terminal**, and a config with `colormode = 24bit` emits
  `\x1b[38;2;r;g;b;48;2;r;g;b` regardless of terminal.
- The Go port (tview/tcell) emits **truecolor** when the terminfo entry for
  `$TERM` has the RGB capability (truecolor terminals like ghostty do), **or**
  when `COLORTERM=truecolor|24bit|24-bit` is set — tcell's `LookupTerminfo`
  fabricates the `38;2;r;g;b` escapes for a 256-color terminfo entry in that
  case (tcell `terminfo/terminfo.go`). Its config also defaults `colormode` to
  `24bit`.

So a Python cast recorded with `colormode = 256` and a Go cast recorded in
truecolor will differ in *every* color cell even when both correctly implement
the **same** palette — urwid approximates the truecolor RGB to the nearest
256-palette index, while tcell emits the exact RGB. **Such color differences are
colormode artifacts, not port bugs.** (The existing `python_session.cast` is
256-color precisely because the user's `~/.nomadnetwork/config` has
`colormode = 256`, written by an older nomadnet; the Go cast was recorded in
ghostty and is already truecolor.)

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

To make a valid color comparison, record both sessions in truecolor so the Go
truecolor RGB values can be diffed against the Python truecolor RGB values
directly. The nomadnet config file is `~/.nomadnetwork/config` (or
`~/.config/nomadnetwork/config`), with `colormode` under the `[textui]` section —
not `~/.reticulum` (that is Reticulum's config, separate). The easiest path is
[`record-cast.sh`](#record-castsh--truecolor-asciinema-recorder), which forces
`colormode = 24bit` for Python and exports `COLORTERM=truecolor` for Go. Done by
hand, set `colormode = 24bit` in the nomadnet config and run in a truecolor
terminal (or `export COLORTERM=truecolor` for the Go target).

**What IS comparable across colormodes** (and is what these tools reliably find):
**structural** differences and **text-attribute** differences that are not
color-index approximations — e.g. a border that exists in one and not the other,
an underline attribute that leaks, a menu item that is bold/highlighted in one
and uniform in the other, a footer that is present/absent, a forced background
where the other uses terminal default.

## `record-cast.sh` — truecolor asciinema recorder

`record-cast.sh` produces the `.cast` files that `parse_screencast.py` compares,
and it **forces 24-bit truecolor for both targets** so the resulting casts have no
colormode artifact (see the caveat above). It is the recorder counterpart to
this suite's decoders.

What it forces, and why:

- **Python** — seeds `colormode = 24bit` in the `[textui]` section of a *copy* of
  your nomadnet config. urwid is config-authoritative, so this alone makes
  Python emit `38;2;r;g;b` truecolor SGR regardless of the terminal. (The
  existing `python_session.cast` is 256-color because the user's real config has
  `colormode = 256`; this fixes that without editing the real config.)
- **Go** — exports `COLORTERM=truecolor`, which makes tcell fabricate
  `38;2;r;g;b` truecolor escapes even when `$TERM`'s terminfo lacks the RGB
  capability (e.g. inside a 256-color terminal). Go's own config already
  defaults to `24bit`.

To preserve your real node identity and discovered peers (so the Network page
populates after Ctrl-L), it **copies** your real nomadnet config dir (default
`~/.nomadnetwork`) to a temp dir, forces `colormode = 24bit` in the copy, and
runs the app with `--config <temp>`. Your real config is never modified. Pass
`--fresh` to start from an empty config instead (boots the first-run Guide;
both targets then self-create their default config, already `colormode = 24bit`).

The recorded BYTES are truecolor whether or not the live terminal can render
truecolor — asciinema records what the program emits, not what the terminal
displays — so the `.cast` is directly comparable even if you record in a
256-color terminal (the live preview just looks approximated).

Usage:

```bash
# Python  (was: asciinema rec python_session.cast --command "nomadnet")
tooling/record-cast.sh --target orig --out python_session.cast

# Go port  (was: asciinema rec go_session-002.cast --command "./gonomadnet.sh")
tooling/record-cast.sh --target go --out go_session-003.cast --force

# First-run Guide, Go, truecolor:
tooling/record-cast.sh --target go --out guide_session.cast --fresh
```

`--force` overwrites an existing `.cast` (asciinema refuses to overwrite by
default). `--config DIR` copies from a non-default real config dir; `--bin PATH`
uses a specific `nomadnet` / prebuilt `gonomadnet` binary (default: `nomadnet`
on PATH for `orig`, `go run ./cmd/gonomadnet` for `go`). `--idle SECS` and
`--title TITLE` pass through to asciinema. Interact with the app, then quit it
(Go: menu → Quit / Ctrl-Q; Python: the Quit menu item / Ctrl-C) to end the
recording; the script prints a colormode check of the resulting cast and the
`parse_screencast.py --diff` command to compare it.

Requires `asciinema` (`asciinema rec`), `python3` (stdlib only), and either
`nomadnet` on PATH (`--target orig`) or a buildable repo (`--target go`).

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

## Performance profiling & benchmarking

`cmd/gonomadnet` had severe visible perf problems (cursor flicker while moving
the mouse over Network/Guide, sluggish wheel-scroll on any browser/guide page,
idle CPU). The harness below was built **measure-first**: quantify each hot
path before fixing, then prove each fix with `benchstat` before/after **and** a
re-captured live profile. The harness itself only *measures* — it changes no
production rendering code.

### 1. Headless benchmark suite

Seven layers, all headless via `tcell.NewSimulationScreen("UTF-8")`, so they
isolate the **Go-side render/event CPU** — *not* the real-terminal flush/cursor
cost (that needs the live pprof in §2). Every benchmark uses `b.ReportAllocs()`,
setup outside the loop, and `for b.Loop()` (the Go 1.24+ idiom).

Files:
- `nomadnet/micron/bench_test.go` — Layer 1: `Parse`, `RenderToStyledLines`,
  `RenderToTView`, `BuildAnchorMap` (scaled Realistic/Small/Medium/Large).
- `tui/perf_test.go` — shared helpers: `newBenchScreen`, `drawFrame` (the
  event-loop's `Clear+Draw+Show`), `benchSyntheticMicron`, `benchGuideTopicMarkups`.
- `tui/perf-render_test.go` — Layers 2+3: `StyledLinesToTviewText`, `BodyMarkup`,
  `FormatChannelMessage`, `FormatConversationItem`, `ScrollBarDraw`,
  `GuideReaderDraw`, `BrowserPageViewDraw`, `TextViewDrawWrappedLarge`
  (TopOffset/DeepOffset/WidthOscillate).
- `tui/perf-draw_test.go` — Layer 4: full `MainDisplay.Root().Draw` per body page
  (Guide/Network/Channels/Log/Interfaces/Config/Conversations) + StandaloneBrowser
  + 12-topic Guide corpus.
- `tui/perf-input_test.go` — Layer 5: synthesized `tcell.EventMouse` → handler →
  `drawFrame`. `URWIDColumnsMouseMove`, `PlainFlexMouseMove`, `GuideWheelScroll`,
  `BrowserWheelScroll`, `GuideWheelScrollDeepOffset`.
- `tui/perf-idle_test.go` — Layer 6: `UnreadBlinkTick`, `UpdateUnreadIndicator`,
  `RedrawMenuBar`.

Run them:

```bash
# All benchmarks, count=5, 1s each (writes /tmp/gonomadnet-bench-latest-<ts>.txt):
scripts/run-bench.sh

# Tighter stats / longer per-bench:
BENCH_COUNT=10 BENCH_TIME=3s scripts/run-bench.sh

# Filter by name:
BENCH_PATTERN='ScrollBar|Wheel|MouseMove' scripts/run-bench.sh

# Save a named baseline, then diff a fix with benchstat:
scripts/run-bench.sh SAVE=before.txt
# ...make the change...
scripts/run-bench.sh SAVE=after.txt
benchstat before.txt after.txt
```

`run-bench.sh` env vars: `BENCH_COUNT` (default 5), `BENCH_TIME` (default 1s),
`BENCH_PATTERN` (default `.`), `BENCH_PKGS` (default
`./tui/... ./nomadnet/micron/...`), `GO_TEST_TIMEOUT` (default 10m), `SAVE`.
It sets `GOCACHE=/tmp/go-cache` (repo convention). Benchmarks are **opt-in**:
`test-all.sh`/`test-integration.sh` never pass `-bench`, so CI and `-short` runs
pay zero cost.

Direct form (no script):

```bash
GOCACHE=/tmp/go-cache go test -run='^$' \
  -bench='URWIDColumnsMouseMove|GuideWheelScroll' \
  -benchmem -count=5 -benchtime=1s ./tui/
```

### 2. Live-app pprof harness (real-terminal CPU)

The headless `SimulationScreen.Show()` is a no-op, so it **cannot** reproduce the
terminal flush / cursor / per-`Show` costs that drive the visible flicker.
`cmd/gonomadnet` has an opt-in `-pprof-addr` flag that starts a `net/http/pprof`
server (zero overhead when unset):

```bash
# Terminal A — the app keeps running the whole time:
gonomadnet -textui -pprof-addr 127.0.0.1:6060

# Terminal B — start a 60s CPU recording:
go tool pprof -http :8080 'http://127.0.0.1:6060/debug/pprof/profile?seconds=60'
```

**`seconds=60` is the recording window, not a short snapshot.** The moment you
hit Enter in Terminal B, switch to Terminal A and **continuously reproduce the
slow behavior for the full 60 s** — scroll a long guide page with the wheel
nonstop, move the mouse over Network, etc. When the window elapses, pprof saves
a `.pb.gz` to `~/pprof/` and opens a flame-graph web UI. If you sit idle, you
capture idle — useless. Use a longer window (`seconds=120`) for more samples,
but only if you stay busy the whole time. Capture **one activity per profile**
(one for guide-scroll, one for mouse-move) for clean attribution; mixed captures
are hard to read.

Text analysis (an image-capable model is not required — the flame graph is just
a visualization of this same data, and the text form is better for precise
attribution):

```bash
F=~/pprof/pprof.gonomadnet.samples.cpu.002.pb.gz   # any saved profile

# Top by cumulative (inclusive) CPU — the expensive call trees:
go tool pprof -top -nodecount=40 -cum "$F"

# Top by flat (self) CPU — the functions burning CPU themselves:
go tool pprof -top -nodecount=40 "$F"

# Interactive: callers/callees of a symbol (regex), or annotated source:
printf 'peek draw\npeek wrappedRowCount\n' | go tool pprof "$F"
printf 'list WordWrap\n' | go tool pprof "$F"
```

How to read it: in `-cum`, the widest bars near the top are where CPU goes.
`Application.draw` at ~95% means almost all CPU is redrawing. `uniseg.StepString`
/ `parseTag` = per-cell text render; `syscall.rawsyscalln` / `Show` = terminal
flush; `resize` / `WindowSize` = tcell's per-`Show` winsize ioctl;
`QueueUpdateDraw.gowrap1` = background-driven redraws. In the web UI, use the
regex filter box (`tui|micron`) to hide runtime noise. The `.pb.gz` is
self-contained — reopen it any time with `go tool pprof -http :8080 <file>`; the
app need not be running.

The live profile sees costs the benchmarks cannot: the terminal write syscall,
the per-`Show` resize ioctl, real-cursor cost. Use **both**: benchmarks prove a
Go-side fix and give a number to diff; pprof proves the flush/redraw benefit
downstream and localizes anything the benchmarks miss.

### 3. Fixes applied (baseline context)

Measured-then-fixed so far (BEFORE baselines in the repo-root
`gonomadnet-bench-latest-1786222767.txt`):

1. **Mouse-move flicker** — `tui/urwid-columns.go` `MouseHandler` no longer calls
   `SetFocusIndex`/`setFocus`/consumes on `MouseMove` (click/scroll/drag
   unchanged). `BenchmarkURWIDColumnsMouseMove` 895 µs / 8692 allocs → 204 ns /
   9 allocs (~4400×).
2. **Guide scroll re-wrap** — `tui/scroll-bar.go` caches the wrapped row count
   keyed on `(GetText(false) raw text, width)`; scrolling is a cache hit so
   `Draw` no longer tag-strips + `WordWrap`s the whole document every frame.
   `BenchmarkGuideWheelScroll` 0.89 ms → 0.66 ms / notch (~25% on the small
   synthetic doc; the live profile showed ~60% of guide-scroll CPU was this
   re-wrap on the real large guide topic).

### Conventions

`GOCACHE=/tmp/go-cache` (repo convention). On macOS use `/tmp` explicitly for
temp dirs you inspect, never `os.MkdirTemp("")`/`$TMPDIR`. Hyphenated filenames
(`perf-render_test.go`; the `_test.go` suffix is required by Go). Modern
`for b.Loop()` over `for range b.N`. Run `gopls check` after edits.

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

===

# Debug techniques

# 1. Find the process and see CPU% — deadlock ≈ 0% CPU, starvation/loop ≈ high CPU
pgrep -fa gonomadnet          # or: pgrep -fa nomadnet
ps -o pid,%cpu,%mem,etime,command -p <PID>

# 2. INSTANT deadlock diagnosis: Go dumps every goroutine stack to STDERR
kill -QUIT <PID>
# Capture the dump: if it's in a tmux/terminal, copy the scrollback;
# if it's a -d daemon, its stderr went to a file (e.g. nohup.out / your launcher log) — grab that.

# 3. The app's own log (~1hr of wedge history, last events before the freeze)
tail -300 ~/.nomadnetwork/logfile ~/.nomadnetwork/logfile.1
