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

## Source of truth — reference facts (the spec; assert against these)

> Condensed from a live headless analysis of the original (urwid 4.0.3, default
> dark theme + 24-bit colormode + Nerd Font glyphs) and from the Python source.

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

### Menu model & global keybindings
- From the body, **`Up` at the top of any list** → focus the menu header. Repeated
  in *every* page’s list `keypress`.
- In the menu: **`Left`/`Right`** move between `[ Name ]` buttons; **`Enter`/`Space`**
  activates; **`Tab`/`Down`** → back to body.
- **`Left`/`Right` inside a page move focus between its panes** (list↔detail,
  channel list↔users, topic list↔reader) — they do **not** switch pages.
- **`Ctrl-Q`** is the only global quit. `Esc` closes dialogs (it is NOT a quit).
- **No digit-prefix menu shortcuts** exist in the original.
- Menu items, on-screen order, **8 items**: `[ Conversations ] [ Network ]
  [ Channels ] [ Log ] [ Interfaces ] [ Config ] [ Guide ] [ Quit ]`. `Directory`
  and `Map` are **not** top-level pages.

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
  (`MicronParser.make_style`). `nomadnet/micron/color-depth.go` implements
  `monoColor/lowColor/highColor` — the TUI must wire it in.
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
- **Borders:** single-line `┌─┐` `│` `└─┘`. Interfaces detail uses rounded `╭─╮`.

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
  resize.**
- `stty -ixon` so `Ctrl-Q`/`Ctrl-S` reach the app (tcell raw mode disables IXON).
- Background threads wake the UI loop via `watch_pipe` (Go equivalent: marshal to
  the tview main loop). Periodic jobs: 2 s menu unread indicator, announce
  stream, 30 s sync status, 1 s bandwidth charts.

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
- [ ] Dialogs are true in-pane overlays (see Known gaps) preserving background;
      `Esc` closes; full forms.
- [ ] Readline kill/yank with a global kill ring in every input.
- [ ] Mouse: click menu, list entries, links, pane gutters, expand gutters.
- [ ] UTF-8 clean; reflows at 80×24; no resize crash; 1 s intro splash.
- [ ] Terminal hardware cursor positioned on focused micron pages (`LinkableText`)
      and text inputs (`ReadlineEdit`) (capture-invisible — verified by a
      `calc_coords` golden test, not `parity.sh`).
- [ ] All pages functional (Conversations send/attach/trust; Browser fetches over
      RNS; Channels connect via RRC; Interfaces real enumeration + forms; Config
      editor; Log live tail; node hosting serves pages).
- [ ] Cross-process integration tests green (RNS page fetch, LXMF ingest, RRC
      HELLO/MSG).
- [ ] `go test ./...`, `go vet ./...`, `gofmt -l .` clean; every package has a
      godoc package comment in a same-named file.

---

## TASK LIST (ordered for success — work top to bottom)

> **Where to start:** The first available task. Each `[ ]` is one TDD unit. Remove
> it when green. Phases are prerequisites for the ones below them.

### Phase 0 — Foundation & cross-cutting gaps

- [ ] Mouse clicks have no correlation with what is under the cursor — fix mouse
      coordinate mapping so clicks hit the right menu/list/link/pane gutter and
	  eliminate off-by-one errors.

### Phase 2 — Browser page over RNS (Python `Browser.py`, 1848 lines)

> No `nomadnet/browser` core fetch backend exists — the browser is TUI-only with
> `displayURL` rendering a hardcoded placeholder. Build the fetch backend on the
> existing go-reticulum `rns.Link.Establish` + `Link.Request` + `Resource`
> primitives (the node side already serves pages via `node.Node.registerRequestHandlers`
> using `Destination.RegisterRequestHandler`). Capture expected behavior from
> `Browser.py`.

- [ ] Wire `browserDisplay.OnOpenRRC` (deferred to Phase 3 — needs the
      channels-display hub/room selection UI which Phase 3 builds): switch to
      the Channels page, find/connect the hub via `a.RRC.FindHub`/`AddHub`, and
      select the room. (All other browserDisplay callbacks — OnRetrieveURL,
      OnFetchPartial, OnJumpAnchor, OnPartialUpdate, OnOpenLXMF, OnBrowserError,
      OnToggleFullscreen, OnDisconnect, OnCopyURL, OnURLDialog, OnSaveNode —
      are wired; OnToggleFullscreen is a documented no-op because the Go
      browser is its own full-screen page, not an in-network-pane widget.)

### Phase 3 — Channels page / RRC (Python `Channels.py`, 2285 lines)

> `rrc.RRCManager`/`RRCHub` are fully functional at the core layer (real
> `rns.NewLink`/`rns.RecallIdentity`/`link.SendPacket`) but the TUI channels page
> is built with `nil` rooms and never bound to `a.RRC`. `a.RRC = rrc.NewManager(...)`
> exists; wire it. Capture expected behavior from `Channels.py`.

- [ ] `RoomWidget` — messages + users pane (`C-u` toggle removes/re-adds the
      users column) + editor; verify vs Python `RoomWidget`.
- [ ] `RoomWidget.sendMessage` (`C-d`) + `handleSlashCommand` + `leaveRoom`
      (`C-x`); `tab` nick-complete is wired to `TabComplete` (cycling tested).
      Verify each slash command vs `_handle_slash_command`.
- [ ] `updateMessages`/`appendMessage` + `_messageWidget` — message list; verify.
- [ ] `newHubDialog`/`confirmNewHubDialog`/`editHubDialog`/`joinRoomDialog` +
      `showUserInfo` — overlay dialogs; verify vs Python.
- [ ] `autoconnect` — `_maybe_autoconnect` on startup; verify.
- [ ] Wire all channel callbacks: `OnNewHub`→`RRCManager.AddHub`;
      `OnJoinRoom`→`RRCHub.JoinRoom`; `OnRemoveHub`→`RRCManager.RemoveHub`;
      `OnEditHub`→`RRCHub` display-name update; `OnConnect`/`OnDisconnect`→
      `RRCHub.Connect`/`Disconnect`; `OnToggleAutoReconnect`→`RRCHub.SetAutoReconnect`;
      plus `OnSendMessage`/`OnLeaveRoom`/`OnToggleCollapse`/`OnToggleChannelList`/
      `OnMemberClick` (declared `tui/channels.go`, currently never wired).
- [ ] `F8` join/part collapse (logic wired + golden-tested) and `C-y` toggle
      channel list (pane removes/re-adds) — visual verify via `parity.sh` for the
      Channels page.

### Phase 4 — Network page remaining (Python `Network.py`, 1974 lines)

> Left-pane list/LocalPeer/NetworkStats/AnnounceInfo/KnownNodeInfo layout is done
> and capture-verified. Remaining items:

### Phase 5 — Interfaces page remaining (Python `Interfaces.py`, 3214 lines)

> List/data wiring, partial-box clipping, first-item ○, and title centering are
> done and capture-verified. Remaining items:

- [ ] `ShowInterface` — detail with bandwidth charts, `h`/`v` horizontal/vertical
      by width, `tab`/`shift-tab` focus cycle; verify.
- [ ] `AddInterfaceView`/`EditInterfaceView` — per-type forms; verify vs Python.
      Wire `OnAddInterface`/`OnEditInterface`/`OnRemoveInterface` (currently
      `// TODO`): edit the RNS config file then `Reticulum.ReloadInterface`
      (go-reticulum provides it).
- [ ] `RNodeCalculator` + `InterfaceBandwidthChart` — field updates + sparkline;
      verify vs Python.
- [ ] `getPortInfo`/`getPortField` — serial port detection; verify.
- [ ] `parseInterfaceConfig`: handle `RNodeMultiInterface [[[ / ]]]` sub-interface
      expansion (Interfaces.py:2843-2856) — currently skipped.

### Phase 6 — App wiring, node hosting & cross-process integration

- [ ] **Background-thread marshaling.** RRC/message/announce callbacks must marshal
      UI updates to the tview main loop (the `watch_pipe` equivalent). Watch the
      QueueUpdateDraw-before-`Run` deadlock gotcha: at wire-time mutate UI
      primitives directly; only background goroutines (tickers) use
      QueueUpdateDraw.

### Phase 7 — Mouse & final polish

- [ ] Mouse parity: click list entries, links, pane gutters, expand gutters.
      Includes the browser "marked link" footer display (Python
      `Browser.marked_link_job`, Browser.py:181-204 — show "Link to <target>"
      in the footer when a link is focused/hovered; pure-logic `MarkedLinkTarget`
      already ported+golden-pinned, only the footer render remains).
- [ ] 135×32 recommendation message (small-terminal reflow verified: 80×24 no
      crash, two-column body survives, menu clips).
- [ ] **Dialog in-pane placement.** Python places every page dialog as
      `urwid.Overlay(dialog, bottom=self.listbox, align=CENTER, width=RELATIVE_100,
      valign=MIDDLE, height=PACK, left=2, right=2)` WITHIN the page’s LEFT pane;
      Go’s `DialogManager` centers dialogs on the FULL SCREEN via `centerDialog`
      + shared `tview.Pages` (Go’s New Conversation `┌` lands at col 15 vs Python
      col 2). Affects ALL page dialogs — broad change to DialogManager + every
      host page + focus-restore. Capture-verify each page after.
- [ ] Final: run `parity.sh` for every page + the New Conversation dialog;
      confirm each Go `summary.py` output matches the original’s.

---

## Known deferred gaps (lower priority; not blocking, but needed for 100%)

- **Interfaces 1-row sizing nuance:** Python sizes its BoxAdapter to
  `screen_rows - iface_row_offset` (constant `iface_row_offset = 4`,
  Interfaces.py:2837) with the 2-row header INSIDE the list → `items =
  screen_rows - 6` + 1 blank buffer row at 80×24; Go fills all 19 remaining rows
  so it shows 1 extra partial row. Matching needs a height cap coupled to
  menu/header/footer heights (fragile at other sizes).
- **tview Checkbox glyph:** Go’s `(X) label` vs urwid’s exact checkbox glyph (not
  capture-reachable; affects KnownNodeInfo checkboxes).
- **RNodeMultiInterface sub-interface expansion:** see Phase 5.

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
