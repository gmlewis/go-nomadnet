# tui-parity — Headless TUI capture & parity tooling

This directory contains tooling to drive the **source-of-truth Python `nomadnet`
(urwid)** and the **Go port (`gonomadnet`, tview/tcell)** headlessly, capture how
each renders, and compare them so an agent can continuously verify **behavioral
parity** while porting.

It was built while producing [`TUI-ANALYSIS.md`](../../TUI-ANALYSIS.md), which
documents the original's behavior and the Go port's current gaps. Use these
tools to keep that analysis honest as the port progresses.

## Why this exists

Both TUIs require a real PTY and express focus/highlight via **color and
reverse-video**, not glyphs. A plain `cat` of their output hides exactly the
things that matter for parity (focus movement, trust-based row colors, headings,
menu selection). These tools:

1. run either binary inside a detached `tmux` session (a PTY),
2. inject keystrokes and capture the screen after each key (plain + ANSI-styled),
3. decode the ANSI styling so focus/highlight/colors are visible in plain text,
4. summarize a frame to the few facts that matter for parity, and
5. run the same scenario against both targets and print the summaries side by side.

## Prerequisites

- `tmux` (provides the PTY + `send-keys` + `capture-pane`).
- `python3` (for the decoders; stdlib only, no packages needed).
- The original `nomadnet` on `PATH` (e.g. `brew install nomadnet` / `pip install nomadnet`),
  **or** pass `--bin /path/to/nomadnet`. The default shipped config (dark theme,
  24-bit colormode, Nerd Font glyphs, mouse on, 1 s intro) is used unless you
  supply `--config`.
- The Go port: a buildable repo (`go run ./cmd/gonomadnet`). The harness runs
  `go run` from the repo root, so the first capture per `go run` invocation
  includes compile time (~15–20 s) — see `--boot`.

## Files

| File | Purpose |
|---|---|
| `capture.sh` | Drive one target (orig or go) at a given size with a key sequence; capture frames. |
| `ansiview.py` | Decode a `capture-pane -e` file so styling (focus, colors, bold) is visible. Modes: default, `--plain`, `--focus`, `--json`. |
| `summary.py` | Reduce a frame to a parity summary: menu items, footer, border style, focus rows, all-bold body heuristic. |
| `parity.sh` | Capture the same scenario from both targets and print both summaries. |

Captures are written to an output dir (default `./captures/`) as
`<label>_<W>x<H>_<NN>_{plain,esc,txt}` plus a `manifest.txt`.

## Quick start

```bash
cd tooling/tui-parity

# 1) Original first-run Guide, walk the topic list and open a topic:
./capture.sh --target orig --size 135x32 --fresh --label guide \
    --keys Left,Down,Down,Down,Down,Down,Down,Enter

# See the rendered first frame:
python3 ansiview.py captures/guide_135x32_00_esc.txt

# Find the focus highlight (the selected topic row):
python3 ansiview.py --focus captures/guide_135x32_07_esc.txt

# Structural summary:
python3 summary.py captures/guide_135x32_00_esc.txt
```

## Keystroke vocabulary (`--keys`)

`--keys` is a comma-separated list of **tmux key names** sent via `tmux send-keys`,
one capture taken after each. Common names:

| tmux name | meaning |
|---|---|
| `Up` `Down` `Left` `Right` | arrow keys |
| `Enter` `Space` `Tab` | enter / space / tab |
| `BTab` | shift-tab (back-tab) |
| `Escape` | esc |
| `C-n` `C-x` `C-q` ... | Ctrl+n, Ctrl+x, Ctrl+q, ... |
| `M-n` | Alt+n |
| `F8` | function key 8 |
| `a` `A` `1` `9` | literal characters |

To send a literal comma inside a key, note that `,` is the list separator —
avoid sending a bare comma (you rarely need to in these TUIs).

## How the original is driven (reference)

The original's interaction model (from `TUI-ANALYSIS.md` §1.2):

- From a page body, **`Up` at the top of the list** → focus the menu bar.
- In the menu: **`Left`/`Right`** move between `[ Name ]` buttons, **`Enter`**
  activates.
- **`Tab`/`Down`** from the menu → back to the body.
- **`Ctrl-Q`** quits from anywhere.

So to reach the Network page from the default Conversations landing page:
`--keys Up,Right,Enter`. To then reach Channels: append `Up,Right,Enter` again
(the menu remembers its focus column), etc. See `TUI-ANALYSIS.md` §1.4 for the
full page-walk sequence used to capture every page.

The Go port currently differs: `Left`/`Right` globally switch pages, so reaching
Network is just `--keys Right` (from the default Network-first landing). This
difference is itself a parity bug — `parity.sh` lets you pass separate sequences.

## Parity workflow

The recommended loop while porting a page or feature:

1. **Decide the scenario** — a page + a key sequence that exercises the behavior
   you're porting (e.g. "open the New Conversation dialog").
2. **Capture both** with `parity.sh`:

   ```bash
   ./parity.sh --label newconv --size 135x32 \
       --keys-orig C-n --keys-go C-n --frame 1
   ```
   prints the original summary and the Go summary next to each other.

3. **Eyeball the summaries** for the regressions called out in `TUI-ANALYSIS.md`:
   - `menu_items` — 8 items, `[ Name ]` decoration, Conversations first?
   - `border_style` — `single` (┌) not `double` (╔)?
   - `footer` — the **page-correct** shortcut bar (not Conversations' on every page)?
   - `focus_rows` — a selected list row with the theme `bg` (≈ `('c',170,170,170)`),
     and trust-based row backgrounds present?
   - `body_all_bold_fraction` — low (body text is normal; only headings bold)?

4. **Diff the raw text** when summaries are close but not identical:
   ```bash
   diff <(python3 ansiview.py --plain captures/orig/newconv_135x32_01_esc.txt) \
        <(python3 ansiview.py --plain captures/go/newconv_135x32_01_esc.txt)
   ```

5. **Iterate** on the Go code; re-run `parity.sh` until the summaries converge.

## Useful one-off captures

```bash
# Original at a small terminal (reflow behavior):
./capture.sh --target orig --size 80x24 --label small

# Original New Conversation dialog (true overlay):
./capture.sh --target orig --size 135x32 --keys C-n --label newconv --frame 1

# Go port Network page, then walk the announce list:
./capture.sh --target go --size 135x32 --keys Right,Down,Down,Down --label network --boot 25

# Go port Guide page (check the all-bold bug):
./capture.sh --target go --size 135x32 --keys Right,Right,Right,Right,Right,Right,Right \
    --label guide --boot 25
python3 summary.py captures/guide_135x32_07_esc.txt
```

## Tips, caveats, and known gotchas

- **Boot time.** The original is fast (`--boot 5` default). The Go port via
  `go run` must compile first, so the default is `--boot 25`. Pre-build the Go
  binary (`go build -o /tmp/gonomadnet ./cmd/gonomadnet`) and pass
  `--bin /tmp/gonomadnet` to skip compile time and shrink boot.
- **First-run Guide.** `--fresh` uses a throwaway config dir so the original
  shows its first-run Guide (otherwise it lands on Conversations). Reusing a
  config dir skips the Guide.
- **Concurrent edits.** This repo may be edited while you run the Go port. If
  `go run` fails to build, that's the cause — re-run. The original is
  unaffected.
- **Resize.** `tmux resize-window -x W -y H` against a running session lets you
  probe reflow; the original survives this and reflows. The Go port has been
  observed to crash on resize (a real bug to fix) — if it dies, that's a finding.
- **Focus detection.** `ansiview.py --focus` lists rows with a non-default
  background — the fastest way to see *where* the focus/highlight is. The
  original's focused list row is ≈ `bg=('c',170,170,170)` (#aaa, `list_focus`);
  the Go port's is currently `bg=('c',102,102,102)` (#666, hardcoded — a bug).
- **Color depth.** Captures reflect the terminal's negotiated color support;
  tmux PTYs negotiate truecolor, so the default 24-bit SGR is emitted. To test
  16/256-color fallback, configure the target's `colormode` in its config file
  and re-capture.
- **Don't commit captures.** The `captures/` output dir is for local use; add it
  to `.gitignore` if you start keeping it inside the repo.

## Reproducing the TUI-ANALYSIS.md evidence

The frames quoted in `TUI-ANALYSIS.md` were produced with exactly this tooling
(then stored under `/tmp/nn_capture/`). To re-derive any of them, run the
matching `capture.sh` command from §"Useful one-off captures" above and decode
with `ansiview.py`.