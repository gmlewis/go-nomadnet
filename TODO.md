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

- **B2 (confirmed): gonomadnet conversation header shows the peer's LXMF hash
  `<hash>`; nomadnet shows the peer's display name.** Both had the peer named
  "Linux-OMEN". nomadnet header: `Linux-OMEN | ◷ 2 hops`; gonomadnet header:
  `<9a2fe7…> | ◷ 2 hops`. Fix: use the directory display name in the header.

- **B3 (confirmed): gonomadnet "My LXMF" (C-p) is an "LXMF Address" modal with
  `<hash>` (no internal spaces) and a "not available" placeholder; nomadnet is a
  "QR Code" modal rendering an actual QR code + `< hash >` (spaces inside the
  brackets).** gonomadnet `OnShowQR` (`textui.go:823`) emits "LXMF address
  display — not available". Fix: render the QR code and match the title/format.

- **B4 (candidate): modal dismiss key.** gonomadnet dismisses the C-p modal on
  Esc; nomadnet's QR popup does NOT dismiss on Esc — it dismisses on Space (and
  a key like C-n is absorbed by the popup rather than dismissed). Confirm and
  match the dismiss semantics.

- **B5 (candidate): New Conversation dialog field-advance key.** gonomadnet
  advances Addr→Name on Tab; nomadnet does NOT — Tab is consumed by the
  ReadlineEdit (text appended to the current field) and the advance key is Down.
  Confirm which behavior the port targets.

- **B6 (candidate, MAJOR): gonomadnet LXMF announce does not propagate to remote
  peers, so a remote peer cannot learn a path to a gonomadnet instance and
  cannot reply.** In the gonomadnet run the Linux side showed `Mac | ◷ unknown`
  after 10+ min and the reply never sent; in the nomadnet run the Linux side
  showed `Mac | ◷ 2 hops` and the reply delivered. Both boxes share the same
  public RNS transport (dfw.us.g00n.cloud:6969), and `gornpath` from the Linux
  box to the Mac's LXMF hash succeeded (4 hops) — so the network is fine; the
  gonomadnet instance's own announce is not being learned by the remote
  gonomadnet. Investigate gonomadnet/go-reticulum announce emission + flood.

- **B7 (candidate): outgoing/incoming message status glyph.** nomadnet uses `✓`
  as the leading marker for both outgoing (`✓ →`) and trusted-incoming (`✓ ←`);
  gonomadnet uses `↑` for outgoing and `⚠` for untrusted-incoming. The `→`/`←`
  direction arrows match; the leading status glyph differs. Confirm the trusted
  vs untrusted incoming glyph set against nomadnet and match it.

- **B8 (candidate): failed/queued outgoing message not shown.** When the
  gonomadnet reply could not send (no path, see B6) it did not appear in the
  conversation at all; nomadnet shows the outgoing message immediately
  (`✓ → just now … ⚿`) before delivery. Disambiguate from B6 (does gonomadnet
  only show outgoing after successful send/path-resolution?).

- **B9 (candidate): tab switching via keyboard Up-at-top → menu.** nomadnet 1.2.8
  does NOT move focus to the menu header on Up-at-top of the Conversations list
  (the `IndicativeListBox` doesn't return `"up"` unhandled at top, so the
  `ConversationsArea` Up→header transition never fires); tab switching is done
  by mouse-clicking the menu bar. Verify whether gonomadnet's keyboard
  Up-at-top→menu fires (it likely does, since gonomadnet's dispatcher owns the
  Up-at-top→FocusMenu transition) — if so, that is a divergence to reconcile.

### Reference behaviors to verify gonomadnet matches (not yet confirmed as bugs)

- **R1:** "Delete conversation" (C-x) removes the conversation/messages but
  RETAINS the directory entry (name + trust). An incoming message then
  re-creates the conversation under the retained entry (nomadnet: deleted "Mac"
  → Trusted 3→2; Mac's next message re-added "✓ Mac" Trusted). Verify gonomadnet
  retains the directory entry across delete.

- **R2:** An open conversation auto-refreshes when a new message arrives
  (nomadnet: the Linux reply appeared live in the Mac's open conversation
  without re-selecting). Verify gonomadnet auto-refreshes (could not test
  earlier because the reply never arrived — blocked by B6).

- **R3:** "Unknown Origin" warning (`⚠ ← Unknown Origin`) is shown for messages
  from untrusted/unknown peers and suppressed for trusted peers (nomadnet: the
  trusted Mac message showed `✓ ← …`, no warning). Verify gonomadnet's
  trust-dependent warning matches.

---
