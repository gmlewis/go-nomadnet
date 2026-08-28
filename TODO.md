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
- **Reference behavior + captures:** `tooling/parity-reference/`
  (`nomadnet-trusted-chat-reference.md`, `gonomadnet-0.22.0-findings.md`, and
  `captures/` — nomadnet 1.2.8 reference grabs `00`–`46` + `g40`–`g42` for the
  gonomadnet run). Diff gonomadnet against these.

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

## UI keyboard/dialog parity bugs (live tmux A/B sweep, 2026-08-27)

Found by driving the REAL instances with identical tmux keystrokes and diffing
SGR captures (`ansiview.py --json`) plus the Go focus diagnostics
(`/tmp/quit-diag.log` on the gonomadnet host). Targets: Python `nomadnet`
(local tmux `local`) vs `gonomadnet` (tmux `glenn-mac-mini-m2`), both 232x50.
"LV" = live-verified on both instances; "SRC" = derived from the Python source
path that Go must replicate. Fix TDD-style, one item per session.

**ACCEPTED ENHANCEMENT (owner decision 2026-08-27 — do NOT "fix"):** closing
dialogs/overlays with **Esc** in addition to Python's dismissal buttons is an
intentional Go-port enhancement (natural muscle memory from other tools). It
STAYS. Parity work must preserve Esc-to-close AND additionally make Python's
button-based dismissal paths and their keyboard traversal exist and work.

### A. Stranded-focus class (the "completely unusable" arrows)

tview has no urwid-style key bubbling, so every Up-at-top/Left/Right escape
Python's widgets return up the widget chain must be re-implemented explicitly.
Today several panes swallow arrows and strand the keyboard.

- **A1 (LV)** Up from the message editor must return focus to the message
  list. Python: `MessageEdit.keypress` "up" at cursor y==0 →
  `frame.focus_position = "body"` (Conversations.py:1816-1825). Go: Up in the
  ReadlineEdit is a dead key — the shortcut bar stays on the editor set and
  focus never leaves `*tui.ReadlineEdit`.
- **A2 (LV)** Left/Right inside the conversation pane must switch column
  focus (list ↔ conversation widget), as urwid Columns does. Go: both dead in
  the conversation pane; the list pane is unreachable by arrows (Tab still
  toggles editor/body — that part is at parity).
- **A3 (LV)** Up at the top of the message list must collapse focus to the
  header menubar — or to the TRUST BANNER first when one is visible
  (ConversationFrame.keypress + `has_visible_trust_banner`,
  Conversations.py:1845-1870). Go: `messageListView` is unknown to
  `MainDisplay.bodyListAtTop` → dead key.
- **A4 (LV)** The message list must be a selectable per-message ListBox:
  Up/Down move the focus highlight message-by-message with autoscroll (Python
  `LXMessageWidget` selectable inside IndicativeListBox). Go renders one flat
  TextView with NO per-message selection at all — arrows can only scroll a
  text blob, and no message can ever be "focused".
- **A5 (LV)** Down at the LAST conversation-list entry must move focus into
  the conversation (right) column (urwid Columns traversal, Python
  ConversationsArea.keypress falls through to Columns). Go: Down is a no-op
  at the bottom; focus stays on the list.
- **A6 (SRC+LV)** Up from the TOP of the conversation list must land on the
  TAB BAR first — the `[ Trusted (N) ] [ Untrusted (N) ]` TabButtons (and the
  `Show blocked (N)` checkbox on the untrusted tab) are keyboard-focusable:
  Left/Right move between the two tabs, Enter switches tab; only another Up
  reaches the menubar. Live-verified on Python (Right+Enter switched to the
  Untrusted tab). Go: the tab bar is NOT keyboard-reachable at all — Up-at-top
  jumps straight to the menubar (`bodyListAtTop` → `FocusMenu`).
- **A7 (SRC)** Trust banner arrow path: when an unknown/untrusted peer is
  open, Up at the messagelist top must focus the banner's Trust/Block buttons
  before the menubar (Python `_header_pile.focus_position = 1`,
  Conversations.py:1854-1862). Go: implement/verify (not live-verifiable on
  the test peers — banner suppressed).

### B. Conversation view rendering

- **B1 (LV)** Message widget order is INVERTED: Python renders the header
  line first (`✓/✕ <arrow> <relative time> | <absolute time><encryption
  glyph>`), then the content lines. Go renders a content-ish line (`msg3:
  from Mac Mini`) ABOVE the header line. Fix order + header format.
- **B2 (LV)** Minimal editor must be an INVISIBLE empty one-line footer (no
  placeholder; Python wraps `MessageEdit` in `AttrMap(..., "msg_editor")`,
  Conversations.py:1916). Go paints a visible `Type a message... (Ctrl-D to
  send)` placeholder row.

### C. Conversation-list dialogs (list-slot overlays)

- **C1 (LV, ACCEPTED-ENH)** Esc-to-close on dialogs is an accepted Go
  enhancement — KEEP it (guardrail above). Remaining parity work: Python's
  dismissal BUTTONS must exist and work for every list-slot dialog —
  `< Back >` (Peer Info, New Conversation, Ingest URI), `< Yes/No/Info >`
  (Connect), `< Sync Now > < Close >` (Message Sync), `< Cancel >` (URL),
  `< Create/Back >` (New Conversation) — including keyboard traversal to
  REACH them (Down/Tab through the dialog's widgets, Enter activates the
  focused button) exactly like Python's Pile focus chain, and mouse clicks.
  Verify per dialog: all buttons present, reachable by keyboard, and
  performing the same action Python's button performs.
- **C2 (LV)** Dialog placement: Python replaces the list column IN PLACE
  (`columns_widget.contents[0] = overlay`, Conversations.py:1024-1029), so
  the dialog sits in the LEFT list slot and the open conversation stays
  visible on the right. Go appends the SlotOverlay to the END of the content
  Flex (`ShowListSlotDialog`), throwing the layout to
  [detail | list+dialog]; `CloseListSlotDialog` then appends `leftPanel` at
  the end again, leaving the panes SWAPPED until the next conversation open
  self-heals it. Replace/restore at column index 0.
- **C3 (LV)** Peer Info dialog: `Pin to top` must render as a checkbox
  (`[ ] Pin to top`); Go paints it as plain text. (Also keep the
  identity-unknown section: warning text + `< Query network for keys >` —
  Python only when the peer's keys are unknown; Go has the conditional but
  the live peer was known, so re-verify against an unknown peer.)
- **C4 (LV)** Ingest URI dialog strings: title `Ingest message URI` (Go:
  `Ingest LXM URI`), field `URI : ` (Go: `URI:`), buttons
  `< Ingest > < Back >` (Go: `< Save > < Cancel >`).
- **C5 (LV)** My LXMF QR dialog: Python = wide (~162-col) WHOLE-SCREEN
  centered overlay: QR + `< <lxmf-hash> >` + `< Close    >`. Go = 52-col slot
  dialog and the `< Close >` button is MISSING entirely.
- **C6 (LV)** Message Sync dialog: Python title `Message Sync`, propagation
  node line `⟳ (no default)` (glyph + label), centered status row, then
  `< Sync Now > < Close >`. Go title `Sync` with extra rows Go invented
  (`Download mode:` radio, `Messages: N`, `[0%]`) and no glyph row. Match
  Python's layout/strings/row order.
- **C7 (LV)** New Conversation dialog: content is at parity (including the
  double-`(X)` radio quirk) — only the C1/C2 placement/Esc issues apply.

### D. Network page

- **D1 (LV)** Enter on an announce in the Announce Stream must open the
  **Announce Info** overlay (Time/Addr/Type/Name/Oprtr/Trust rows + Announce
  Data + `< Back > < Connect > < Msg Op > < Save >`; Python Network.py
  announce-info). Go: Enter is a NO-OP on the announce stream (focus diag:
  key=13 swallowed by `*tui.pileFiller`).
- **D2 (LV)** C-u URL dialog must be PRE-FILLED with the current page URL
  (Python shows `URL : <hash>:/page/index.mu`); Go opens it empty.
- **D3 (LV)** After Esc hides the browser (nodes list returns), keyboard
  focus must go to the saved-nodes list; Go strands it on
  `*tui.browserPageView` (every arrow dead until Tab). Same stranding when
  the Network page is re-entered from the menubar with a browser still open —
  Go drops focus into the page view; Python keeps list focus.
- **D4 (LV)** Announce Stream (tabs, Search field, Show toggle), C-l
  toggle, saved-nodes list, Connect-to-node dialog (Yes/No/Info), browser
  chrome + page render + Down cursor traversal: AT PARITY — keep green.

### E. Guide page

- **E1 (LV)** After opening a topic, the topics list must stay arrow-
  navigable. Go strands focus on the reader's ScrollBar wrapper (Esc is a
  visual no-op; Up/Down swallowed until Left/Tab recovers). Python: topics
  list and reader are live Columns columns; Left/Right switch between them;
  Up/Down always work on whichever is focused.
- **E2 (SRC)** Up on the FIRST topic must collapse focus to the menubar
  (`TopicList.keypress`, Guide.py:191-195). Verify after E1 (currently
  unreachable).
- **E3 (LV)** Topic content Down on plain (unlinked) text is a no-op on BOTH
  — parity, keep green. Topic list content itself matches exactly.

### F. Log page

- **F1 (SRC)** Python intercepts the FIRST Up and returns focus to the
  header (`LogTerminal.keypress` "up" → header, Log.py:58-61; the embedded
  `tail -fn50` terminal never scrolls). Go instead scrolls its TextView log
  and only collapses to the menu once scrolled to the very top
  (`logAtTop`). Match Python: first Up → menu (log content stays).

### G. Interfaces page

- **G1 (LV)** Enter on an interface must open Python's FULL-PAGE
  InterfaceShow view: `===` header, Connection/Radio/Network/IFAC parameter
  blocks, RX/TX traffic charts (asciichart), footer `< Back >`/Toggle row.
  Go shows a tiny `Interface: <name>` overlay with only Type/Status/Target.
- **G2 (LV, ACCEPTED-ENH)** Esc closing the interface detail overlay is an
  accepted Go enhancement — KEEP it. Remaining parity work (after G1 makes
  the detail a real full-page view): implement Python's Tab → footer button
  row focus move (Interfaces.py:2644-2660) and the `< Back >` button /
  `switch_to_list` (Interfaces.py:2819, 3011) so dismissal also works by
  button navigation, not only Esc.
- **G3 (LV)** Interface list: Up/Down selection, ●/○ markers, shortcut bar
  `[C-a] Add [C-e] Edit [C-x] Remove [Enter] Show [C-w] Open Text Editor`,
  and the empty-state/config pages (Config page = exact parity): keep green.
  Note: interface list ORDER follows each host's own `~/.reticulum/config` —
  not a bug.

### H. Menubar (verify-then-fix)

- **H1 (SRC)** Go's menu Left at the leftmost button WRAPS to Quit
  (`handleMenuInput` `prev < 0 → n-1`); Python's urwid Columns does not wrap
  (Left at the edge stays). Verify Python live (use the Right×N+Enter probe —
  NEVER blind-Enter near Quit), then remove Go's wrap.
- **H2 (LV)** Menubar Left/Right movement, Enter activation, focus RETENTION
  on the activated button across pages, and Tab/Down → body: at parity.
  Verified: identical page-advance behavior on both targets.

### I. Channels page

- **I1 (LV)** Shortcut bar matches exactly (`[C-n] New Hub [C-a] Add Room
  [C-r] Connect [C-w] Disconnect [C-t] Auto-reconnect [C-e] Edit Hub
  [C-x] Remove`). Keep green.
- **I2 (LV)** Empty-state rendering: Go wraps `No hubs yet. Press Ctrl-N to
  add one.` mid-phrase across lines with no padding. Compare against
  Python's hubless Channels empty state (run Python with an RRC-hubless
  config) and match wording/wrap.

### J. Cross-cutting

- **J1** Every stranded-focus fix (A1-A5, D3, E1) needs a tview-level focus
  escape test: assert that after each key the focused primitive matches the
  Python focus path for the same state.
- **J2 (LV, note)** Python mouse clicks work via SGR-1006 injection; gonomadnet
  mouse is still broken (known) — keyboard-only paths above are the parity
  contract; mouse parity is tracked separately by the parity skill.

---

## OPEN (updated 2026-08-26 late): nano slowness + remaining delivery gaps

- pprof verdict on glenn-nano2gb: NO busy loop active (37 goroutines stable,
  0 cs CPU in sampled windows). Cost splits ~50% tview/tcell full-screen
  redraws under public-relay announce firehose + `maintenance` self-time
  spike attributed to serial pathTable persist on slow eMMC. Shipped fix:
  async single-flight persistPathTable/flushKnownDestinations inside the
  maintenance loop. Follow-up candidates: adaptive UI coalescing design
  session (trailing-edge debounce semantics are load-bearing for 4 tests;
  a throttle variant needs careful spec), urwidColumns.Draw cost.
