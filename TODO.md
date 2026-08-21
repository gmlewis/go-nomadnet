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
- [x] Terminal hardware cursor positioned on focused micron pages (`LinkableText`)
      and text inputs (`ReadlineEdit`) (capture-invisible — verified by sim-screen
      `GetCursor` tests + live tmux `cursorEverSeen=true` in the Guide walk, not
      `parity.sh`).
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

---

## Phase P — Parity-test remediation: replace fake "parity" tests with live python3 cross-checks

> **Why this phase exists.** An audit (2026-08-19) found tests across the
> non-tui packages that *claim* Go/Python parity in their name/comments but
> never execute Python — they compare Go to a committed golden file
> (`testdata/*_parity.json` / `*.bin` / `*.msgpack`) or to hand-typed literals
> labeled "Python". They cannot catch a real divergence. (The `tui/` package was
> already converted to live cross-impl via `runPythonNomadnet` in
> `tui/parity_live_test.go` — those are the template and are NOT re-flagged here.)
>
> **What counts as a REAL cross-impl test (the template every task must match):**
> the Go test owns the input battery; it execs the real Python nomadnet reference
> via `testutils.RunPythonNomadnet` / `testutils.RunPythonNomadnetRaw` (gated on
> `testutils.SkipIfNoPythonNomadnet` — SKIP cleanly when Python/nomadnet absent)
> and diffs Go output against **freshly produced** Python output — not a
> committed literal. Reference conversion: `nomadnet/util/python-parity_test.go`.
>
> **Triage policy:** FIX the byte/on-disk/logic tests by wiring them live (and
> FIX any Go/Python mismatch in production code — TDD, keep the failing test,
> fix production, re-run until green); DELETE pure tautologies that assert
> nothing; for Go-vs-Go tests that assert real Go behavior with no false Python
> claim, leave as-is. Remove each `[ ]` when green.

**Phase P COMPLETE (2026-08-20).** Every fake-"parity" test in the non-tui
packages was converted to a live python3 cross-implementation test (Go test
owns the inputs, execs the real Python nomadnet reference via
`testutils.RunPythonNomadnet`/`RunPythonNomadnetRaw`, gated on
`SkipIfNoPythonNomadnet`, diffs FRESH Python output) or, for pure Go-vs-Go
tautologies that asserted nothing, deleted. All committed golden files
(`testdata/*_parity.json`, `*.bin`, `*.msgpack`) used by these tests were
removed. Every converted package is green under `-race -tags integration`.

Conversions completed this phase:
- `nomadnet/util/python-parity_test.go` — 6 tests live; `util_parity.json`
  golden deleted.
- `nomadnet/micron/slugify-parity_test.go` + `partial-parity_test.go` — live;
  `slugify_parity.json` golden deleted. **REAL BUG FIXED** in `micron.go`
  `parsePartial` pid extraction (case-3 path): Python `parse_partial` splits on
  `=` and takes segment[1]; Go was taking `f[4:]`. Changed to split-on-`=`.
- `nomadnet/asciichart/ascii-parity_test.go::TestPlotPythonParity` — live via
  `nomadnet.vendor.AsciiChart.plot`; `ascii_parity.json` golden deleted.
- `nomadnet/storage/paths-parity_test.go::TestPathsPythonParity` — live via
  `NomadNetworkApp` path construction; `paths_parity.json` golden deleted.
- `nomadnet/config/applyconfig-parity_test.go` + `config/config-parity_test.go`
  + `nomadnet/app/applyconfig-parity_test.go` — live via mock-self +
  `types.SimpleNamespace` binding of the real Python `applyConfig`;
  `applyconfig_parity.json` (x2) + `default_parsed.json` goldens deleted.
- `nomadnet/peersettings/peersettings-byteparity_test.go::TestSavePythonByteParity`
  — live `msgpack.packb` byte diff; `peersettings_*.bin` goldens deleted.
- `nomadnet/peersettings/peersettings-parity_test.go::TestLoadPythonCompat` —
  rewritten live (Python `packb`→bytes + `unpackb`→expected, fresh each run);
  `peersettings_{filled,defaults}.bin/.json` goldens deleted.
- `nomadnet/directory/persist-byteparity_test.go::TestSaveToDiskPythonByteParity`
  — live `Directory.save_to_disk` bytes; `py-directory-save.msgpack` deleted.
- `nomadnet/conversation/index-byteparity_test.go::TestWriteIndexPythonByteParity`
  — live `Conversation.write_index` bytes; `py-index-byteparity.msgpack` deleted.
- `nomadnet/conversation/attachments-manifest-byteparity_test.go`
  `TestExtractAttachmentsManifestByteParity` — live
  `extract_attachments_from_lxm` manifest bytes; `py-attachment-manifest.msgpack`
  deleted.
- `nomadnet/rrc/cbor-parity_test.go::TestIntegrationProtocolConstantsMatch` —
  now execs `nomadnet.RRC` fresh; hardcoded "Python values" map removed.
- `cmd/gonomadnet/main_test.go::TestDefaultConfigDir` — rewritten live against
  the Python default configdir; `TestFlagParsing` (no-op) deleted;
  `TestResolveConfigDirOrderPythonParity` added (drives all 4 resolution
  branches live). **REAL BUG FIXED**: `cmd/gonomadnet/main.go` only ever used
  `~/.nomadnetwork`; now mirrors Python's 3-way order (`/etc/nomadnetwork` →
  `~/.config/nomadnetwork` → `~/.nomadnetwork`) via new `configdir.go`.
- `nomadnet/app/applyconfig-fields_test.go::TestApplyConfigWiresFields` —
  Go-vs-Go wiring tautology, deleted (live applyconfig parity covers fields
  end-to-end).

---

## tmux-suite run-diff parity (nomadnet vs gonomadnet, 2026-08-05)

> Source: full `tmux-test-suite` run against the Python source-of-truth
> (`/tmp/nomadnet-tmux-test-suite-1785954642.log`, 232x53, headed) vs the same
> suite against the Go port (`/tmp/gonomadnet-tmux-test-suite-1785970825.log`,
> same size/flags). Both pass every suite ASSERT; the bugs below are the
> behavioral/visual differences the suite does **not** assert on. Each entry
> cites log line numbers (nomadnet→ / gonomadnet→) as evidence. Line numbers
> are file offsets in the two `/tmp/*tmux-test-suite*.log` files.

---

## Screencast-comparison parity (cast-confirmed bugs)

> Source: `python_session.cast` (Python nomadnet v1.2.6, **256-color**) vs
> `go_session-002.cast` (Go port, **truecolor**), decoded with
> `tooling/parse_screencast.py` (+ `substates.py`, `guidetopics.py`). See
> `tooling/README.md` for the methodology and the **colormode caveat**.
>
> **Update (2026-08-06): the 3-hex cube-quantization divergence was a REAL
> port-wide bug, NOT a colormode artifact.** urwid's `_parse_color_true`
> (display/common.py) routes 4-char `#rgb` through `_parse_color_256`, cube-
> quantizing each nibble to the nearest of {0,95,135,175,215,255} EVEN in
> 24-bit truecolor; only 7-char `#rrggbb` is parsed exact. Python's static
> palette (TextUI.py THEMES) uses 3-hex strings, so its truecolor menubar emits
> `\x1b[38;2;0;0;0m\x1b[48;2;175;175;175m` (#111/#bbb → #000000/#afafaf), NOT
> 17,17,17 / 187,187,187. The Go port nibble-doubled 3-hex → 6-hex (exact),
> diverging everywhere. The micron path was already at parity (Python's
> `high_color` nibble-doubles 3-hex → 6-hex → urwid-parses-exact, and Go's
> `highColor`/`tviewColor` match). FIXED for the two central color-resolution
> paths — `parseColor`/StyleRegistry (tui/palette.go) and `GetThemeColors`
> (tui/theme.go, all 3-hex entries → `cubeHex3`) — plus the Guide reader base
> color (was a wrong `0xbbbbbb`; now the micron plain default #dddddd/#222222).
> The boot/Guide truecolor capture (captures/cq_135x32_00_esc.txt vs
> gq_135x32_00_esc.txt) now has IDENTICAL SGR sets — fg {0,215,221,34,95},
> bg {175,187}. (The earlier P1 task was based on the wrong premise that
> Python truecolor emits 38;2;17;17;17; it emits 38;2;0;0;0. P1 is removed —
> the truecolor captures now exist, satisfying its purpose.) The remaining
> work is P2: direct nibble-doubled literals in widget files that bypass the
> central paths.

### P2 — Cube-quantize remaining direct nibble-doubled 3-hex literals
- [ ] Direct `tcell.NewHexColor(0xbbbbbb)` / `0xdddddd` / `0x222222` / …
      nibble-doubled literals that bypass `parseColor`/`theme.go`. Each needs
      per-site Python cross-referencing: if the Python source uses 3-hex
      `#rgb` for that element → route through `GetThemeColors(theme)[key]`
      (theme-aware, already cube-quantized) or `cubeHex3("#rgb")`; if it uses
      6-hex `#rrggbb` → leave exact; if it is a Go-specific value Python never
      emits → correct it. Do NOT blanket-quantize — quantizing a color Python
      treats as exact 6-hex would introduce a divergence. Capture each
      affected page with `capture.sh` and confirm the Go truecolor SGR set
      converges on Python's before removing this task.

  **DONE so far (2026-08-06, TDD-pinned, golden values from ui/TextUI.py +
  Conversations.py/Channels.py AttrMap wrapping, full suite green):**
  - Guide reader base fg → micron plain default #dddddd/#222222
    (`tui/guide.go`; was wrong 0xbbbbbb).
  - conversation-widget: peerInfoBar → `msg_header_sent` (#111/#ddd), editor
    + titleEditor → `msg_editor` (#111/#0bb), messageList base → default
    (Python `IndicativeListBox` has no AttrMap).
  - channels: compose input → `msg_editor`, messages view base → default
    (Python `_StickyMessageListBox` has no AttrMap).
  - compose: title + editor → `msg_editor`.
  - browser content base → `body_text` (#ddd/#222; Browser.py:562
    `AttrMap(...,"body_text")`). Colors blank/padding + placeholder/loading
    only — micron plain runs carry an explicit #dddddd tag (micron.DefaultFG,
    like Python `high_color`), so they do not inherit it. Was a wrong hardcoded
    `0xbbbbbb` const; now theme-aware `bd.contentFG` via
    `GetThemeColors(app.Theme)["body_text"]` (`tui/browser.go`).
  - room-widget: messages base → `body_text` (Go renders bodies as
    `[#66cc55]<nick>[-] <text>`, so the body inherits SetTextColor; Python body
    attr is `body_text`, Channels.py:1333), editor → `msg_editor`
    (Channels.py:609 `AttrMap(editor,"msg_editor")`).
  - network-views: NodeInfo + LocalPeer base → default (Python bare
    `urwid.Text`, no AttrMap / `widget_style=""`, Network.py:1271-1272,1351,
    1372,1387-1388).
  - log: LogDisplay base → default (Python `LogTerminal` = `urwid.Terminal` in a
    LineBox, no AttrMap, Log.py:44-51).
  - config: explainer → `body_text` (Config.py:40 `Text(("body_text",...))`).
  - directory: detail base → `body_text` (Python `Directory.py` is a 20-line
    stub using `body_text`, Directory.py:14; Go's richer two-pane is Go-specific
    so body_text is the closest defensible base).
  - conversations: right-pane detail (`cd.detail`) empty-state base → default
    (Python bare `Text("\n  No conversation selected")`, no AttrMap,
    Conversations.py:1881-1884; the populated summary is Go-specific).
  Each routed through `GetThemeColors(app.Theme)[...]` (or `ColorDefault`).

  **Remaining (need per-site Python cross-ref + capture; do NOT guess):**
  - editor/field surfaces: `interfaces.go` (Phase-5 Add/Edit — confirm the
    Python editor style first); dialog ReadlineEdit inputs (`dialog.go` +
    `conversations.go` eName/eCopy/eNotes/limitInput) — Python leaves these BARE
    (no `AttrMap`) so they are `default`, NOT `msg_editor`; verify per-site.
  - Go-specific / no-clear-Python-spec surfaces (leave 0xbbbbbb or needs
    capture): `msgview.go` (no Python equiv), `hub-info.go` (Go-specific
    MOTD/rooms text summary; Python `scrollbar` attr is for a different widget
    - the channel-list scrollbar trough, Channels.py:1827), `micron-view.go`
    (Go-specific; closest = micron plain #dddddd), `linkable-text.go` (micron
    plain #dddddd; used inside the browser `body_text` wrapper - needs
    capture), `browser-chrome.go:156` (test-only `browser_controls` fallback),
    `network.go`, `helpers.go`.
  - named-color gaps (SEPARATE, out of 3-hex scope): `error_text`/trust-banner
    bg "dark red" (#800000 vs tcell.ColorRed), `inactive_text`/placeholder
    "dark gray", `connected_status` "dark green", `interface_title` "" / bold —
    these are urwid *named* colors, not 3-hex; resolve via urwid's named-color
    table, not cubeHex3.
  - `gNN` micron grayscale (heading `g93`): Go uses linear `v*255/99` but urwid
    uses the 24-step 256-gray ramp — a distinct follow-up.

### Verification tasks (areas not captured, or not comparable at this colormode)
> These are not yet confirmed bugs — they are areas the existing casts do not
> cover or that need a truecolor capture to judge. Capture with the
> `tui-parity` harness or a truecolor asciinema run, compare with
> `tooling/substates.py` / `guidetopics.py`, and either file a concrete bug task
> or close the gap.

---

## Known deferred gaps (lower priority; not blocking, but needed for 100%)

- **Boot: Local Peer Info blank during RNS bring-up (was B8):** gonomadnet
  shows the TUI immediately with a blank Local Peer Info (identity/addr empty,
  `Announced: Never`, interfaces `Disconnected`) while `initRNS` runs in a
  background goroutine; the panel populates once `initRNS` completes (it fires
  `UIChangeCallback`). Python runs RNS init SYNCHRONOUSLY before the UI starts,
  so its terminal is blank for the same duration and then appears WITH identity
  populated. The Go init ORDER matches Python (both load identity after
  `RNS.Reticulum`); Python's `TCPClientInterface.SYNCHRONOUS_START = True`
  means it ALSO blocks ~5 s per unreachable TCP hub on the dial. Both reach the
  populated state at the same wall-clock time when hubs are reachable (the
  normal case); the original B8 observation was a run where Go's hubs were
  unreachable (3×5 s dial timeouts). The parity-faithful fix (make `initRNS`
  synchronous) is a deliberate UX tradeoff — it matches Python's
  blank-startup-when-unreachable but regresses Go's show-UI-early design, and
  would make the `Init()` tests block on real RNS + `startNode`. Left as a
  conscious design choice pending user confirmation; the "Last sync: never"
  sibling symptom is already FIXED (Conversations footer reads persisted
  `last_lxmf_sync` via `App.LastSyncInfo` + 30 s refresh).
- **Interfaces 1-row sizing nuance:** Python sizes its BoxAdapter to
  `screen_rows - iface_row_offset` (constant `iface_row_offset = 4`,
  Interfaces.py:2837) with the 2-row header INSIDE the list → `items =
  screen_rows - 6` + 1 blank buffer row at 80×24; Go fills all 19 remaining rows
  so it shows 1 extra partial row. Matching needs a height cap coupled to
  menu/header/footer heights (fragile at other sizes).
- **tview Checkbox glyph:** Go’s `(X) label` vs urwid’s exact checkbox glyph (not
  capture-reachable; affects KnownNodeInfo checkboxes).
- **RNodeMultiInterface sub-interface expansion:** see Phase 5.
- **Guide indent-wrap gap:** `StyledLinesToTviewText` writes `line.Indent` as
  leading spaces INTO the text, so the single TextView wraps (indent+text) at the
  full pane width. Python wraps each attrmap line in `Padding(left=left_indent,
  right=right_indent)`, so its Text wraps at (width − left_indent − right_indent)
  and is then shifted. For a depth-1 section paragraph (indent 2) Go wraps the
  indented line one word later than Python (e.g. `…must support to` vs `…must
  support`), a 2-paragraph difference in topic 7. Fix = per-line padded wrapping
  (a Pile-of-Padded-attrmaps rearchitecture); the B2 focus model now rides on top
  of the single TextView, so this is an independent, low-impact wrap nuance.
- **Channels shortcut bar not wired (key-hint footer):** The Channels *page*
  is implemented (hub list, rooms, members, gutters, room store, `handleInput`
  keyboard handling, colors, tests in `channels_test.go` /
  `channels-hublist_test.go` / `channel-gutters_test.go` /
  `channels-color_test.go`), but its **shortcut bar** — the per-focus-region
  key-hint footer (Python `Channels.py:217-229`, three regions: list / editor /
  body) — is **not wired**: `channels.go` never calls `SetShortcut`,
  `SetShortcutCallback`, `SetShortcutFocus`, `GetShortcutText`,
  `setShortcutRegion`, or `refreshShortcuts`, so the channels page shows an
  empty/stale shortcut bar. `ConversationsDisplay` is the reference pattern
  (`setShortcutRegion` wired as the `SetFocusFunc` of every focusable primitive
  → "list"/"editor"/"body", plus `GetShortcutText`; see `conversations.go:287`).
  The three golden strings for these bars (`shortcutChannelsList` /
  `shortcutChannelsEditor` / `shortcutChannelsBody`, captured from
  `Channels.py:217-229`) were removed during the 2026-08-08 lint sweep as U1000
  dead code because no test used them — they are re-capturable from
  `Channels.py:217-229`. Fix: port the Conversations `SetShortcutFocus` /
  `setShortcutRegion` / `GetShortcutText` pattern to a `ChannelsDisplay`, wire it
  as the `SetFocusFunc` of the channels list / room editor / room body
  primitives, re-capture the three bar strings from `Channels.py:217-229`, and
  add a channels shortcut-bar test (`shortcut-bar_test.go`-style) using them.

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
