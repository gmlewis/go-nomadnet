# Go NomadNet — Port to 100% Behavioral Parity with the Python Original (TDD)

> **Mission:** Reach 100% behavioral parity between this Go port and the
> source-of-truth Python `nomadnet` v1.2.6 (urwid). Work autonomously, top to
> bottom, one small TDD task at a time. This file is the single work order and
> the only tracker — when a task is done, **remove it**.

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
  measures of progress. “It looks right” is not done.
- **TODO.md must shrink over time.** When a task is completed, **remove it
  entirely**. Do not mark “done” or strike it through. This file getting shorter
  is the progress signal.
- **Python is READ-ONLY.** Never edit the original sources (see locations below).
- **Every new file starts with the standard copyright header**
  (`copyright_header.txt`).
- **Style:** hyphens in filenames, `go fmt`, `any` over `interface{}`, `%v` in
  fmt strings, godoc package comment in a same-named file.
- **Do not create tracking/progress markdown files.** This TODO.md is the only
  tracker. Put findings/decisions into commit messages or code comments.
- **Work in listed order within a phase.** Phases are dependency-ordered; later
  phases depend on earlier ones. If a task is blocked by an incomplete
  prerequisite, do the prerequisite first.
- **After every TUI task, re-run the parity tooling** for the affected page
  (see Verification Method) and confirm the Go summary is converging on the
  original’s. Paste the `summary.py` output when claiming parity.

## Verification Method (how “done” is measured)

**Pure logic** (parsers, formatters, config, RRC protocol, micron *parsing*):
1. Capture Python golden values with `tooling/micron-parity/*.py` (reads micron
   lines from stdin, emits JSON) or by running the Python in `/tmp`. Encode them
   as a Go table-driven test.
2. Implement until `go test` is green.

**TUI rendering & behavior** (layouts, menu, focus, colors, keybindings, dialogs):
1. Capture the original with `tooling/tui-parity/capture.sh --target orig ...`
   and the Go port with `--target go ...` (or both at once via `parity.sh`).
2. Compare with `tooling/tui-parity/summary.py` (menu items, border style,
  footer, focus-row bg colors, all-bold heuristic) and `ansiview.py --focus`.
3. For widget *logic*, unit-test with a mocked `tview.Application` (no real
  terminal) asserting on the widget tree / callbacks, with expected values
  captured from the Python original.

**Cross-process** (RNS fetch, LXMF, RRC): integration test with a temporary TCP
RNS transport between a Go process and a Python subprocess; assert byte-for-byte
/ on-disk-layout equality.

---

## Source of truth — reference facts (inline so you never need TUI-ANALYSIS.md)

> These facts are the spec. Tests should assert against them. They are condensed
> from a live headless analysis of the original (urwid 4.0.3, default dark theme
> + 24-bit colormode + Nerd Font glyphs) and from the Python source.

### Locations
- **Python source (READ-ONLY):**
  - Checkout: `/Users/glenn/src/github.com/markqvist/nomadnet`
  - Installed (identical, always available): `/opt/homebrew/lib/python3.14/site-packages/nomadnet/`
  - Runnable binary (for live captures): `/opt/homebrew/bin/nomadnet`
- **Python TUI source:** `nomadnet/ui/textui/*.py` + `nomadnet/ui/TextUI.py` +
  `nomadnet/NomadNetworkApp.py`.
- **Go port:** `tui/` (widgets), `cmd/gonomadnet/` (entrypoint + wiring),
  `nomadnet/{app,util,rrc,config,asciichart,micron,directory,peersettings,storage,version,node,conversation}`.
- **Parity tooling:** `tooling/tui-parity/` (live capture + summary) and
  `tooling/micron-parity/` (micron golden-value extraction). Read their READMEs.

### Top-level layout (Python `Main.py`)
Root is a `urwid.Frame`: **header** = `MenuColumns` of `[ Name ]` buttons (with a
leading Nerd Font menu-indicator glyph), **body** = the active sub-display,
**footer** = the active sub-display’s shortcut-bar `Text`. Both header and footer
have a filled background (`menubar`/`shortcutbar`: `#111` on `#bbb`). First run
opens the **Guide**; otherwise **Conversations**. A 1 s intro splash precedes it.

### Menu model & global keybindings (the part the port gets most wrong)
- From the body, **`Up` at the top of any list** → focus the menu header. This is
  repeated in *every* page’s list `keypress`.
- In the menu: **`Left`/`Right`** move between `[ Name ]` buttons; **`Enter`/`Space`**
  activates; **`Tab`/`Down`** → back to body.
- **`Left`/`Right` inside a page move focus between its panes** (list↔detail,
  channel list↔users, topic list↔reader) — they do **not** switch pages.
- **`Ctrl-Q`** is the only global quit. `Esc` closes dialogs (it is NOT a quit).
- **No digit-prefix menu shortcuts** exist in the original.
- Menu items, on-screen order, **8 items**: `[ Conversations ] [ Network ]
  [ Channels ] [ Log ] [ Interfaces ] [ Config ] [ Guide ] [ Quit ]`. `Directory`
  and `Map` are **not** top-level pages (remove them from the Go menu).

### Per-page keybindings (spec for the Go handlers)
**Conversations** — list: `C-e` peer info · `C-x` delete · `C-n` new · `C-u`
ingest URI · `C-r` sync · `C-g` fullscreen · `C-o` sort · `C-p` my LXMF/QR ·
`tab`→menu. Editor: `C-d` send · `C-p` paper · `C-f` attach · `C-s` save ·
`up`→message list. Body: `tab` toggle editor↔msglist · `C-w` close · `C-u` purge
failed · `C-t` toggle title · `C-x` clear history · `C-g` fullscreen · `C-o`
sort · `C-a` attach · `C-s` save.
**Network** — `C-l` toggle Known Nodes↔Announce Stream · `C-g` fullscreen ·
`C-e` node info · `C-p` reinit LXMF peers · `C-w` disconnect · `C-d` back ·
`C-f` forward · `C-r` reload · `C-u` URL · `C-s`/`C-b` save node · `C-y` copy
URL · `C-x` remove entry.
**Channels** — list: `C-n` new hub · `C-a` join room · `C-r` connect · `C-w`
disconnect · `C-t` auto-reconnect · `C-e` edit hub · `C-x` remove · `C-y`
toggle channel list · `F8` collapse join/part · `tab`→menu. Room editor:
`tab` nick-complete · `C-d` send · `C-x` leave · `F8`. Room body: `C-x` leave ·
`C-u` toggle users · `C-y` toggle channel list · `F8` · `tab`→editor.
**Interfaces** — `C-a` add · `C-x` remove · `C-e` edit · `C-w` config editor ·
`enter` show. Detail: `tab`/`shift-tab` cycle focus, `h`/`v` horizontal/vertical
charts.
**Guide** — `TopicList`: `up` at first→menu, `enter`/click opens topic.
`GuideColumns`: `left`/`right` move focus between topic list and reader.
`LinkableText`: `left`/`right` move cursor between styled parts; on a link part
`enter`/click activates; `up`/`down` scroll; `left` at position 0 releases focus
back to the topic list.
**Log/Config** — embedded `urwid.Terminal` (`tail -fn50 logfile`; `editor
configpath`) with `escape_sequence="up"` so `up` escapes the terminal to menu.
**Readline** (every input field, `ReadlineEdit.py`): `C-a`/`C-e` line start/end ·
`C-u` kill to start · `C-k` kill to end · `C-w` kill word · `C-l` kill buffer ·
`C-y` yank · `C-left`/`C-right` word motion; a **module-global kill ring** shares
kills across widgets.
**List scrolling** (`IndicativeListBox`): plain `up/down/pgup/pgdn/home/end` move
selection; an `up` at the top propagates to the parent (menu-escape); `▲`/`▼`
indicators show scroll position.

### Colors / styles (spec for `tui/theme.go` + micron rendering)
- A **5-tuple palette** per style (16-color, monochrome, 88/256/true-color),
  selected by config `colormode` (`monochrome/16/88/256/24bit`; shipped default
  **24-bit**). Below 256, reset/restore the terminal palette. **Micron styles
  are synthesized per-depth** from the 5-tuple at render time
  (`MicronParser.make_style`). The Go `nomadnet/micron/color-depth.go` already
  implements `monoColor/lowColor/highColor` — the TUI must **wire it in**.
- Key truecolor (dark) styles: `menubar`/`shortcutbar` `#111`/`#bbb`;
  `list_focus` `#111`/`#aaa`; `list_off_focus` `#111`/`#777`; trust row colors
  `list_trusted` `#6b2`, `list_untrusted` `#a22`, `list_unresponsive` `#b92`,
  `list_unknown` `#bbb` (each with a `list_focus_*` variant; untrusted/
  unresponsive rows are **background-colored across the whole row**); message
  header by state (`msg_header_ok` green, `_caution` amber, `_sent` grey,
  `_propagated`/`_delivered` blue, `_failed` grey); trust banner
  `msg_warning_untrusted` red; RRC `irc_nick_self` `#6c5`, `irc_nick_peer`
  `#3cd`, `irc_mention` `#fb4` bold, `irc_notice` `#fd3`.
- **Glyphs** (`ui/TextUI.py:140-172`): `plain`/`unicode`/`nerdfont` sets (shipped
  default **nerdfont**). Menu indicator `󰐻` (or `unread_menu` when unread,
  refreshed every 2 s); node glyph `󰙎`; `check` ✓, `cross` ✕, `unread` ✉, etc.
- **Borders:** single-line `┌─┐` `│` `└─┘` (the port wrongly uses double-line
  `╔═╗`). Interfaces detail uses rounded `╭─╮`.

### Micron markup (spec for `nomadnet/micron` + `tui/micron-view.go`)
Backtick-based markup. `>` headings (depth = count; heading slug auto-anchor);
`<` reset section; `-` divider (`-X` custom char); `` `= `` literal toggle;
`` `t `` table; `` `{ `` partial; `#` comment; `\` escape. Inline (backtick
entering formatting): `` `_ `` underline, `` `! `` bold, `` `* `` italic,
`` `F`` ``/`` `f `` fg color (3 or 6 hex), `` `B`` ``/`` `b `` bg, `` `gNN ``
grayscale, `` `c``/`l``/`r``/`a `` align, double-backtick full reset, `` `: ``+name
anchor, `` `< `` input field (flags `!` masked/`?` checkbox/`^` radio + optional
width, default 24), `` `[ `` link `label`url`fields]. Section indent = 2 per level.
**`LinkableText`/`LinkSpec`**: links are focusable parts (carry target+fields);
`enter`/click activates → `delegate.handle_link(target, fields)`; `#name` URLs
are in-page anchors; 2 s key-timeout shows/hides cursor and peeks the focused
link in the footer (“Link to …”). Output is a **list of `AttrMap`-wrapped rows**
with `anchors` and `header_rows` metadata — not one flat string.

### Dialogs (spec for `tui/dialog.go`)
All dialogs are `urwid.Overlay(LineBox(Pile), bottom, …)` — **true modal
overlays preserving the underlying screen**. `Esc` universally closes (via per-
module `DialogLineBox` subclasses). Forms use `ReadlineEdit` fields, trust
`RadioButton`s, and `Button` rows. Example: New Conversation has `Addr`, `Name`,
trust radios `( ) Untrusted / ( ) Unknown / ( ) Trusted`, `< Create >  < Back >`.

### Rendering / robustness (spec for the port)
- `urwid.raw_display.Screen()`, UTF-8 forced, mouse on. Mouse: click menu
  buttons, list entries, links, pane gutters, expand gutters.
- Widgets reflow to terminal size; no enforced minimum, but the Guide recommends
  **135×32**. At **80×24** the original still works (menu clips, two-column body
  survives, shortcut bar wraps to two lines). **The Go port must not crash on
  resize** (it currently does).
- `stty -ixon` so `Ctrl-Q`/`Ctrl-S` reach the app.
- Background threads wake the UI loop via `watch_pipe` (Go equivalent: marshal
  to the tview main loop). Periodic jobs: 2 s menu unread indicator, announce
  stream, 30 s sync status, 1 s bandwidth charts.

---

## CURRENT STATE SUMMARY (as of consolidation)

- ~40k lines of Go across 14 packages; existing tests pass; `go build`/`go vet`
  clean. The port compiles, runs, and streams real RNS announce data.
- **Core library** packages are partially implemented with major gaps in
  conversation message I/O, directory persistence, app orchestration, RRC
  networking, and micron interactive rendering.
- **The TUI is a non-functional scaffold behaviorally.** `cmd/gonomadnet/textui.go`
  has ~25 TODO markers; most menu actions show placeholder dialogs. The browser
  does not fetch over RNS, channels do not connect to hubs, conversations cannot
  send messages, interfaces show hardcoded data.
- **Top behavioral regressions to fix first** (verified by live capture):
  1. `Left`/`Right` are globally hijacked to switch pages (`tui/main.go:247-258`)
     — must move focus between panes instead.
  2. Menu is 10 items, wrong order, no `[ Name ]`, no indicator glyph, adds
     `Directory`/`Map` top-level (`tui/theme.go:296-307`).
  3. Dialogs are `SetRoot` root-swaps, not overlays; forms stripped (`tui/dialog.go`).
  4. Selection bg is hardcoded `#666` not `#aaa`; no trust row coloring; no
     color-depth detection (`RegisterThemeStyles` is a no-op).
  5. Wrong shortcut bar on every page (`textui.go:195` returns Conversations bar).
  6. Micron is one flat all-bold `TextView`; links inert; no fields/tables/
     partials/anchors.
  7. UTF-8 `U+FFFD` glitches; double-line borders; resize crash.

---

## DEFINITION OF DONE — 100% behavioral parity

The port is at parity when **all** of the following hold (each backed by a green
test or a matching `parity.sh` summary):
- [ ] Menu: 8 items, `[ Name ]`, indicator glyph, Conversations first, no
      Directory/Map top-level.
- [ ] Focus model: `Up`→menu, `Left/Right` in menu move between buttons, `Enter`
      activates, `Tab/Down`→body, `Left/Right` in body move between panes,
      `Ctrl-Q` only global quit, `Esc` closes dialogs, no digit menu shortcuts.
- [ ] Every per-page keybinding in the reference tables above is implemented and
      tested.
- [ ] Every page has the original two-pane layout (fixed list + weighted detail)
      with single-line `┌─┐` borders (Interfaces detail rounded).
- [ ] Colors: depth-aware (mono/16/256/true), `list_focus` `#aaa`, trust row
      backgrounds, `menubar`/`shortcutbar` fill, Nerd Font glyphs + fallbacks.
- [ ] Shortcut bar is correct per page and per focus-region (3 bars on
      Conversations).
- [ ] Micron: structured styled rows, clickable links (`LinkableText`/`LinkSpec`)
      with footer peek, input fields, tables, partials, `#anchor` jumps; body
      text not all-bold.
- [ ] Dialogs are true overlays preserving background; `Esc` closes; full forms.
- [ ] Readline kill/yank with a global kill ring in every input.
- [ ] Mouse: click menu, list entries, links, pane gutters, expand gutters.
- [ ] UTF-8 clean; reflows at 80×24; no resize crash; 1 s intro splash.
- [ ] Terminal hardware cursor positioned on focused micron pages (`LinkableText`)
      and text inputs (`ReadlineEdit`) so the green block cursor matches the
      original (capture-invisible — verified by a `calc_coords` golden test, not
      `parity.sh`).
- [ ] All pages functional (Conversations send/attach/trust; Browser fetches over
      RNS; Channels connect via RRC; Interfaces real enumeration + forms; Config
      editor; Log live tail).
- [ ] Cross-process integration tests green (RNS page fetch, LXMF ingest, RRC
      HELLO/MSG).
- [ ] `go test ./...`, `go vet ./...`, `gofmt -l .` clean; every package has a
      godoc package comment in a same-named file.

---

## TASK LIST (ordered for success — work top to bottom)

> **Where to start:** The first available task. Each `[ ]` is one TDD unit. Remove it
> when green. Phases are prerequisites for the ones below them.

### Phase 0 — Foundation & cross-cutting infrastructure (do first; everything depends on it)

- [ ] **Terminal cursor parity (the “solid green rectangle”).** The green cursor
      the user sees in the Python original is the **terminal emulator’s hardware
      cursor**, which urwid positions by setting `canvas.cursor = (x, y)` on a
      focused widget’s render output — `LinkableText.render` (MicronParser.py:982-
      992) does `c.cursor = self.get_cursor_coords(size)` (maps `_cursor_position`
      → `(x,y)` via urwid `calc_coords`), and `urwid.Edit`/`ReadlineEdit` position
      it at `edit_pos`. The Go port tracks the cursor **offset** (`tui/linkable-
      text.go`: `cursor`/`CursorVisible()`; `tui/readline.go`: `cursorPos`) but
      **never calls `screen.ShowCursor`** (`grep ShowCursor tui/*.go` is empty), so
      no hardware cursor ever appears. This was missed because `tmux capture-pane`
      records the cell buffer, not the terminal cursor overlay — the cursor is
      invisible to captures in *both* versions. Split into:
  - [ ] `LinkableText` / micron pages: in the focused page’s `Draw`, when
        `CursorVisible(now, focused)` is true, port urwid `calc_coords` over the
        line-wrapped styled text to map the `cursor` rune offset → `(x, y)` and
        call `screen.ShowCursor(x, y)`. tview’s `Application.Draw` calls
        `HideCursor` each frame, so the cursor must be re-shown on every focused
        draw. Golden `(x,y)` table captured from Python `calc_coords` over a known
        wrapped string (unit test — capture cannot see the cursor).
  - [ ] `ReadlineEdit` visible caret: wire the model `cursorPos` to a shown
        hardware cursor (tview `InputField`/`textArea` calls `ShowCursor`
        internally when it has focus, but our wrapper bypasses the public cursor
        setter — see the Phase 0.5/0.6 note “tview InputField exposes no public
        cursor setter, displayed caret may lag”). Test that the reported caret
        column tracks `cursorPos` after a non-end cursor move.
- [ ] Under the Network panel, the discovered nodes should be, by default, sorted by descending time.
- [ ] Mouse clicks do not work because they appear to have no correlation with what is under the mouse click and needs to be fixed.
- [ ] Go errors should NEVER be ignored.

### Phase 1 — Core interaction model (highest-impact TUI fixes; unblocks all page work)

### Phase 2 — Core library prerequisites (needed by the TUI wiring in Phase 5)

- [ ] *(Plus the app/conversation/directory methods referenced by Phase 5 wiring
      5.x — add a TDD task per method as they become prerequisites. Capture
      expected behavior from the Python `nomadnet/` core.)*

### Phase 4 — TUI page widgets (two-pane layouts, per-widget keys, mouse)

> For each page: build the two-pane layout (fixed list + weighted detail, single-
> line borders per 0.4), the per-sub-widget `SetInputCapture` handlers (§Per-page
> keybindings), mouse handlers, and unit-test widget logic with a mocked
> `tview.Application`. Capture expected layout/keys with `capture.sh --target
> orig` and compare with `summary.py`.

#### 4.N Network (Python `Network.py`, 1974 lines)
- [ ] `NetworkDisplay` left-pane composition: `NetworkLeftPile` of Saved Nodes +
      `C-l` toggle Announce Stream/Known Nodes + LXMF Peers + LocalPeer (PACK) +
      NodeInfo + NetworkStats; right pane = Remote Node Browser (needs 4.E).
      (Done: the left pane is now a PILE of two separately-bordered LineBoxes —
      the mode-titled list box (Saved Nodes/Announce Stream/Announce Info) and
      the Local Peer Info panel — with NO outer border, matching Python's
      NetworkLeftPile (Network.py:1641, 867, 446, 256). `listBox` carries the
      list slot's border+title; `setLeftList` swaps content+title; `leftPanel`
      is the unbordered `[listBox(weight), localPeer(pack 10)]` stack. Saved
      Nodes is the default — Python `list_display=1`, Network.py:1638. The
      KnownNodes empty-state is ported via `centeredText` (ceil-left centering,
      matching urwid; tview AlignCenter floors → was 1 col off), Network.py:833-
      882. Capture-verified at 80x24 (`Up,Right,Enter`): the WHOLE Network page
      is now byte-identical to Python (left pane Saved Nodes empty-state +
      Local Peer Info; right pane "Remote Node" browser disconnected; shortcut
      bar wraps identically) EXCEPT the identity/LXMF hashes (different seeded
      identity files — expected) and the announce age (capture-timing). The
      shortcut-bar wrap point is FIXED (see the wrap item below). Remaining
      gaps: right-pane Browser page FETCHING/rendering (Phase 5 RNS link — the
      disconnected boot state is parity).)
      (AnnounceInfo detail view DONE + capture-verified: `tui/announce-info.go`
      ports Python's AnnounceInfo Pile (Network.py:59-256) — labels/glyphs,
      Oprtr row inserted at index 4 for nodes, trust palette color, flat <>
      buttons with weights [11,2,11,2,11,2,11] (node) / [23,5,22] (pn/peer),
      data truncation when not trusted, Esc→stream. Directory fields resolve
      at view time via `OnResolveAnnounceInfo` (wired in textui.go); op_str is
      "Unknown" until Phase 5 RNS identity recall. KEY fix: the Pile is wrapped
      in a new `pileFiller` primitive (`tui/pile-filler.go`) replicating
      urwid.Filler(valign=TOP, height=PACK) — when the natural Pile height
      overflows the bordered slot (node = 11 rows vs 9-row-inner slot at
      80x24), it trims the TOP to keep the focused button row visible at the
      bottom (urwid Filler.render cursor-trim, filler.py:228-238), clipping
      Time+Addr. tview's Flex does NOT clip overflow (children spill past their
      allocation and corrupt the LocalPeer panel below), so the dedicated
      primitive is required. Capture-verified at 80x24: Announce Info box shows
      Type→Name→Oprtr→Trust→divider→Announce Data:→data→divider→buttons with
      the button row on the last inner row and LocalPeer intact below.
      `focusLeftList` (SetFocus the listBox after UI-thread swaps) makes Enter
      open the detail view (tview dispatches keys to the ROOT primitive via a
      top-down Flex cascade; a freshly-swapped item has no focused descendant
      so Enter is lost without it). Pinned by TestAnnounceInfo*Layout/
      DataTruncation/EscReturnsToStream/FallsBackWithoutResolver/
      TopTrimClipsHeader/PeerTopTrim + TestPileFillerPadsBottomWhenContentFits
      + TestTrustStringAndStyleMapping. The announce LIST content is per-run
      (different announces received), so only STRUCTURE is verifiable, not
      byte-identity.)
- [ ] `KnownNodeInfo` — node detail dialog (name/sort edits, trust radios, default
      propagation node + identify checkboxes, Back/Connect/Msg/Save). Verify.
      (DONE: `tui/known-node-info.go` ports Python's KnownNodeInfo
      (Network.py:593-810) — opened by Ctrl-E on a saved node (NOT Enter;
      Python `node_list_selection` is a no-op, Network.py:888, Ctrl-E →
      selected_node_info → KnownNodeInfo, Network.py:1603). A TOP-filled Pile
      (wrapped in the `pileFiller` from 4.N's AnnounceInfo work) of: Type /
      Name(ReadlineEdit) / Node Addr / Operator / Distance / Sort
      Rank(ReadlineEdit) / divider / centered LXMF-PN-address line / divider /
      2 CheckBoxes (default PN / identify on connect) / divider / 3 trust
      RadioButtons (Untrusted/Unknown/Trusted) / divider / weighted
      Back/Connect/Msg Op/Save button row. Operator + Distance are inserted at
      pile indices 3 and 4 (Network.py:789,797). Labels align the colon at
      column 10 ("Type      : ", "Name      : ", "Node Addr : ", "Operator  :
      ", "Distance  : ", "Sort Rank : "). pile.focus_position = last (button
      row), button_columns.focus_position = 0 (Back) — replicated via
      `pile.SetFocusIndexLast()`. Trust radios preselect by incoming
      TrustLevel (untrusted/unknown/trusted); "warning" leaves none checked
      (Python falls through). FormData() collects edited name/sort-rank, the
      radio-selected trust (defaults "untrusted"; Unknown/Trusted override per
      Python's if/elif chain, Network.py:755-785), and the two checkbox states.
      Esc → showKnownNodes (the pileFiller intercepts Esc via onEsc, since the
      app-level/page-level Esc forwarding doesn't reach the swapped left pane
      — same gap AnnounceInfo hit). Tab/Up/Down cycle focus among the
      selectable fields via the pileFiller's moveFocus (blur old + focus new).
      Wired in `cmd/gonomadnet/textui.go`: `OnResolveKnownNodeInfo` (directory
      DisplayStr/SortStr/TrustLevel/IdentifyOnConnect; op_str/hops/LXMF-PN
      address/UseAsPN stubbed "Unknown"/"No associated …" until Phase 5 RNS),
      `OnKnownNodeSave` (directory.NewEntry + Remember with edited name,
      HostsNode=true, IdentifyOnConnect, TrustLevel switch, SortRank Atoi),
      and `OnEditNode` (Ctrl-E → SelectedNode → ShowKnownNodeInfo). NOT
      capture-reachable (neither seed dir has a `directory` file → Saved Nodes
      empty, so no node to Ctrl-E); parity source-verified against
      Network.py:593-810 + urwid Filler.render. Pinned by TestKnownNodeInfo
      Structure/TrustPreselect/TrustWarningPreselectsNone/FormData/
      FormDataTrustDefault/FocusOnButtonRow/EscReturnsToNodes + the
      pileFiller tests. GAPS deferred to Phase 5: tview.Checkbox "(X) label"
      vs urwid exact glyph (not capture-reachable), RNS-dependent fields
      (op_str via Identity.recall, hops via Transport.hops_to, the PN address
      hash, the current user-selected PN + autoselect).)
- [ ] `LXMFPeers`/`LXMFPeerEntry` — peers list; `C-x` unpeer, `C-r` delivery sync.
- [ ] `LocalPeer` / `NodeInfo` / `NetworkStats` — stat panels (1 s refresh).
      (NetworkStats widget done — bordered "Network Stats" panel, injected
      count providers, 5 s refresh; LocalPeer DONE — see above; NodeInfo
      not-hosting branch DONE — see above; the NodeInfo HOSTING stat lines
      (Last Announce/Storage/Active Links/Total Connects/Pages/Files, 1 s
      refresh) still pending — blocked on Phase 5 node hosting.)
- [ ] `BrowserFrame.keypress` — `C-w/d/f/r/u/s/b/y/g` per §Network; `up`→menu.

#### 4.C Conversations (Python `Conversations.py`, 3093 lines)
- [ ] `ConversationWidget` — trust banner, messages, editor, footer; verify vs
      Python `ConversationWidget`.
- [ ] `attachFile`/`fileBrowserClosed`/`saveFocusedAttachments` (`C-f`/`C-s`/`C-a`)
      — attachment flow; verify vs Python.
- [ ] `paperMessage` (`C-p`) — print_qr/save_qr/save_uri; verify vs Python.
- [ ] `ClickableAttachment`/`FileBrowserDialog` — attachment save dialog; verify.
- [ ] `syncConversations`/`updateSyncDialog` (`C-r`) — sync flow + progress; verify.
- [ ] `ingestLXMURI` (`C-u`) — parse `lxm://` + create message; verify vs Python.
- [ ] `editSelectedInDirectory` (`C-e`) — peer info editor overlay; verify.
- [ ] Three shortcut bars by focus region (list/body/editor); verify via
      `summary.py` footer after focusing each region. `SetShortcutFocus` must be
      wired on every region focus change so the footer text + height track the
      active region.

#### 4.H Channels (Python `Channels.py`, 2285 lines)
- [ ] `composeListWidgets` — hub/room list; verify vs `_compose_list_widgets`
      (the populated-hubs branch — Phase 5 RRC; the no-hubs empty state is DONE
      above).
- [ ] `RoomWidget` — messages + users pane (`C-u` toggle now removes/re-adds
      the users column) + editor; verify vs Python `RoomWidget`.
- [ ] `RoomWidget.sendMessage` (`C-d`) + `handleSlashCommand` + `leaveRoom`
      (`C-x`); `tab` nick-complete is wired to `TabComplete` with member
      candidates + own-nick exclusion (cycling tested). Verify each slash
      command vs `_handle_slash_command`.
- [ ] `updateMessages`/`appendMessage` + `_messageWidget` — message list; verify.
- [ ] `newHubDialog`/`confirmNewHubDialog`/`editHubDialog`/`joinRoomDialog` +
      `showUserInfo` — overlay dialogs; verify vs Python.
- [ ] `autoconnect` — `_maybe_autoconnect` on startup; verify.
- [ ] `F8` join/part collapse (logic wired + `IsJoinPartSystem`/
      `CollapseJoinPartMessages` golden-tested vs Python) and `C-y` toggle
      channel list (pane actually removes/re-adds) — remaining: visual verify
      via `parity.sh` for the Channels page.

#### 4.I Interfaces (Python `Interfaces.py`, 3214 lines)
- [ ] `ShowInterface` — detail with bandwidth charts, `h`/`v` horizontal/vertical
      by width, `tab`/`shift-tab` focus cycle; verify.
- [ ] `AddInterfaceView`/`EditInterfaceView` — per-type forms; verify vs Python.
- [ ] `RNodeCalculator` + `InterfaceBandwidthChart` — field updates + sparkline;
      verify vs Python.
- [ ] `getPortInfo`/`getPortField` — serial port detection; verify.
- [ ] `openConfigEditor` (`C-w`) — launch `$EDITOR` on RNS config and return;
      verify vs Python.

#### 4.B Browser (Python `Browser.py`, 1848 lines — needs Phase 3 micron + Phase 5 RNS)
- [ ] `BrowserDisplay.retrieveURL` — fetch a page over RNS (link establish,
      request, response); test with mocked transport asserting request bytes.
- [ ] `loadPage` — render a retrieved Micron page (Phase 3); verify vs
      Python `load_page`.
- [ ] `downloadFile` — download over RNS; verify vs `download_file`.
- [ ] `partialReceived`/`partialProgressed`/`partialFailed`/`updatePartials` —
      partial handlers; verify vs Python.
- [ ] `cachePage`/`getCached`/`cleanCache`/`uncachePage` — cache management;
      audit limits/eviction vs Python.
- [ ] `identify`/`disconnect` — link identity + disconnect; verify.
- [ ] `saveNodeDialog`/`urlDialog` — overlay dialogs; verify.
- [ ] `markedLink`/`copyUrl` — link mark + clipboard (OSC52); verify.
- [ ] `responseReceived`/`requestFailed`/`requestTimeout`/`responseProgressed`/
      `statusText` — request lifecycle; verify.
- [ ] `jumpToAnchor` — scroll to anchor; verify.

### Phase 5 — Wiring the TUI to the core (`cmd/gonomadnet/textui.go`, ~25 TODOs)
> Implement bottom-up as the underlying app methods land. Each wiring task
> replaces a TODO with a real call into `app`/core and an end-to-end test.

- [ ] Wire `networkDisplay` to live RNS announce/node/peer data (announces +
      known nodes now live via UIChangeCallback; LXMF peers list still pending
      the router peers accessor).
- [ ] Wire network “Navigate” to the real browser over RNS.
- [ ] Wire network “Show peers” to the LXMF peers list from the router.
- [ ] Wire `conversationsDisplay.OnIngestURI` to `Conversation.Ingest` of an LXM URI.
- [ ] Wire block/unblock/ping peer callbacks to `App.BlockDestination`/
      `UnblockDestination` and the RNS link ping (block/unblock wired; ping
      still pending the RNS link ping).
- [ ] Wire `channelsDisplay` to a real `rrc.RRCManager` from the app.
- [ ] Wire `channelsDisplay.OnNewHub`→`RRCManager.AddHub`;
      `OnJoinRoom`→`RRCHub.JoinRoom`; `OnRemoveHub`→`RRCManager.RemoveHub`;
      `OnEditHub`→`RRCHub` display-name update; `OnConnect`/`OnDisconnect`→
      `RRCHub.Connect`/`Disconnect`; `OnToggleAutoReconnect`→`RRCHub.SetAutoReconnect`.
- [ ] Wire `interfacesDisplay` to the real RNS `TransportSystem` interface list;
      `OnAddInterface`→RNS config addition; `OnConfigEditor`→launch external editor.
- [ ] Wire `browserDisplay` to a real `Browser` over RNS (fetch, history, cache,
      save-node, identify, disconnect); `OnToggleFullscreen`→main display toggle.
- [ ] Wire the splash/quit display to the real intro content and graceful shutdown.
- [ ] End-to-end smoke: start `-daemon` with a temp config, verify it registers
      destinations and stays alive until SIGTERM.

### Phase 6 — Integration, mouse & final polish
- [ ] Integration: Go node serves a Micron page, a Python node fetches it over a
      temp TCP RNS transport and the bytes match.
- [ ] Integration: Go app receives an LXMF message from a Python sender and
      ingests it into a conversation identical to Python’s on-disk layout.
- [ ] Mouse parity: click list entries, links, pane gutters, expand gutters
      (13/14 interactions missing per PARITY-REPORT).
- [ ] Background-thread marshaling: RRC/message callbacks marshal UI updates to
      the tview main loop (equivalent of `watch_pipe`).
- [ ] 135×32 recommendation message (small-terminal reflow verified: 80×24
      no crash, two-column body survives, menu clips; Ctrl-Q reaches the app
      via tcell raw mode which disables IXON).
- [ ] Final: run `parity.sh` for every page + the New Conversation dialog;
      confirm each Go `summary.py` output matches the original’s.

---

## How to use the parity tooling (quick reference)

```bash
# Live-capture the original (first-run Guide) and decode focus/colors:
cd tooling/tui-parity
./capture.sh --target orig --size 135x32 --fresh --label guide \
    --keys Left,Down,Down,Down,Down,Down,Down,Enter
python3 ansiview.py --focus captures/guide_135x32_07_esc.txt
python3 summary.py  captures/guide_135x32_00_esc.txt

# Capture the Go port (longer boot for `go run`):
./capture.sh --target go --size 135x32 --keys Right,Down,Down,Down \
    --label network --boot 25

# Both at once, summaries side by side:
./parity.sh --label network --keys-orig Up,Right,Enter --keys-go Right

# Micron parser golden values (no urwid needed):
cd tooling/micron-parity
printf '`[Label`url]\n`<field`data>\n>Heading\n' | python3 micron_inline.py
printf '>Heading\n-\n`{partial_url`5`f1|f2}\n' | python3 micron_parseline.py
```
