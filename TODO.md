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

(none — pick the next parity gap by diffing a capture against
`tooling/parity-reference/`, or re-run the workflows in the parity skill.)

## OPEN: RRC Channels parity bugs (6-node #test A/B, 2026-09-03)

Evidence: `tooling/parity-reference/captures/test-room/{node}.txt` and
`{node}.sgr.txt` — five gonomadnet nodes + one Python `nomadnet` SOT
(`glenn-mac-mini-m2`), all in `#test` on the local rrcd hub after the user sent
"Message 1..6" from each node. Python file = ground truth.

**Message stream / ordering**

1. Reverse chronological order: Go renders newest message at TOP (Message 6
   before Message 1); Python renders oldest→newest. Every Go node showed it.
2. On-join history replay dropped: rrcd sends the room history on JOIN
   ("Sending N response(s)"); Go keeps only its own session's messages.
   Python renders the full replay.
3. Old history interleaves after new messages in the same buffer (replay
   appended after live traffic, wrong order within the buffer).
4. Duplicated messages: rrcd fans out one copy per room member with that
   member's nick in K_NICK (unique mids per copy), so Go's mid-based dedupe
   cannot collapse them; own message appears 2x (self-echo + hub copy), others
   appear nickless. Python renders all copies too — Go and Python disagree on
   dedupe strategy; decide parity target with user.
5. "room test: unregistered; mode=(none); topic=(none)" system notice rendered
   2–3 times per join.
6. Last chat row clipped: the message above the composer renders only its
   first line; Python shows a clean bottom indicator instead.

**Users panel**

7. Member counts differ per client and are all wrong (4/5/2/5/3/6 seen for 6
   members) — JOINED/Parted fanouts lost per client after reconnect storms.
8. Member rows show bare hashes for nickless peers instead of Python's
   nick-or-truncated-nick model; learned-from-message nicks render hard-cut
   ("Go port of NomadN") where Python ellipsizes ("Go port of Noma…").
9. Member list ordering differs from Python's (Python: self first then nicks).

**Colors / attributes**

10. Nick color model: Python cycles `nick_colors` per message index (same nick
    → different colors per message, verified 5+ palette values); Go hashes the
    nick (constant per nick, 3 values seen). Same shared palette — assignment
    differs.
11. Message body text fg `215;215;215` vs Python `221;221;221` (#ddd).
12. No timestamps on messages; Python prefixes grey `[HH:MM:SS]`.
13. Hash substrings in message bodies not linkified (Python: underline +
    fg `119;153;221` on 32-hex runs).

**Layout / chrome**

14. Double borders on the room area: Go draws an outer border around the right
    pane plus the inner message box and Users box borders; Python has no outer
    right-pane border (single borders only).
15. Room header too sparse: Go `#test @ RaspPi Local Hub`; Python
    `#test ┄ RaspPi Local Hub v0.3.2  (RaspPi Local Hub) | Connected`.
16. Users header/footer styling: Go shows plain "Users" + count only; Python
    also renders "→" self row first.
17. Shortcut bar does not switch to the editor bar when the composer focuses
    (Python: `[C-d] Send [C-x] Leave [F8] Collapse [Tab] Complete Nick`).
18. Hub info panel: "Server" line omits hub version (Python appends `v0.3.2`);
    hints read "(Ctrl-E to toggle)" vs Python "(Ctrl-E to edit)"; divider glyph
    differs (single `┄` vs Python's full-width run).
19. New Hub dialog is two sequential single-field dialogs vs Python's single
    dialog with address+name+error line; Tab inside dialogs navigates (Go)
    vs tab-completion (Python — Down/Up is the traversal).

**RRC wire/behavior**

20. "Connected" status set at link-establish before WELCOME (Python sets
    CONNECTING "Identified, sending HELLO" until WELCOME arrives).
21. Nick silently omitted when display name > hub `max_nick_bytes` (32) —
    Python sends it verbatim too, but hub drops it: decide clamp-vs-omit parity.
22. Idle link teardown after ~30s (low-RTT paths) / ~200s (relays): client
    keepalive never answered by rrcd; links cycle reconnect storms that
    multiply room members and duplicates. Root cause in go-reticulum
    keepalive/pong cross-implementation — needs forensics session.
23. `MaybeAutoconnect` exists but is never called (dead code).

**rrcd 0.3.2 (server, report upstream — not gonomadnet)**

24. Fanout sends one copy per member with that member's nick in K_NICK; every
    member receives every copy.
25. History replay attaches registry nicks (wrong peer's nick) to copies.
26. `Forwarded` log line prints peer/nick of a different member than sender.
27. Over-long nicks (>32B) dropped silently with no client-visible error.