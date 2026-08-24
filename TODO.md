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

- **B6 (CONFIRMED gonomadnet-specific, MAJOR): gonomadnet LXMF announce/path
  does not let a remote peer reply.** In the gonomadnet run the Linux side
  showed `Mac | ◷ unknown` after 10+ min and the reply never sent; in the
  nomadnet run the Linux side showed `Mac | ◷ 2 hops` and the reply delivered.
  Both boxes share the same public RNS transport (dfw.us.g00n.cloud:6969), and
  `gornpath` from the Linux box to the Mac's LXMF hash succeeded (4 hops) — so
  the network is fine; the gonomadnet instance's own announce is not being
  learned by the remote gonomadnet. Investigate gonomadnet/go-reticulum announce
  emission + flood. CONFIRMED clean with the Mac Mini (never-conversed):
  nomadnet Python learned the path on send (`unknown` → `2 hops`) and replied
  successfully; gonomadnet stayed `unknown`/failed (see B11). Investigate
  gonomadnet/go-reticulum announce emission + flood AND on-send path resolution.
  AGENT NOTE: B13 fix (announce rebroadcast hop count inflation) may resolve
  this — the inflated hop count (2N-1 instead of N) caused announces to exceed
  ReticulumHopsMax and stop propagating. Needs live verification.

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

- **B11 (NEW, CRITICAL — gonomadnet↔gonomadnet direct delivery fails both
  ways):** The Mac gonomadnet sent a message to the Mac Mini gonomadnet
  (conversation header `MacMini | ◷ 2 hops`, outgoing rendered
  `↑ → just now | 17:44:02 ⚿`), but it NEVER arrived at the Mac Mini — no
  on-disk message file in `~/.nomadnetwork/storage/conversations/712ffbf…/` and
  nothing displayed. The Mac Mini's reply likewise did not send/store (no file)
  and did not arrive at the Mac. So gonomadnet→gonomadnet LXMF direct delivery
  is broken both ways (the sender shows `↑ → ⚿` as if sent, but nothing
  delivers). This subsumes/confirms B6 (announce/path) and B8 (failed outgoing
  not surfaced) for the gonomadnet↔gonomadnet case. Investigate LXMF direct-send
  + path resolution in gonomadnet/go-reticulum. Second-highest-priority interop
  bug.
  AGENT NOTE: A TCP-based Go→Go integration test (TestB11TCPDirectDelivery in
  go-reticulum/lxmf/lxmf-int-tcp_test.go) PASSES — direct delivery over a
  loopback TCP transport works. The bug requires the actual public multi-hop
  RNS network to reproduce. Needs live debugging with gornpath/gornstatus.
  AGENT LIVE TEST (2026-08-23): With both gonomadnet instances running on the
  live network, Mac Mini→Mac delivery WORKS (✓ ← signature verified — B10 fix
  confirmed!). But Mac→Mac Mini delivery FAILS — the packet is sent
  (sendMessagePacketLocked succeeds, receipt=true) but lost in transit. Root
  cause was B13 (hop count inflation, now FIXED): go-reticulum's rebroadcast
  hop count was packet.Hops+1 instead of packet.Hops, compounding across hops
  (N actual hops showed as 2N-1). The inflated hop count caused announces to
  exceed ReticulumHopsMax and stop propagating, leading to long/missing paths.
  The Mac Mini→Mac direction worked because the Mac Mini had a shorter path.
  Needs live verification that B13 fix resolves this.
  Also found: the list's SetSelectedFunc called showDetail (peer info text)
  instead of DisplayConversation (conversation widget with composer) — pressing
  Enter on the conversation list didn't open the conversation. FIXED: changed
  SetSelectedFunc to call DisplayConversation.

### Re-confirmed on gonomadnet 0.22.0

### Operational note (not a bug, but affects debugging)

- gonomadnet's stderr log (`gonomadnet-*.log`) is nearly empty (only the pprof
  line) — it does NOT log LXMF send/receive/deliver events, which made
  diagnosing B11 (delivery failure) hard. Consider adding LXMF delivery logging
  to gonomadnet.

---