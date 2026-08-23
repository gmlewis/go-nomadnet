# gonomadnet 0.22.0 exploration findings (live run vs nomadnet 1.2.8 reference)

Run gonomadnet 0.22.0 on both the Mac (`gonomadnet` tmux session) and the Mac
Mini M2 (`gnomad-linux` tmux session, ssh'd to `glenn-mac-mini-m2`) — same
public RNS transports, shared `~/.nomadnetwork` identities (Mac LXMF
`2a6105f57145860441a62fe3b2a1352c`, Mac Mini `712ffbfdb82c7fe60d0c5fa163ad2955`)
— in 24-bit via `env -u NO_COLOR COLORTERM=truecolor ./gonomadnet.sh`, and
re-ran the previously-unknown-node flow. Captures `g40`–`g42` in
`tooling/parity-reference/captures/`. Reference: `nomadnet-trusted-chat-reference.md`.

NOTE: TODO.md was being touched by something during this round (edit tool kept
rejecting "file changed since read"), so these findings live here to be merged
into TODO.md as B10–B13 + the re-confirmations.

These are interop/behavior bugs in gonomadnet (Go) to fix TDD-style; do not fix
here.

## CRITICAL

- **B10 — cross-impl LXMF signature incompatibility:** gonomadnet displays a
  nomadnet (Python)-sent LXMF message as `← Invalid Signature`. The Mac Mini
  gonomadnet showed the Mac's nomadnet message (17:26:54 "Hello Mac Mini! First
  contact…") with `← Invalid Signature` + `… | 2026-08-23 17:26:54` (no `⚿`, a
  blank status glyph). The same stored message verified fine in nomadnet. So
  go-reticulum/gonomadnet cannot verify an LXMF signature produced by
  Reticulum-Python/nomadnet — a signature-scheme/format incompatibility that
  breaks nomadnet→gonomadnet message trust. Investigate the LXMF signature
  verification path in go-reticulum vs Reticulum (curve/format/encoding).

- **B11 — gonomadnet↔gonomadnet direct delivery fails (both ways):** The Mac
  gonomadnet sent a message to the Mac Mini gonomadnet (conversation header
  `MacMini | ◷ 2 hops`, outgoing rendered `↑ → just now | 17:44:02 ⚿`), but it
  NEVER arrived at the Mac Mini — no on-disk message file in
  `~/.nomadnetwork/storage/conversations/712ffbf…/` and nothing displayed. The
  Mac Mini's reply likewise did not send/store (no file) and did not arrive at
  the Mac. So gonomadnet→gonomadnet LXMF direct delivery is broken both ways
  (the sender shows `↑ → ⚿` as if sent, but nothing delivers). This subsumes/
  confirms B6 (announce/path) and B8 (failed outgoing not surfaced) for the
  gonomadnet↔gonomadnet case. Investigate LXMF direct-send + path resolution in
  gonomadnet/go-reticulum.

## Re-confirmed

- **B1 — gonomadnet auto-opens the conversation after `C-n` Create** (header
  `<hash> | ◷ 2 hops` + "No messages yet. Type below to send." + composer);
  nomadnet 1.2.8 does NOT. (capture `g40`)
- **B5 — gonomadnet advances New Conversation dialog fields with `Tab`
  (Addr→Name); nomadnet uses `Down`.** (capture `g40`)
- **B7 — gonomadnet outgoing marker is `↑ →` (nomadnet `✓ →`).** (capture `g41`)

## New, needs investigation

- **B2 (nuanced):** gonomadnet's conversation header showed the **hash**
  `<712ffbf…>` for "MacMini" in the Mac's view (matching the earlier Linux-OMEN
  observation), BUT showed the **name** `Mac |  9 hops` in the Mac Mini's view
  of the Mac (named "Mac" via C-n). So gonomadnet's header-name-vs-hash choice
  is inconsistent — it may depend on whether the peer announced a display name
  vs the manual directory name. Determine the exact rule and match nomadnet
  (which always uses the directory display name).

- **B12 — mouse input via tmux SGR-1006 injection does NOT work on gonomadnet.**
  Clicking the `[ Network ]` menu button and the `[ Untrusted ]` tab on
  gonomadnet did nothing (bar/view unchanged), while the identical clicks
  worked on nomadnet (urwid). gonomadnet does `tviewApp.EnableMouse(true)`, so
  this is a tcell/tview mouse-mode or tmux-passthrough discrepancy, not "mouse
  disabled." Impact: tab switching (menu), the Trusted/Untrusted tab toggle
  (which has NO keyboard shortcut in gonomadnet), and all mouse-driven nav are
  unreachable via tmux injection — a workflow/automation parity gap.
  Investigate tcell SGR-1006 mouse handling under tmux. (This is why the
  Untrusted tab couldn't be opened on the Mac Mini gonomadnet — the workaround
  was to C-n add the Mac as Trusted so it appeared in the viewable Trusted tab.)

- **B13 — path/hop count discrepancy:** the Mac Mini gonomadnet showed
  `Mac |  9 hops` for the Mac, where nomadnet (same machines, same transports)
  showed `2 hops`. Either gonomadnet is choosing a far longer path or its hop
  count display/calculation differs. Confirm whether this is a real path-choice
  difference or a display bug.

## Outstanding (blocked by B10/B11)

- **R1** (delete retains directory entry): gonomadnet's C-x Delete removed
  MacMini from the list (Trusted 6→5); whether the directory trust entry is
  retained was not re-tested.
- **R2** (open-conversation auto-refresh): could not be tested because no
  gonomadnet message arrived (B11).
- **R3** (untrusted warning): gonomadnet showed `← Invalid Signature` (B10)
  instead of nomadnet's `This peer isn't trusted yet.` header banner, so the
  untrusted-peer warning presentation also differs (and is masked by the
  signature bug).

## Operational notes (not bugs, but affected the run)

- gonomadnet 0.22.0 starts fine in 24-bit via `env -u NO_COLOR
  COLORTERM=truecolor TERM=xterm-256color ./gonomadnet.sh` (the gonomadnet.sh
  rebuild + run worked on both Mac and Mac Mini).
- gonomadnet's stderr log (`gonomadnet-*.log`) is nearly empty (only the pprof
  line) — it does NOT log LXMF send/receive/deliver events, which makes
  diagnosing B11 (delivery failure) hard. Consider adding LXMF delivery logging
  to gonomadnet.