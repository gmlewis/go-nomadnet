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

## Fleet soak (user-driven, after redeploying the 11-findings fixes)

The 11 findings from the 2026-09-03 12:32 full-fleet A/B re-deploy diff are
fixed with green unit tests plus Python-mini-hub integration tests over a
loopback TCP RNS transport (nomadnet/rrc/integration-fanout_test.go: the
fanout burst collapse + nick backfill + arrival timestamps + member/nick
learning, the roomless MOTD notice joining the active room, and the
full-list JOINED heal; tui/channels-fill_test.go, tui/
room-widget-users-pane_test.go and tui/indicative-centering_test.go pin the
hub-row fill, the users-row fill and the ILB bar centering). Remaining user
steps: the fix files are already copied into the raspberrypi, penguin,
glenn-OMEN-875 and glenn-nano2gb checkouts (local is the working tree, the
mac-mini runs the Python SOT and is untouched) - restart each node's
./gonomadnet.sh, then soak the live fleet confirming every node shows the
same stable member count in the Channels #test room, and save the final
full-fleet capture set into tooling/parity-reference/captures/test-room/
after-*/.


## RRC Channels parity bugs (6-node #test A/B, 2026-09-03) — ALL 27 RESOLVED

Every numbered item below is fixed and verified; the evidence lives in
`tooling/parity-reference/captures/test-room/after-local*.txt` (the fixed
`local` node converging on the Python SOT capture `glenn-mac-mini-m2.txt`) and
in the test suite. The numbered entries were removed per the TODO rules; the
notes below carry only what the user still needs.

**User follow-ups (agent cannot do these):**

1. **Push go-reticulum.** The keepalive fix (TODO item 22) lives in
   `~/go/src/github.com/gmlewis/go-reticulum` `rns/transport.go` (the Inbound
   duplicate filter now exempts keepalive/resource/cache-request/channel
   packets and never remembers link-table packets, mirroring Python
   Transport.py:1388-1392 + 1529-1531). Push that repo, then bump the
   `replace` pseudo-version in go-nomadnet's `go.mod` (the `go.work` use line
   already covers local dev). Until the fix reaches the other nodes, their
   links keep churning every few idle minutes — the `local` node's link has
   been verified stable across a 5-minute silent window post-fix while the
   unpatched nodes kept churning. Live post-fix detail: the client↔rrcd
   keepalive exchange runs bidirectionally (~17s cadence at the measured
   83ms RTT — the client log now shows the hub's 0xFE replies being
   delivered; they never arrived before the fix) and one link survived a
   10.5-minute silent window (pre-fix links died in 30-100s). The residual
   occasional churn on this node is single-loss fragility inherent to
   Python's keepalive design (stale = 2x keepalive; keepalives are single
   unacknowledged packets) on a lossy 2-hop path — Python has the same
   exposure there; the Python SOT's hours-long links ride a hops=0 LAN path
   instead.
2. **Deploy to the other 5 nodes.** ✅ DONE by the user (2026-09-03 ~12:3x) —
   the 12:32 full-fleet captures in /tmp are the first post-deploy A/B set.

**Fix index (what to look at):** items 1-5 — `nomadnet/rrc/hub.go`
(recordMessage chronological append, fanoutGroups/recentSentBodies collapse,
learnMsgPeer member/nick learning) + `tui/message-sort.go` stable ts sort +
load-time collapse in loadHistory; item 6 — `tui/indicative-messages.go`
(IndicativeListBox bars) + sticky tail in renderMessages; items 7-9 —
member learning + `tui/room-widget.go` renderMembers row model; items 10-13 —
`tui/message-body.go` (the Python _message_widget render model: ts prefix,
#dddddd body, palette-by-src nick, linkified hash runs); items 14-19 —
right-pane single borders, room header verbatim, Users-in-border title,
editor-region shortcut bar, hub-info version/hints/full-width divider, and the
single-dialog New Hub form; items 20-23 — CONNECTING until WELCOME, verbatim
nick (decided parity), and the MaybeAutoconnect wiring. Items 24-27 are
upstream rrcd 0.3.2 bugs worked around client-side (no rrcd patching, per the
user's direction); report them upstream.

## Room composer multiline wrap (fleet bug, 2026-09-03) — FIXED locally, awaiting deploy

`gonomadnet`'s Channels room composer was a fixed one-row single-line field:
long drafts clipped at the panel's right border and the hardware caret walked
past the panel edge (live on glenn-OMEN-875), while Python's
`RoomMessageEdit(caption="", edit_text="", multiline=True)` urwid-Frame footer
(Channels.py:605) wraps the draft and grows the panel. Fixed in
`tui/readline-multiline.go` + `tui/room-widget.go` (urwid-parity wrap rows,
wrapped caret, footer growth via the chatBox DrawFunc, body focus
Up/Down/Tab), pinned by `tui/editor-multiline-parity_test.go` +
`tui/room-editor-grow-parity_test.go` (goldens captured live from urwid
4.0.3) and verified visually in a local tmux replica at the fleet geometry.

**User follow-up (agent cannot do this):** commit/push the go-nomadnet tree,
then rebuild and restart `./gonomadnet.sh` on glenn-OMEN-875 (and the other Go
nodes when convenient); the mac-mini runs the Python SOT and is untouched.
