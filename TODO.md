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

## OPEN (remaining work)

- **Adaptive UI coalescing under announce firehose:** design session for
  reducing tview/tcell full-screen redraw cost during public-relay announce
  storms. A throttle variant must be specified carefully — the trailing-edge
  debounce semantics of the existing UI change coalescer are load-bearing for
  4 tests.
- **urwidColumns.Draw cost:** profile and reduce the draw cost of the
  Columns widget (second-largest contributor to full-screen redraw time).

## Bugs found (6-node trusted-conversation mesh sweep, 2026-08-28)

Driven live over tmux on 6 gonomadnet sessions (1 local + 5 remote, same
version), setting up trusted bidirectional LXMF conversations between every
node pair. Recorded as found; reproduce steps included.

- **BUG-1 — Stacked non-modal dialogs overlap (Conversations page):** with the
  My-LXMF QR dialog open (Ctrl-p), pressing Ctrl-n opened the New Conversation
  dialog ON TOP of the QR dialog — both drew simultaneously and interpenetrated
  (New Conversation box drawn across the QR's left half; QR overlay still
  full-screen behind it). In Python, `show_my_qr` replaces the entire widget
  and is modal. Repro: Conversations list → Ctrl-p (QR opens) → Ctrl-n. Esc
  pops only the top dialog (QR first, then New Conversation second) — so the
  stack at least unwinds LIFO. Expected: either C-n no-ops while the QR dialog
  is up, or the QR dialog blocks input to the page.

- **BUG-2 — Conversations list selection resets to the top during background
  refreshes (busy node):** on a node under an announce firehose (~250
  announces/min), moving the Conversations list highlight with Down and then
  going idle drifts the highlight TOP WITHOUT ANY INPUT (verified with 5s and
  9s idle captures: highlight returns to row 0, right pane follows). Root
  cause: every conversation-set refresh calls `SetConversations` →
  `populateList()` → `tview.List.Clear()` + re-AddItem, which resets the
  list's current item to 0 (tui/conversations.go:889 `populateList`,
  tui/conversations.go:1315 `SetConversations`, wired at
  cmd/gonomadnet/textui.go:813). On quiet nodes this is invisible; on the
  busy local node keyboard navigation fights the refreshes (Down×6 then idle
  → back to MacMini even though PixelBook was highlighted). Fix: preserve
  the selected conversation's source hash across `populateList` and restore
  the current item after re-adding.
  STRENGTHENED REPRO (same session, ~20:40Z): navigating one row per
  keystroke with a capture after EVERY Down shows the highlight moving
  backward mid-walk (kMan_phone → Mac (row 0) → Linux-OMEN …) with zero user
  input — the refresh resets the model selection to 0 between keystrokes while
  the RENDER keeps showing the previous row highlighted. Worst case: the
  on-screen highlight sat on "Go port of NomadNet on PixelBook", Enter was
  pressed, and the conversation that actually OPENED was "Undefined" (model
  row ≠ highlighted row). So under the firehose the visual highlight is not
  trustworthy and Enter acts on a different entry than displayed.
  MOUSE REPRO (same session, ~20:50Z): with the render stable for two
  consecutive captures, an SGR-1006 mouse click directly on the rendered text
  of the PixelBook row opened the "Undefined" conversation instead, and a
  click one entry up opened "Davo" — while keyboard Enter (no movement) opened
  `<712ffbfdb82c7fe60d0c5fa163ad2955>`, an address whose conversation is not
  visible in the rendered list at all. So on the busy node the rendered list
  (rows, order, highlight) can all be out of sync with the underlying list
  model when clicked/activated, via mouse AND keyboard.

- **BUG-4 — Attach File dialog can end up focus-detached and un-dismissable
  from its own controls:** after input misroutes (Enter/Tab pressed with focus
  in the composer while a conversation page was open), the Attach File
  file-browsing dialog appeared spontaneously but afterwards neither Escape
  nor walking focus to its Done/Cancel buttons (Down to list bottom → Down →
  Enter on Done) had any effect — the dialog stayed on screen while all
  keystrokes fell through to the page underneath (list navigation, editor
  typing, editor bar updates). Live recovery required re-invoking Ctrl-f from
  the editor (which re-opens the dialog focused) and then Escape. Repro:
  Conversations → open conversation (editor focus) → press Enter/Tab bursts →
  Attach File dialog appears → press Escape. Expected: dialog modal; Esc
  always dismisses; keys never fall through while it is open.

- **BUG-5 — Conversation composer Backspace/C-a/C-k dead with (apparent)
  editor focus:** with a conversation open (fedor, untrusted) and the editor
  shortcut bar visible ([C-d] Send ...), stray text in the composer ("local",
  leaked from tmux key-driving) could NOT be cleared — Backspace ×8 and
  Ctrl-A/Ctrl-K had no visible effect, while the shortcut bar kept claiming
  editor focus. Ctrl-w (close conversation) cleared the draft. Repro: open a
  conversation, type stray text, press Backspace. Expected: the composer
  (readline editor) clears like on other nodes/sessions. Consistent with the
  focus-vs-bar mismatch class — bar stale or input capture not forwarded.
  NOTE: this happened on the busy local node under the announce firehose, so
  a stale-render confound is possible.

- **BUG-6 — New Conversation dialog "Create" silently does nothing on the
  remote nodes (penguin, glenn-nano2gb, glenn-OMEN-875):** C-n → type 32-hex
  addr → Tab×4 → Space (radio visibly flips to (X) Trusted) → Tab → Enter
  closes the dialog but creates NO directory entry: both tab counts stay
  Trusted (0) / Untrusted (0). Verified byte-for-byte via capture-pane -e
  (dialog closes; no new row in either list). Tried Enter AND Space on the
  presumed Create focus, and both a neighbor identity (712ffbd…) and 2a6105…
  on penguin — all no-ops. On the current (rebuilt-today) builds — local and
  glenn-mac-mini-m2 — the exact same recipe creates the entry (mac-mini:
  Trusted 0→1 and "671ed4…" appeared in the list). Suspects: stale binary on
  the remotes (see BUG-3/BUG-8) or a dialog-focus regression where the final
  Tab lands on Back and Enter dismisses. Workaround available on stale
  builds: none found yet — peer info dialog (C-e, which does open) may be
  the only trust editor.

- **BUG-7 — LXMF message delivery fails node-to-node (mac-mini → penguin,
  2026-08-28 20:11Z):** mac-mini created a trusted conversation for
  671ed4dc… (penguin), typed a message, C-d sent (composer shows "→ just
  now"), penguin's Conversations stayed Trusted (0)/Untrusted (0) for >90 s.
  LXMF delivery needs a path; penguin's LXMF identity has apparently not
  propagated to mac-mini's RNS. Not necessarily a port bug — flag as sweep
  blocker: without paths, bidirectional proof is impossible even when trust
  is set. Watch whether announce propagation fills in paths over minutes.

- **BUG-8 — Conversations page chrome chaos on OMEN (same family as BUG-3):
  page navigation lands on the wrong panel and the two-pane order flips
  WHILE USING THE PAGE.** Concrete sequence: on the Conversations page,
  pressing Up (expecting the tab bar) then Right/Enter repeatedly displayed a
  "Go port of NomadNet on <node>" list headed "Saved Nodes" next to a
  "Conversations ─" frame — i.e. the Saved Nodes / peer list panel and the
  Conversations list both fighting for the same region, with no conversation
  selected state persisting. Also: one Up-keypress from a non-top list row
  reached the MENUBAR, so Right+Enter activated [Network] unintentionally
  (and an earlier session hit [Quit] the same way, exiting the app). Up at
  top is consumed by the menubar in some focus states, so blind
  Up/Right/Enter bursts are dangerous. Evidence: capture dumps /tmp/mesh/o2.txt,
  /tmp/mesh/o3.txt.

- **BUG-3 — Conversations two-pane layout SWAPPED on glenn-nano2gb:** on the
  Jetson (232x50 pane, identical to the other 5 sessions), the Conversations
  page renders DETAIL placeholder LEFT and list RIGHT — the opposite of the
  other 5 nodes and of the source (`content.AddItem(leftPanel)` then
  `AddItem(cd.detail)`, tui/conversations.go:306-307). Persists across C-g
  fullscreen toggles and across page-away/page-back (Network→Conversations),
  so it is durable display state, not a stale frame. This matches the
  historically-fixed C2 "panes swapped until next conversation open
  self-healed" bug (see the setListColumn comment at
  tui/conversations.go:1005-1025), so the nano's running binary most likely
  predates that fix — verify the build on the Jetson (user believes all 6
  nodes run the same version). Suggest: rebuild + relaunch on nano2gb.
  UPDATE: glenn-OMEN-875 swapped the SAME WAY mid-sweep (after the
  New-Conversation dialog open/create + close-conversation sequence),
  self-healing only temporarily after some C-w presses. So this is not a
  stale binary on one node — it reproduces on current builds: the two-pane
  order flips under certain dialog interactions and only heals on some
  triggers.
  STRONGER REPRO on local-build glenn-mac-mini-m2 (~21:00Z): opening a
  conversation and closing it with C-w leaves the Conversations page in an
  edge-to-edge "No conversation selected" state with the LIST PANEL GONE
  (no tab bar, no list) — C-g no longer toggles anything while the page is
  in that state. Also reproducible transiently: the left panel collapses to
  blank (focus rectangle visible, no rows) while the tab bar and "Last sync"
  footer remain. A page away/back cycle (menubar → Network → menubar →
  Conversations) fully heals it and the list reappears. So list-pane
  collapse/loss is a live state corruption on current builds, not a stale
  binary.
  ROOT-CAUSE POINTER (code read): `cw.OnClose` (tui/conversations.go:677)
  does `cd.content.RemoveItem(cd.content.GetItem(1))` — it removes whatever
  is at content index 1, which is the conversation widget ONLY while the
  two columns are still in the built order [leftPanel, right]. If the pane
  order has been flipped (BUG-3's swap) or a detail/slot overlay shifted the
  indices, index 1 is the corrected right pane or worse, and the ONLY copy
  of the left list panel can be removed from the Flex entirely. Nothing ever
  re-AddItems `leftPanel` to `content` after construction (grep: only line
  306), so once removed the page cannot recover by navigating away/back or
  by C-g (the collapsed panel keeps a focus rectangle, but
  `cd.handleInput`'s list-region gate short-circuits so C-g/C-n never fire).
  Observed end state on glenn-mac-mini-m2: full-width "No conversation
  selected" pane, no tab bar/list/footer, unresponsive to C-g/C-n/page
  cycles — requires restarting the app.
