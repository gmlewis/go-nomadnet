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

## OPEN: full-fleet A/B re-deploy diff (6 nodes, 2026-09-03 12:32)

Evidence (full-color screenshots, saved in `/tmp`): `shot-{node}.txt` (plain)
and `shot-{node}.sgr.txt` (SGR), decoded per-row/per-cell into
`/tmp/dec-{node}.json` via `tooling/tui-parity/ansiview.py --json`. The
comparison script is `/tmp/analyze.py`. All 6 sessions were driven to the
Channels `#test` room before capture; `glenn-mac-mini-m2` (Python `nomadnet`
1.2.8, urwid) is the SOT. The user's Messages A1–A6 (sent 12:26–12:28) exist
on all 6 nodes and are the shared render fixture. Chat-pane columns below are
relative to the chat box inner edge (capture col 37); the chat inner is 96
cols wide on all 6 nodes (border `┐` at abs cols 35/133/155 on every node).

**Python render-model facts pinned from the SOT capture** (golden values for
the tests below): `NomadNetworkApp.py:163-166` defaults —
`rrc_ui_justify_msgs=True`, `rrc_ui_space_msgs=False`,
`rrc_ui_render_markdown=True`, `rrc_ui_render_micron=True`.

1. **Composer renders a "Type a message..." placeholder; Python's editor is
   empty.** Row 34 chat pane: mac `| |` (empty); every Go node
   `|Type a message...|`. Python RoomWidget builds
   `ReadlineEdit(caption="", edit_text="", multiline=True)` (Channels.py:605) —
   no placeholder. Fix: remove the placeholder argument from
   `NewRoomWidget`'s editor (`tui/room-widget.go`). Test: render an idle
   RoomWidget and assert the editor row is empty.
2. **Message rows must render in Python's justify two-column layout** (owner
   symptom: "gonomadnet is missing a single space immediately before the
   nickname, and long messages indent the wrapped line to the same column as
   the `<` while gonomadnet does no indentation at all"). Python (default
   `rrc_ui_justify_msgs=True`, Channels.py:1408-1413) renders each message as
   `urwid.Columns([(PACK, ts-prefix), (body)], dividechars=1)`: the ts
   `[HH:MM:SS] ` (11 chars, fg #888888) then a **default-styled gap space**
   (the column divider, Channels.py:1412) then the body column, so `<nick>`
   lands at chat col **13** (measured: mac A1 `<` at col 13; the gap space
   between ts and nick carries NO fg). Body text wraps inside the remaining
   ~83 cols and EVERY wrapped continuation line starts at chat col **13 — the
   same column as the `<`** (mac rows 19/21/28: `test`, `original
   source-of-truth)` all start at col 13). Go renders inline: `<` at chat col
   12 (no gap space — the missing single space before the nickname) and wraps
   continuation lines to chat col 0 (no indentation). Fix: two-column render
   in `tui/room-widget.go`/`message-body.go` (ts TextView column + body
   column, or pad continuations to col 13 in the TextView), honoring
   `rrc_ui_justify_msgs` from config. Test goldens: `<` at col 13; gap space
   unstyled; continuation indent = 13 (the `<` column); the A2 wrap point.
3. **Leading padding space color.** Python's leading indent space is
   DEFAULT-styled (no fg); Go paints it with the body color (#dddddd).
   Measured on every message row. Fix with the layout rework (task 2) — the
   padding must not inherit the body fg.
4. **system/notice/error rows use the static palette (cube-quantized) colors,
   not the micron nibble-doubled values.** Python: the notice icon+body run is
   fg **(255,215,95)** = #ffd75f (urwid cube of `#fd3`, ui/TextUI.py:66), the
   error row #f55 → **(255,95,95)**, the system row #888 → **(135,135,135)**,
   and the system/notice/error ts prefix ("irc_ts" palette attr) is
   **(135,135,135)** — the notice ts run also carries a leading space inside
   the styled run (`' [12:21:08] '`, measured on the mac Welcome row). Go
   renders notice icon+body (255,221,51) (#ffdd33 nibble-doubled) and ts
   (136,136,136), and styles the leading space with the body color. The
   message-row colors (ts #888888=136, body #dddddd=221, nick palette) are
   byte-exact on all 6 nodes — do NOT change those. Fix: system/notice/error
   rows in `tui/message-body.go` must take the CUBE-quantized palette values
   (`cubeHex3`: #ffd75f / #ff5f5f / #878787) for icon+body and the ts prefix.
   Test: table of kind → {icon+body fg, ts fg} with the live-capture values.
5. **The hub MOTD notice never renders.** Python renders rrcd's global MOTD
   notice in the room ("󰙎 Welcome to the RaspPi Local Hub!", mac row 24):
   `RRCHub._record_notice` (RRC.py:817-839) attributes a roomless notice to
   the manager's ACTIVE room, appends it to that room's buffer and notifies.
   Go's TypeNotice branch stores roomless notices only via `SetMOTD` and routes
   them to the global `Notices` list — the room never shows the welcome.
   Evidence: mac rows 24-25 (Welcome + unregistered) vs Go nodes: only
   "room test: unregistered…". Fix in `nomadnet/rrc/hub.go`: mirror
   `_record_notice` — a roomless notice joins the active room's buffer (and
   still sets MOTD). Test: HandleData with a roomless T_NOTICE → the message
   lands in the active room's buffer with Kind=notice.
6. **Users pane: phantom blank rows + highlighted count row.** Every Go node
   renders a blank row after EACH member row (raspberrypi: count, blank,
   member, blank, member…) — the fork `tview.List` defaults
   `ShowSecondaryText(true)` and each member item's empty secondary text
   paints a phantom row (the channels hub list already calls
   `ShowSecondaryText(false)` for this exact reason). Python renders members
   on consecutive rows. ALSO the Go count row (" 4 users") renders with the
   SELECTION HIGHLIGHT (fg 0,0,0 / bg 175,175,175 — it is the List's current
   item) and the blank row under it inherits the highlight; Python's count row
   is default-styled plain text. Fix in `tui/room-widget.go` renderMembers:
   `rw.usersList.ShowSecondaryText(false)` and move the count row OUT of the
   List (a separate TextView above it, or an empty-widget pattern) so it never
   takes the selection style. Test: renderMembers with 3 members → 4 painted
   rows (count + 3), no blank rows, count row default-styled.
7. **Fanout collapse should prefer a copy that carries a nick.** When
   collapsing a fanout burst the Go keeps the FIRST-arrived copy; A2 renders
   `<464360ee59ed>` (the kept copy's K_NICK was empty) even though rrcd's
   fanout attaches the sender's nick to other copies (the user-ordered
   deviation says render ONCE with the sender's nick). Fix in
   `nomadnet/rrc/hub.go`: while a fanout window is open, if a later copy of
   the same body carries a non-empty nick and the kept message's nick is
   empty, backfill the kept message's nick (and learnMsgPeer it). Test: 6
   fanout copies, first nick="", a later copy nick="N" → the kept message
   renders nick "N".
8. **Timestamp source: arrival time, not the sender's envelope ts.** Python
   stamps EVERY inbound message with its own `_now_ms()` arrival time
   (RRC.py:1043) — A1 shows [12:26:56] on the mac; Go shows the sender's
   envelope ts → [12:26:55] (a real, visible 1s skew; worse on relays). Fix
   in `nomadnet/rrc/hub.go` HandleData (TypeMsg/TypeAction/TypeNotice): stamp
   `Ts = NowMs()` for received messages like Python (keep the envelope ts only
   as the fanout-dedupe window key; the chronological render sort is stable so
   replay order is preserved). Decide parity: match Python. Test: deliver an
   envelope with ts = Now-60s → the recorded message's Ts ≈ now, not the
   envelope's.
9. **ILB indicator bar centering is 3 columns left of Python's.** Python
   centers the bars in the 96-wide chat inner with urwid's ceil center:
   `▲` at chat col 48, `───` at col 47 (mac rows 3/33). Go: `───` at col 44
   on every node (rows 3 and 33), and the ▲ (raspberrypi/OMEN) at col 47 —
   inconsistent between bar lengths and ~3 left of Python. Fix in
   `tui/indicative-messages.go`: center against the chat inner width Python
   uses (the LineBox inner = 96) with `(maxcol-len+1)/2`, and make top/bottom
   bars agree. Test: IndicativeMessages drawn at width 96 → the top bar starts
   at col 48 (▲) / 47 (───), bottom bar likewise.
10. **Member set/nick state: Python's "6 users" is rock-solid; every gonomadnet
    node deviates** (owner symptom). Python's users pane: 6 members, 5 named
    (`→ 464360ee59ed` self-hash + 5 "Go port of Noma…" rows) and it has stayed
    at 6 for hours; the Go nodes: 4-6 members with fewer named (local: 4 users
    = self + a hash + 2 named; raspberrypi: 5 users, 4 hashes + 1 named-self),
    and the counts vary per node and over time. The keepalive fix (resolved
    item 22, now deployed) removes the reconnect storms that used to CLEAR
    member sets on every teardown, so with stable links the Go sets should
    converge to 6 and STAY — the remaining work is the learning coverage:
    (a) Python learns nicks + members from EVERY fanout copy BEFORE its own
    dedupe (RRC.py:1031-1035); the Go's sentIDs/recvIDs guards return before
    `learnMsgPeer` in some paths; (b) check whether rrcd 0.3.2's JOINED fanout
    body carries the FULL member list (each join would then heal the whole set
    client-side) and whether the Go's JOINED handling adds all body hashes the
    same way (RRC.py:944-948); (c) rrcd's per-copy rewritten srcs mean each
    node sees a different subset. Investigate in a dedicated session: log
    every fanout copy's (src, nick) the Go client processes (DEBUG), compare
    against Python's nicks/members bookkeeping (RRC.py:1031-1035, 1106-1118),
    and align the learning coverage. Test: replay a captured fanout burst
    (from the rrcd DEBUG log) through HandleData → the member set and nicks
    match Python's; then a soak on the live fleet confirming all nodes show
    the same stable count.
11. **LOW — full-width attribute fill.** Python paints attributes across the
    full row width (urwid AttrMap fill): the connected hub row's fg #5faf00
    covers all 34 left-pane columns (Go paints only the text run); users rows'
    trailing spaces carry the member's fg; message bodies' trailing spaces
    carry #ddd to the box edge. Invisible at default bg, visible with any bg.
    Fix pattern (from the explore skill's ledger): set the style on the
    background, not only the text run. One task per surface.

**Verified convergent (do NOT re-flag):** the room header is byte-exact
(` #test ┄ RaspPi Local Hub v0.3.2  (RaspPi Local Hub) | Connected `); the nick
palette-by-src colors are byte-exact across all 6 nodes (A1 bbab00, A3 f380c7,
A5 c58ffa, A6 8cacbb); the message ts fg (#888888), body fg (#dddddd), the
notice glyph 󰙎 (U+F064E), the menubar glyph (decoration_menu U+F043B when no
unread / unread_menu U+F0E0 on Darwin — Python TextUI.py:134 is also F0E0 on
Darwin), both shortcut-bar variants, the connected-hub ✓ color #5faf00, the
selected-room highlight (fg 0 / bg 135,135,135), the left-pane ILB bars
(─── centered identically), and the user-ordered fanout dedupe (Python
renders 4-5 copies of each message, Go renders 1 — intentional deviation).

Extra hub rows in some Go lists ("RNS Community Hub", "Ratspeak") are the
user's per-node saved hubs — state, not a bug.

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
