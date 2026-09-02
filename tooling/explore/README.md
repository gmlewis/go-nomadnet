# Differential TUI explorer (py ⇄ go)

`explore.py` drives the Python `nomadnet` (source of truth) and `gonomadnet`
(Go port) through **identical key paths from identical seeded states** and
diffs the results — screen frames (normalized) and on-disk side effects.
Because both implementations get byte-identical input, any divergence is a
parity bug, and every finding ships with the exact key path that reproduces
it.

**Runtime constraint (owner-stated):** the two stacks never run at the same
time. Each branch boots one target from a fresh seed copy, drives, captures,
kills, then does the same for the other target, and only then compares. All
runs happen in a dedicated tmux server (socket `parexp`) — your sessions are
never touched — with an offline RNS config (no interfaces, shared instance
off), so nothing reaches the live network or `~/.reticulum` /
`~/.nomadnetwork`.

## Usage

```bash
# Full depth-1 sweep of the default key universe (36 keys, ~10 min):
python3 tooling/explore/explore.py

# Focused subset, depth 2 (follows dialogs and tab switches):
python3 tooling/explore/explore.py --keys tab,enter,esc,C-n,Down,Up --depth 2 --max-branches 80

# One target only (seed generation still runs for both — skip the other side
# by reusing its seed via --out from an earlier run):
python3 tooling/explore/explore.py --list-keys
```

Artifacts land in `--out` (default `/tmp/explore-runs/<timestamp>`):
`report.txt` (per-branch MATCH/DIFF), `findings.json`, and
`findings/NN-<label>/` with `py.plain.txt`, `go.plain.txt`, the normalized
`text.diff` / `style.diff` / `disk.diff`, and `paths.txt` (every key path that
hits the same divergence — one bug class per finding).

## Key universe

`--list-keys` prints it. Defaults: arrows, Tab/BTab, Enter, Esc, Space,
Backspace, Home/End, PgUp/PgDn, F8, C-a/b/d/e/f/g/j/k/l/n/o/s/t/u/v/w/x/y and
the runes `a`, `x`, `1`. Excluded by default (with reasons in
`explore.py`): `C-c`/`C-q` (quit), `C-p` (My-LXMF QR renders the seed's own
identity), `C-r` (sync waits on a propagation node offline), `C-z` (SIGTSTP),
and terminal aliases (`C-i`, `C-m`, `C-h`). `--include-quit` restores the quit
keys; `--keys` overrides the whole universe.

## Seeds

Each target gets its own seed template (different identities — hash/timing
normalization absorbs that): a first boot writes the default config (clearing
the firstrun flag; colormode forced to 24bit so color values diff directly),
then the seed is augmented with an `ignored` destination (the blocked-peer
flows: Untrusted tab → "Show blocked" → Ctrl-X) and an empty conversations
dir. On-disk comparison covers `ignored`, `storage/conversations`,
`storage/rrc_hubs`; identity-dependent files (identity, directory, logfile,
lxmf caches) and the Go-only boot-time `storage/pages/index.mu` provisioning
are excluded by design.

## Phase 3: conversation/message seeding (BUILT)

`--seed-messages N` (default 3) seeds REAL encrypted LXMF conversations into
each target's seed via `seed_messages.py` (run under the parity interpreter —
the one that can import RNS/LXMF): an unread message, a read message from a
**trusted, named peer** (written straight into the msgpack directory file —
both implementations read Python's layout), and a sent/failed one. Cross-impl
interop is verified: Go decodes Python-packed LXMF envelopes and the directory
entry, and the boot frames match after hash/time normalization. The disk
digest normalizes message-file names (`<LXM#i>`) and peer dirs (`<PEER>`) so
counts and flags still compare.

`--start-keys` applies a key path before exploring (e.g.
`--start-keys up,right,enter`), and the state signature is derived from the
STYLED frame so focus/highlight moves count as real state changes.

**Depth-2 findings queue (open — reproducible via the artifacts; root causes
traced):**
- `enter` (any route) — **the highest-value bug**: the opened conversation
  renders the *failed/no-source* header (`⚠ just now | …`) instead of
  Python's two-line header (`⚠ ← Unknown Origin` then the timestamp line),
  plus the whole "identity keys are not known" banner is missing, and Go's
  `.index` cache is 1 byte vs Python's 345-byte index. **Root cause traced:**
  Python's `LXMessage.unpack_from_file` (LXMessage.py:825) parses the stored
  container DIRECTLY — `msgpack.unpackb(packed_payload)` with no keys needed
  (opportunistic payloads are plaintext in the container; signature stays
  unverified with reason SOURCE_UNKNOWN → "Unknown Origin"). Go's
  `lxmf.UnpackMessageFromFile(ts, f)` takes a transport and leaves `SourceHash`
  unset for Python-packed containers, so the header takes the `SourceHash ==
  nil` failed branch (message-header.go:80-82). Fix target:
  `go-reticulum/lxmf`'s unpack path must populate SourceHash/State/Method and
  signature status from Python's container layout without requiring keys.
- `C-e` — Peer Info dialog body text wraps differently (word-break class, same
  family as workflow C's URL-bar/wrap findings).
- `C-r` — "No trusted nodes found, cannot sync!" dialog: Python centers and
  pads the text; Go left-aligns, and the wrap of the continuation lines differs.
- `C-x` — delete-confirm dialog: Python titles it `?` with a two-line body
  ("Delete conversation with\n<name>\n"); Go's title/content differ.
- `esc` — sync-footer row width differs by 2 columns in the esc state.

**Findings queue — status after the P0–P3 fix session (2026-09-02):**

FIXED this session (all verified by re-running the exact paths):
- ✅ Stored Python-packed LXMF messages now DECODE in gonomadnet — the root
  cause was go-reticulum's strict `transport_encryption` type check rejecting
  Python's `packed_container` (which always emits the key, as `None` for a
  message that never went through a transport). Fixed in `../go-reticulum/lxmf`
  with a regression test; the no-source/failed header is gone.
- ✅ The opened conversation's message header, the identity-unknown banner
  (check_editor_allowed), and the `.index` cache (write-after-load, 345-byte
  structural parity) all match Python now.
- ✅ **Ctrl-J opens the selected conversation** like Python: urwid decodes
  byte 0x0A as Enter; the Go dispatcher normalizes KeyLF/KeyCtrlJ → KeyEnter.
- ✅ The delete-confirm dialog is now the list-slot overlay with Python's "?"
  title and centered two-line body (C-x MATCHES).
- ✅ The ingest dialog wiring (C-u MATCHES) and the sync dialog's centered
  no-nodes line + urwid-pre-wrapped explainer.

QUEUED (fine-grained fill/offset nuances of the urwid-vs-tview wrap/center
family — each reproducible via `tooling/explore/explore.py --keys …`):
- `ctrl+j` / `enter` (style-only): the banner region's blank-row fill and a
  1-row vertical offset (py's banner region is one row lower).
- `C-r`: the Close button's horizontal placement differs by 3 columns inside
  its 45%-weight column, plus the same 1-row offset.
- `C-e`: the Peer Info dialog body wraps/indents 1 column differently.

Also: the C-e dialog and the banner both reveal that the parity skill's
documented installed-package copy was the WRONG one of two trees — see the
corrected user-site note in the skill.

## What it finds (evidence so far — first full depth-1 sweep)
The very first full sweep (36 keys × both targets) surfaced **five real
divergences**, all triaged and closed the same session:

| Key path | Divergence | Outcome |
|---|---|---|
| (boot) | Go's "Last sync" row painted its background over the text run only; Python paints the full row width (attribute-only — invisible to text diffs, and rooted in the tview fork's `TextView` fill rule: `SetBackgroundColor` sets both the box AND text style, suppressing the fill) | **Fixed** — explicit `SetTextStyle` so the fill fires, plus row padding |
| `ctrl+u` | Ingest dialog used the generic DialogManager form ("Ingest LXM URI" / "URI:" / Save-Cancel, centered full-screen) while the correct parity slot-overlay dialog (`IngestURIDialog`, title "Ingest message URI", "URI : ", Ingest/Back) existed but was never wired | **Fixed** — `OnIngestURI` rewired to `IngestURIDialog` |
| `ctrl+u` | `URI : ` caption rendered in tview's default bright-yellow label color; Python uses the default attribute | **Fixed** — `SetLabelColor(tcell.ColorDefault)` |
| `ctrl+x` | Python 1.2.8 **crashes the whole app** with an empty conversations list (`'AttrMap' object has no attribute 'source_hash'` — the empty placeholder is not None) | **Accepted** — Go deliberately survives; ledgered |
| `ctrl+y` | Python 1.2.8 crashes (urwid `RuntimeError: stdin has been closed`) | **Accepted** — ledgered |
| `ctrl+n` | New Conversation radios: Python/urwid renders BOTH Untrusted and Unknown checked (`[True, True, False]` verified against the installed urwid) | **Accepted** — deliberate owner decision (fleet bug #7): exactly one pre-checked radio; ledgered |

## The triage ledger

`known_divergences.json` (checked in, next to this README) lists reviewed
divergences as `{key: "<key-path>|<kind>", paths, kind, reason}` entries. The
explorer prints them as `KNOWN (accepted)` and does not count them as
findings — the bug queue stays actionable. Add an entry (with the reason)
whenever a divergence is a deliberate owner decision or an upstream Python
crash; delete the entry if the decision is reversed.

## Future work (Phase 3)

- LXMF conversation/message seeding (cross-format message file synthesis) to
  reach receive/unread/failed/paper/attachment flows offline.
- Coverage steering from the Phase-1 keyspec table: prefer (state, key) pairs
  that exercise Python handlers not yet hit.
- Parallel A/B booting under isolated RNS configs (safe today, but the owner
  prefers strictly sequential runs).