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

- **B1 (confirmed): gonomadnet auto-opens the conversation after New
  Conversation (C-n) Create.** nomadnet 1.2.8 does NOT — after Create it returns
  to the list with "No conversation selected"; the user must select the peer and
  press Enter to open it. gonomadnet `cmd/gonomadnet/textui.go:816` calls
  `DisplayConversation(addrHex)` after Create and the comment at
  `tui/conversations.go:1868` claims it "matches Python where the dialog closes
  into the conversation's focused footer" — but the live Python 1.2.8 does not.
  Fix: drop the auto-open (or confirm against the exact ported version).
  USER NOTE: I think we have to be careful with this one because the behavior
  changes once a remote note becomes "trusted", even if the conversation is
  deleted, this trusted user is remembered. So to fully verify this, we should
  perform this experiment AGAIN with TWO DIFFERENT nodes that have never conversed
  before... BEFORE we declare any behavioral parity differences here. TODO.
  AGENT FOLLOW-UP: Done — re-tested with two never-conversed nodes (Mac ↔ Mac
  Mini M2, both nomadnet 1.2.8, captures `40`–`46` in
  tooling/parity-reference/). B1 RE-CONFIRMED clean: nomadnet 1.2.8 still does
  NOT auto-open after Create (right pane "No conversation selected", peer added
  to Trusted). So B1 is NOT a trust-contamination artifact; it holds for a
  first-contact node. B1 stays confirmed. (Also re-confirmed on the gonomadnet
  side in the 0.22.0 run — gonomadnet DID auto-open, capture `g40`.)

- **B2 (confirmed, nuanced — see gonomadnet 0.22.0 findings): gonomadnet
  conversation header shows the peer's LXMF hash `<hash>`; nomadnet shows the
  peer's display name.** Both had the peer named "Linux-OMEN". nomadnet header:
  `Linux-OMEN | ◷ 2 hops`; gonomadnet header: `<9a2fe7…> | ◷ 2 hops`. Fix: use
  the directory display name in the header. (In the 0.22.0 run gonomadnet showed
  the hash for "MacMini" but the name "Mac" for the Mac — see B2-nuance in the
  gonomadnet 0.22.0 section; the rule is inconsistent and needs pinning down.)

- **B3 (confirmed): gonomadnet "My LXMF" (C-p) is an "LXMF Address" modal with
  `<hash>` (no internal spaces) and a "not available" placeholder; nomadnet is a
  "QR Code" modal rendering an actual QR code + `< hash >` (spaces inside the
  brackets).** gonomadnet `OnShowQR` (`textui.go:823`) emits "LXMF address
  display — not available". Fix: render the QR code and match the title/format.
  (Re-confirmed on gonomadnet 0.22.0: modal titled "LXMF Address", `<2a6105…>`
  no spaces, no QR.)

- **B4 (candidate → keep Esc, add nomadnet parity): modal dismiss key.**
  gonomadnet dismisses the C-p modal on Esc; nomadnet's QR popup does NOT dismiss
  on Esc — it dismisses on Space (and a key like C-n is absorbed by the popup
  rather than dismissed). Confirm and match the dismiss semantics.
  USER NOTE: I like the additional (very natural) support for the Esc key to
  dismiss modals, and I don't mind having `gonomadnet` have ADDITIONAL nice
  features for the user as long as they don't interfere with parity operation.
  So in other words, if gonomadnet supports everything that nomadnet supports,
  but in addition has added functionality that is helpful for the user, then
  let's keep the added helpful capabilities (like the Esc key support which
  seems really natural, obvious, and intuitive).
  AGENT RESPONSE: Agreed — keep gonomadnet's Esc-to-dismiss as a deliberate
  enhancement. The parity bar is "gonomadnet supports AT LEAST nomadnet's
  dismiss inputs" — so the action for B4 is to ADD nomadnet's dismiss parity
  (Space, and any key nomadnet uses to close the QR/popup) on top of the kept
  Esc, not to remove Esc. B4 is a "candidate parity gap to close by addition,"
  not a regression to revert.

- **B5 (confirmed): New Conversation dialog field-advance key.** gonomadnet
  advances Addr→Name on Tab; nomadnet does NOT — Tab is consumed by the
  ReadlineEdit (text appended to the current field) and the advance key is Down.
  (Re-confirmed on gonomadnet 0.22.0, capture `g40`.)

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

- **B7 (confirmed): outgoing/incoming message status glyph.** nomadnet uses `✓`
  as the leading marker for both outgoing (`✓ →`) and trusted-incoming (`✓ ←`);
  gonomadnet uses `↑` for outgoing and `⚠` for untrusted-incoming. The `→`/`←`
  direction arrows match; the leading status glyph differs. (Re-confirmed on
  gonomadnet 0.22.0: outgoing `↑ → … ⚿`, capture `g41`.) Confirm the trusted vs
  untrusted incoming glyph set against nomadnet and match it.

- **B8 (confirmed via B11): failed/queued outgoing message not shown.** When the
  gonomadnet reply could not send (no path, see B6/B11) it did not appear in the
  conversation at all (no on-disk file, nothing displayed); nomadnet shows the
  outgoing message immediately (`✓ → just now … ⚿`) before delivery. Note
  gonomadnet DOES show a successful-looking `↑ → ⚿` for a send that then fails
  to deliver (B11) — so the `⚿` status is not a reliable delivery indicator.
  Fix: show the outgoing message immediately (optimistic) and reflect real
  delivery state in the status glyph.

- **B9 (candidate): tab switching via keyboard Up-at-top → menu.** nomadnet 1.2.8
  does NOT move focus to the menu header on Up-at-top of the Conversations list
  (the `IndicativeListBox` doesn't return `"up"` unhandled at top, so the
  `ConversationsArea` Up→header transition never fires); tab switching is done
  by mouse-clicking the menu bar. Verify whether gonomadnet's keyboard
  Up-at-top→menu fires (it likely does, since gonomadnet's dispatcher owns the
  Up-at-top→FocusMenu transition) — if so, that is a divergence to reconcile.
  NOTE: gonomadnet mouse via tmux SGR injection does not work at all (B12), so
  on gonomadnet the menu CANNOT be reached by mouse either — keyboard
  Up-at-top→menu is the ONLY way on gonomadnet; confirm it works.

### Reference behaviors to verify gonomadnet matches (not yet confirmed as bugs)

- **R1:** "Delete conversation" (C-x) removes the conversation/messages but
  RETAINS the directory entry (name + trust). An incoming message then
  re-creates the conversation under the retained entry (nomadnet: deleted "Mac"
  → Trusted 3→2; Mac's next message re-added "✓ Mac" Trusted). Verify gonomadnet
  retains the directory entry across delete. (gonomadnet's C-x Delete removed
  MacMini from the list (Trusted 6→5) in the 0.22.0 run, but whether the trust
  entry was retained was not re-tested.)

- **R2:** An open conversation auto-refreshes when a new message arrives
  (nomadnet: the Linux reply appeared live in the Mac's open conversation
  without re-selecting; confirmed again in the Mac Mini never-conversed test).
  Verify gonomadnet auto-refreshes — could NOT be tested on gonomadnet 0.22.0
  because no gonomadnet message arrived (blocked by B11).

- **R3 (REFINED): untrusted/unknown-peer warning.** nomadnet 1.2.8 shows a
  HEADER BANNER ` This peer isn't trusted yet.` directly under the conversation
  header, then the message with the NORMAL marker `✓ ← … ⚿` (NOT an inline
  `⚠ ← Unknown Origin`). The earlier "⚠ ← Unknown Origin + `!`" wording was from
  the gonomadnet run, NOT nomadnet 1.2.8. So the parity difference is:
  gonomadnet uses inline `⚠ ← Unknown Origin` + `!` status; nomadnet 1.2.8 uses
  a header banner `This peer isn't trusted yet.` + normal `✓ ← … ⚿`. Match
  nomadnet's header-banner presentation. (On gonomadnet 0.22.0 this was masked
  by B10 — gonomadnet showed `← Invalid Signature` instead.)

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

- **B10 (NEW, CRITICAL — cross-impl LXMF signature incompatibility):** gonomadnet
  displays a nomadnet (Python)-sent LXMF message as `← Invalid Signature`. The
  Mac Mini gonomadnet showed the Mac's nomadnet message (17:26:54 "Hello Mac
  Mini! First contact…") with `← Invalid Signature` + `… | 2026-08-23 17:26:54`
  (no `⚿`, a blank status glyph). The same stored message verified fine in
  nomadnet. So go-reticulum/gonomadnet cannot verify an LXMF signature produced
  by Reticulum-Python/nomadnet — a signature-scheme/format incompatibility that
  breaks nomadnet→gonomadnet message trust. Investigate the LXMF signature
  verification path in go-reticulum vs Reticulum (curve/format/encoding). This
  is the highest-priority interop bug.

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

### Re-confirmed on gonomadnet 0.22.0

- **B1** — gonomadnet auto-opens after `C-n` Create (header
  `<hash> | ◷ 2 hops` + "No messages yet. Type below to send." + composer);
  nomadnet does NOT. (capture `g40`)
- **B5** — gonomadnet advances dialog fields with `Tab`; nomadnet uses `Down`.
  (capture `g40`)
- **B7** — gonomadnet outgoing marker `↑ →`; nomadnet `✓ →`. (capture `g41`)

### New, needs investigation

- **B2-nuance:** gonomadnet's conversation header showed the **hash**
  `<712ffbf…>` for "MacMini" in the Mac's view (matching the earlier Linux-OMEN
  observation), BUT showed the **name** `Mac |  9 hops` in the Mac Mini's view
  of the Mac (named "Mac" via C-n). So gonomadnet's header-name-vs-hash choice
  is inconsistent — it may depend on whether the peer announced a display name
  vs the manual directory name. Determine the exact rule and match nomadnet
  (which always uses the directory display name).

- **B12 (NEW — gonomadnet mouse via tmux SGR-1006 injection does NOT work):**
  Clicking the `[ Network ]` menu button and the `[ Untrusted ]` tab on
  gonomadnet did nothing (bar/view unchanged), while the identical clicks
  worked on nomadnet (urwid). gonomadnet does `tviewApp.EnableMouse(true)`, so
  this is a tcell/tview mouse-mode or tmux-passthrough discrepancy, not "mouse
  disabled." Impact: menu tab-switching, the Trusted/Untrusted tab toggle
  (which has NO keyboard shortcut in gonomadnet), and all mouse-driven nav are
  unreachable via tmux injection — a workflow/automation parity gap (and it
  blocked viewing the Mac Mini's Untrusted list; the workaround was to C-n add
  the Mac as Trusted so it appeared in the viewable Trusted tab). Investigate
  tcell SGR-1006 mouse handling under tmux.

- **B13 (NEW — path/hop count discrepancy):** the Mac Mini gonomadnet showed
  `Mac |  9 hops` for the Mac, where nomadnet (same machines, same transports)
  showed `2 hops`. Either gonomadnet is choosing a far longer path or its hop
  count display/calculation differs. Confirm whether this is a real path-choice
  difference or a display bug.

### Outstanding (blocked by B10/B11)

- **R1** (delete retains directory entry) — gonomadnet's C-x Delete removed
  MacMini from the list (Trusted 6→5); whether the directory trust entry is
  retained was not re-tested.
- **R2** (open-conversation auto-refresh) — could not be tested because no
  gonomadnet message arrived (B11).
- **R3** (untrusted warning) — gonomadnet showed `← Invalid Signature` (B10)
  instead of nomadnet's `This peer isn't trusted yet.` header banner, so the
  warning presentation also differs (masked by the signature bug).

### Operational note (not a bug, but affects debugging)

- gonomadnet's stderr log (`gonomadnet-*.log`) is nearly empty (only the pprof
  line) — it does NOT log LXMF send/receive/deliver events, which made
  diagnosing B11 (delivery failure) hard. Consider adding LXMF delivery logging
  to gonomadnet.

---