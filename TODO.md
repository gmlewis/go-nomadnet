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
  `captures/`). Diff gonomadnet against these.

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

---

## Outstanding items

- **Deploy the 2026-09-03 evening Channels fixes to the fleet.** The four
  fixes (the channels-list selection follows the shown room + the multi-hub
  info-panel lookup, the /who-reply member-set replacement + the 60 s silent
  membership reconciliation, the pinned all-rooms greeting MOTD, and the
  Users-pane no-selection-highlight under the "N users" count —
  `tui/room-widget.go`, pinned by `tui/users-pane-selection_test.go`) are in
  this working tree, uncommitted — the agent never commits or pushes. Commit
  + push go-nomadnet, then rebuild and restart `./gonomadnet.sh` on the five
  Go nodes (`local`, `glenn-OMEN-875`, `glenn-nano2gb`, `penguin`,
  `raspberrypi`); `glenn-mac-mini-m2` runs the Python SOT and is untouched.
  Expect after reconnect: every node shows the same live member count
  (reconciles within ~60 s) with NO highlighted row under "N users", the
  opened room's row carries the selection highlight, and every room shows
  the hub's greeting MOTD.