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

## UI keyboard/dialog parity bugs (live tmux A/B sweep, 2026-08-27) — KEEP-GREEN RE-CHECKS ONLY

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

### D. Network page

- **D4 (LV, keep green)** Announce Stream (tabs, Search field, Show toggle), C-l
  toggle, saved-nodes list, Connect-to-node dialog (Yes/No/Info), browser
  chrome + page render + Down cursor traversal: AT PARITY — keep green.

### E. Guide page

- **E3 (LV, keep green)** Topic content Down on plain (unlinked) text is a no-op on BOTH
  — parity, keep green. Topic list content itself matches exactly.

### G. Interfaces page

- **G3 (LV, keep green)** Interface list: Up/Down selection, ●/○ markers, shortcut bar
  `[C-a] Add [C-e] Edit [C-x] Remove [Enter] Show [C-w] Open Text Editor`,
  and the empty-state/config pages (Config page = exact parity): keep green.
  Note: interface list ORDER follows each host's own `~/.reticulum/config` —
  not a bug.

### H. Menubar (verify-then-fix)

- **H2 (LV, keep green)** Menubar Left/Right movement, Enter activation, focus RETENTION
  on the activated button across pages, and Tab/Down → body: at parity.
  Verified: identical page-advance behavior on both targets.

---


## New findings (2026-08-28, from A-class verification captures)

- Empty-state placeholder highlight: on the Conversations empty list Python
  paints the centered "No trusted/untrusted conversations" row with the
  list_focus background while the list is focused (live capture
  tooling/tui-parity/captures/pyaconv_100x28_00_esc.txt row 4, bg #878787);
  the Go port renders the placeholder without the focus highlight (see
  IndicativeListBox.SetEmptyText drawing path).
- Conversations list shortcut bar wrap at 100 cols: Go's wrapped second line
  ends "My LXMF " with one trailing space; Python ends "My LXMF". Cosmetic
  one-space diff in urwidSpaceWrap output.

## OPEN (updated 2026-08-26 late): nano slowness + remaining delivery gaps

- pprof verdict on glenn-nano2gb: NO busy loop active (37 goroutines stable,
  0 cs CPU in sampled windows). Cost splits ~50% tview/tcell full-screen
  redraws under public-relay announce firehose + `maintenance` self-time
  spike attributed to serial pathTable persist on slow eMMC. Shipped fix:
  async single-flight persistPathTable/flushKnownDestinations inside the
  maintenance loop. Follow-up candidates: adaptive UI coalescing design
  session (trailing-edge debounce semantics are load-bearing for 4 tests;
  a throttle variant needs careful spec), urwidColumns.Draw cost.
