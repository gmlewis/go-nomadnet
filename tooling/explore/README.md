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