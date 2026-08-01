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
      (Separate borders + no outer border + mode-titled left pane done. Saved
      Nodes is now the default — Python `list_display=1`, Network.py:1638 — and
      the KnownNodes empty-state is ported: centered warning-colored info glyph +
      "Currently, no nodes are saved\n\nCtrl+L to view the announce stream"
      shown in place of the list when no nodes are saved, Network.py:833-882;
      `nodesView()` swaps empty-state↔list, `refreshNodesView()` updates it on
      `UpdateNodes`. Verified vs `pynet_80x24_03`: content matches; remaining
      gaps: LocalPeer (PACK) panel below the list, right pane is still the detail
      TextView not the Browser "Remote Node", and a 1-col centering offset on
      the empty-state (urwid left-pads ceil, tview floors) — all need RNS/4.E.)
- [ ] `AnnounceInfo` — announce detail view with Back/Connect/Msg Op/Save
      (node), Use-as-default (pn), Converse (peer) buttons. The in-detail
      actions fire callbacks (`OnSaveNode`/`OnMsgOp`/`OnUseAsPN`/`OnConverse`)
      tested to pop back to the stream; `OnConverse`/`OnMsgOp` wired to create a
      directory entry + switch to Conversations via `MainDisplay.SelectPage`.
      `OnUseAsPN` still pending a `SetDefaultPropagationNode` app method.
      Constructor bug fixed (`announceData` now stored so `SelectedAnnounce`/
      detail work for constructor-supplied announces). Verify vs Python.
- [ ] `KnownNodeInfo` — node detail dialog (name/sort edits, trust radios, default
      propagation node + identify checkboxes, Back/Connect/Msg/Save). Verify.
- [ ] `LXMFPeers`/`LXMFPeerEntry` — peers list; `C-x` unpeer, `C-r` delivery sync.
- [ ] `LocalPeer` / `NodeInfo` / `NetworkStats` — stat panels (1 s refresh).
      (NetworkStats widget done — bordered “Network Stats” panel, injected
      count providers, 5 s refresh; LocalPeer/NodeInfo still pending, need RNS
      data + ReadlineEdit name field + save/announce dialogs.)
- [ ] `BrowserFrame.keypress` — `C-w/d/f/r/u/s/b/y/g` per §Network; `up`→menu.

#### 4.C Conversations (Python `Conversations.py`, 3093 lines)
- [ ] `ConversationsDisplay` two-pane: list (52) + detail; `[ Trusted (0) ]
      [ Untrusted (0) ]` sub-tabs — digit prefixes removed and unread-glyph
      alert counts added (golden-tested vs Python `_label`); the two tabs are
      still a single centered TextView, not two clickable `TabButton`s.
      Verify layout vs original.
- [ ] `ConversationWidget` — trust banner, messages, editor, footer; verify vs
      Python `ConversationWidget`.
- [ ] `sendMessage` (`C-d`) — send editor content; verify vs Python `send_message`.
- [ ] `attachFile`/`fileBrowserClosed`/`saveFocusedAttachments` (`C-f`/`C-s`/`C-a`)
      — attachment flow; verify vs Python.
- [ ] `paperMessage` (`C-p`) — print_qr/save_qr/save_uri; verify vs Python.
- [ ] `ClickableAttachment`/`FileBrowserDialog` — attachment save dialog; verify.
- [ ] `syncConversations`/`updateSyncDialog` (`C-r`) — sync flow + progress; verify.
- [ ] `ingestLXMURI` (`C-u`) — parse `lxm://` + create message; verify vs Python.
- [ ] `newConversation` (`C-n`) — overlay dialog Addr/Name/trust radios/Create/Back.
- [ ] `editSelectedInDirectory` (`C-e`) — peer info editor overlay; verify.
- [ ] `toggleFullscreen` (`C-g`) — wired: list pane collapses to width 0 via
      `Flex.ResizeItem` and restores (draw-tested on a simulation screen);
      remaining: visual verify via `parity.sh`.
- [ ] Three shortcut bars by focus region (list/body/editor); verify via
      `summary.py` footer after focusing each region.

#### 4.H Channels (Python `Channels.py`, 2285 lines)
- [ ] `ChannelsDisplay` two-pane: list (36) + room; verify layout vs Python.
- [ ] `composeListWidgets` — hub/room list; verify vs `_compose_list_widgets`.
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
