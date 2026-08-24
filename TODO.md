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

## Parity bugs found: nomadnet 1.2.8 (Python reference) vs gonomadnet

Discovered by running the identical trusted-chat experiment on both runtimes
(Mac + Linux, both on nomadnet 1.2.8, shared LXMF identities) and diffing
against the earlier gonomadnet run. Full reference capture + screen grabs live
in `tooling/parity-reference/` (see `nomadnet-trusted-chat-reference.md`).
**Remove each entry when the Go test proves parity.**

- **B6 (FIXED): gonomadnet LXMF announce/path does not let a remote peer
  reply.** Three root causes found and fixed:
  1. AnnounceNow was not updating PeerSettings.LastAnnounce before saving
     (only the App-level LastAnnounce was set). FIXED in nomadnet/app/app.go.
  2. Mac Mini was missing go.work file, so its binary used the published
     go-reticulum instead of the local source with all fixes. FIXED by
     creating go.work on Mac Mini.
  3. go-reticulum ingress limiting held unknown-destination announces
     indefinitely on busy network interfaces (announce frequency never
     dropped below the burst threshold). FIXED in go-reticulum rns/transport.go:
     RequestPath now calls ReleaseHeldAnnounce on all interfaces to
     immediately release any held announce for the requested destination,
     bypassing the frequency gate. New ReleaseHeldAnnounce method added
     to Interface interface and BaseInterface.

- **B11 (FIXED): gonomadnet↔gonomadnet direct delivery fails both ways.**
  Root cause: go-reticulum's LXMF router sent direct-delivery messages as
  raw fire-and-forget packets instead of establishing a Link (as Python
  LXMRouter.process_outbound does). FIXED in go-reticulum lxmf/router.go:
  ProcessOutbound for MethodDirect now checks for existing direct links,
  uses active ones, or establishes new Links. sendMessagePacketLocked has
  a new MethodDirect link-based branch with delivery/timeout callbacks.
  DeliveryLinkAvailable now checks directLinks. 16 tests updated.
  Also fixed: the list's SetSelectedFunc called showDetail instead of
  DisplayConversation — pressing Enter on the conversation list didn't open
  the conversation. FIXED: changed SetSelectedFunc to call DisplayConversation.

### NEW: TUI rendering bugs found during live bidirectional test (2026-08-24)

- **B14 (NEW): Conversation window does not auto-scroll to latest messages.**
  After sending/receiving messages, the conversation view stays stuck at
  old messages instead of scrolling to show the newest. Both Mac and Mac
  Mini gonomadnet show messages from 20+ minutes ago while the latest
  messages (sent seconds ago) are not visible.

- **B15 (NEW): Incoming messages not displayed in conversation view.**
  The Mac Mini's conversation with the Mac shows only outgoing messages.
  The Mac's incoming messages are on disk (confirmed by file count) but
  are not rendered in the conversation view. The Mac side shows incoming
  messages with "✓ ←" but the Mac Mini side does not show them at all.

- **B16 (NEW): Message state indicator rendering differs from nomadnet.**
  gonomadnet shows different state indicators than nomadnet:
  - Mac (gonomadnet): `↑ →` for delivered, `✕ →` for failed, `✓ ←` for
    incoming, `!` and `⛿` suffixes
  - Mac Mini (gonomadnet): `✓ →` for delivered, `→` for sent, no incoming
    markers, empty suffix
  - nomadnet (Python): consistent `↑ →`/`✕ →`/`✓ ←` with `!`/`⛿` suffixes
  The gonomadnet rendering is inconsistent between the two instances and
  differs from the Python reference.

### Reference behaviors to verify gonomadnet matches (not yet confirmed as bugs)

---

## gonomadnet 0.22.0 exploration findings (live run vs nomadnet 1.2.8 reference)

Full detail + captures in `tooling/parity-reference/gonomadnet-0.22.0-findings.md`
(captures `g40`–`g42`). gonomadnet 0.22.0 run on Mac + Mac Mini M2
(`gnomad-linux` tmux session ssh'd to `glenn-mac-mini-m2`), same public RNS
transports, shared `~/.nomadnetwork` identities (Mac LXMF `2a6105…`, Mac Mini
`712ffbf…`), in 24-bit via
`env -u NO_COLOR COLORTERM=truecolor TERM=xterm-256color ./gonomadnet.sh`.
**These are gonomadnet (Go) bugs to fix TDD-style; do not fix here.**

### CRITICAL

### Re-confirmed on gonomadnet 0.22.0

### Operational note (not a bug, but affects debugging)

- gonomadnet's application log (`~/.nomadnetwork/logfile`) logs announce
  receives, path learning, and (after B6 fix) announce sends/failures, but
  does NOT log LXMF send/receive/deliver events. Consider adding LXMF delivery
  logging to gonomadnet for easier debugging.

---