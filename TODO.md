# Go NomadNet — Port to 100% Behavioral Parity with the Python Original (TDD)

> **Mission:** Reach 100% behavioral parity between this Go port and the
> source-of-truth Python `nomadnet` (urwid). Work autonomously, top to
> bottom, one small TDD task at a time. This file is the single work order
> and the only tracker — when a task is done, **remove it**.

---

## CRITICAL RULES

- **One function/method per session.** Each task targets a single function,
  method, or small cohesive unit. If a task feels big, split it into sub-tasks
  and list them — do not implement a whole widget in one go.
- **Tests first, always.** Write a failing Go test whose expected values are
  **captured from the Python source** before implementing. The expected values
  live inside the test (golden tables), not in a separate doc.
- **`go test` is the lie detector.** Run `go test ./...` (set
  `GOCACHE=/tmp/go-cache` on this host) after every change. Never claim a task
  complete without pasting passing test output into your final message.
- **Never self-assess parity.** The test suite + the parity tooling are the only
  measures of progress. "It looks right" is not done.
- **TODO.md must shrink over time.** When a task is completed, **remove it
  entirely**. Do not mark "done" or strike it through. This file getting shorter
  is the progress signal.
- **Python is READ-ONLY.** Never edit the original sources (see locations below).
- **Every new file starts with the standard copyright header**
  (`copyright_header.txt`).
- **Style:** hyphens in filenames, `go fmt`, `any` over `interface{}`, `%v` in
  fmt strings, godoc package comment in a same-named file.
- **Do not create tracking/progress markdown files.** This TODO.md is the only
  tracker. Put findings/decisions into commit messages or code comments.
- **After every TUI task, re-run the parity tooling** for the affected page
  and confirm the Go summary is converging on the original's.

## Source locations

- **Python source (READ-ONLY):** `/Users/glenn/src/github.com/markqvist/nomadnet`
  (also installed at `/opt/homebrew/lib/python3.14/site-packages/nomadnet/`;
  runnable binary: `/opt/homebrew/bin/nomadnet`)
- **Python TUI source:** `nomadnet/ui/textui/*.py` + `nomadnet/ui/TextUI.py` +
  `nomadnet/NomadNetworkApp.py`
- **Go port:** `tui/` (widgets), `cmd/gonomadnet/` (entrypoint),
  `nomadnet/{app,util,rrc,config,asciichart,micron,directory,peersettings,storage,version,node,conversation}`
- **Parity tooling:** `tooling/tui-parity/` (live capture + summary) and
  `tooling/micron-parity/` (micron golden-value extraction). Read their READMEs.

## Verification Method

**Pure logic** (parsers, formatters, config, RRC, micron *parsing*): capture
Python golden values via `tooling/micron-parity/*.py` or by running Python in
`/tmp`; encode as a Go table-driven test; implement until `go test` is green.

**TUI rendering & behavior**: capture both targets with
`tooling/tui-parity/capture.sh --target orig|go`, compare with
`tooling/tui-parity/summary.py` and `ansiview.py --focus`. For widget *logic*,
unit-test with a mocked `tview.Application` (no real terminal).

**Cross-process** (RNS, LXMF, RRC): integration test with a temporary TCP RNS
transport between a Go process and a Python subprocess.

## Parity tooling quick reference

```bash
# Live-capture and compare:
cd tooling/tui-parity
./capture.sh --target orig --size 135x32 --fresh --label guide \
    --keys Left,Down,Down,Down,Down,Down,Down,Enter
python3 summary.py captures/guide_135x32_00_esc.txt

./capture.sh --target go --size 135x32 --keys Right,Down,Down,Down \
    --label network --boot 25
./parity.sh --label network --keys-orig Up,Right,Enter --keys-go Right

# Micron parser golden values:
cd tooling/micron-parity
printf '`[Label`url]\n`<field`data>\n>Heading\n' | python3 micron_inline.py
```

---

## Parity Analysis Findings (from live A/B exploration)

The following bugs and behavioral differences were discovered by running live
instances of both `nomadnet` (Python/urwid) and `gonomadnet` (Go/tview) in
tmux PTYs at 135x32, exploring all menu pages, keyboard shortcuts, and mouse
interactions. Each item is an action item to fix.

### Cursor & Focus

- **FIX: Hardware cursor moves to last-drawn cell after every screen update
  instead of staying at the focused widget position.** In Python nomadnet
  (urwid), the cursor stays sticky-focused wherever it belongs (e.g., on the
  selected menu bar item at row 0, or hidden at (0,0) in a list). In
  gonomadnet (tcell), after ANY asynchronous screen redraw (e.g., the
  "Announced: X seconds ago" line updating to "2 minutes ago"), the hardware
  cursor jumps to the cell that was last painted during that redraw (e.g.,
  from (109,31) to (26,25) near the "Announced" line) and stays there. This
  is the single most visible parity bug. Root cause is likely in tcell's
  `ShowCursor` / draw cycle: tcell sets the cursor to the last cell drawn
  when the application does not explicitly position it, while urwid always
  positions the cursor at the focused widget. The fix must ensure the cursor
  position is explicitly set (or hidden) after every redraw, matching urwid's
  sticky-focus behavior.

- **FIX: Cursor does not stay on the menu bar after Enter selects a menu
  item.** In Python nomadnet, after pressing Enter on a menu item (e.g.,
  Network), the cursor stays on the menu bar at the selected item's position
  (e.g., (23,0) for Network). In gonomadnet, the cursor jumps into the
  content area (e.g., (15,25) in the Local Peer Info section) because
  `contentArea.SwitchToPage()` drives the focus chain and moves focus to the
  new page's widgets. The `selectMenuLocked` comment says "does NOT drop
  focus to the body" but `SwitchToPage`'s side effects override this. Fix:
  after `selectMenu`/`selectMenuLocked`, explicitly restore focus to the
  menu bar if `focusRegion == "menu"`.

- **FIX: Guide topic list focus gets stuck on `*tui.ScrollBar` widget.** In
  gonomadnet, after the Guide page renders, focus lands on the `ScrollBar`
  widget instead of the `IndicativeListBox` (the actual topic list). This
  prevents: (a) keyboard navigation (Up/Down/Enter) in the topic list, (b)
  mouse clicks on topics from moving focus to the list, (c) the Up-to-menu
  transition (because `bodyListAtTop` doesn't recognize `ScrollBar` as a
  list and returns false). In Python nomadnet, the Guide topic list receives
  all keyboard input and Up-at-top moves focus to the menu bar. Fix: ensure
  the `IndicativeListBox` (not the `ScrollBar`) is the focus target after
  the Guide page renders, and/or teach `bodyListAtTop` to recognize
  `ScrollBar` as a list-at-top.

- **FIX: Announce Stream focus chain never reaches the `IndicativeListBox`.**
  In gonomadnet, pressing Tab in the Network Announce Stream view cycles focus
  between `*tview.Flex` and `*tui.pileFiller` containers but never reaches the
  `*tui.IndicativeListBox` (the actual announce list). This prevents keyboard
  navigation of the announce list and the Up-to-menu transition. In Python
  nomadnet, Tab/arrow keys navigate the announce list normally. Fix: ensure
  the focus chain in the Announce Stream includes the `IndicativeListBox`.

### Mouse

- **FIX: Mouse clicks on the menu bar do not navigate to the selected page.**
  In Python nomadnet, clicking on a menu item (Conversations, Network,
  Channels, Log, Interfaces, Config, Guide, Quit) with SGR-1006 mouse events
  navigates to that page. In gonomadnet, mouse clicks on the menu bar are
  silently ignored (the `SetMouseCapture` handler exists but the event never
  fires, possibly because tcell's mouse mode in tmux isn't enabling SGR-1006,
  or the event coordinates don't match). Fix: verify tcell enables SGR-1006
  mouse mode and that the `menuBar.SetMouseCapture` handler receives and
  processes `MouseLeftClick` events correctly.

- **FIX: Mouse scroll wheel does not scroll the Announce Stream list.** In
  Python nomadnet, mouse wheel down/up scrolls the Announce Stream list. In
  gonomadnet, mouse wheel events on the Announce Stream are silently ignored
  (the list doesn't scroll). Fix: ensure mouse wheel events are forwarded to
  the `IndicativeListBox` or the appropriate scrollable widget.

### Layout & Rendering

- **FIX: Ctrl-G fullscreen in Network does not hide the left pane.** In
  Python nomadnet, Ctrl-G in the Network page hides the left pane (Saved
  Nodes / Local Peer Info) entirely and expands the right pane (Remote Node
  browser) to full terminal width. In gonomadnet, Ctrl-G keeps both panes
  visible side-by-side, merely resizing them to roughly equal width. Fix:
  fullscreen should hide the left pane and expand the browser pane to full
  width, matching Python's `NetworkDisplay.toggle_fullscreen` behavior.

- **FIX: Guide "Topics" border title alignment differs.** Python renders the
  border title left-heavy: `┌────────────────── Topics ─────────────────┐`
  (18 dashes left, 16 dashes right). Go renders it centered:
  `┌───────────────── Topics ──────────────────┐` (17 dashes each side).
  Fix: match urwid's border title alignment (left-heavy, not centered).

- **FIX: Announce Stream tab bar formatting differs.** Python renders tab
  labels as `[ Nodes  ] [ Peers  ] [ Propagation Nodes (N)   ]` with item
  counts below on a second line: `  (12)       (49)`. Go renders tab labels
  with inline counts: `[ Nodes (8) ] [ Peers     ] [ Propagation Nodes (11)  ]`
  and a different second-line count layout. Fix: match Python's tab label
  format (no inline count in the tab, counts on the second line below).

- **FIX: URL dialog button labels have slightly different spacing.** Python:
  `< Cancel             >     < Go                 >`. Go:
  `< Cancel              >     < Go                  >` (one extra space in
  each button). Fix: match Python's button label widths exactly.

### Known issues from parity skill (already documented, not re-found)

- **Browser back (Ctrl-D) broken after click-navigated link** — Go's Ctrl-D
  does not return to the previous page after clicking a link (stays on the
  linked page). Python's Ctrl-D correctly returns. (From
  `tooling/parity-mouse.sh` sweep findings.)
- **URL-bar truncation** — Go shows `<hash>:/page/     ` (truncated, no
  ellipsis); Python shows `<hash>:/page/ind…` (ellipsis). (From
  `tooling/parity-ab.sh` findings.)
- **Footer link-peek URL truncation** — Same root cause as URL-bar
  truncation: Go shows no ellipsis, Python does.
- **Line wrapping** — Go wraps at a different word boundary than Python by
  one word in some cases. (From `tooling/parity-ab.sh` findings.)