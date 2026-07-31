# micron-parity — Micron parser golden-value harnesses

This directory contains Python extraction harnesses for verifying the Go port
(`nomadnet/micron`) against the **source-of-truth Python
`nomadnet/ui/textui/MicronParser.py`** at the parsing layer.

## Why this exists

`MicronParser.py` cannot be imported on hosts where **urwid does not import**
(this macOS is one — missing GLib). Anything urwid-coupled —
`markup_to_attrmaps`, `make_style`, `parse_line`'s widget construction, and
`render_table` (via `MarkdownToMicron`) — cannot run directly.

The harnesses here copy the **pure parsing logic verbatim** out of
`MicronParser.py` and stub only the urwid/app-dependent helpers, so the
*structural* output (text segmentation, links, fields, headings, dividers,
section-resets, partials, table config) can be captured as JSON golden values
and encoded into Go table-driven tests — no urwid required.

## Prerequisites

- `python3` (stdlib only — no third-party packages).
- Access to the READ-ONLY original source at
  `/Users/glenn/src/github.com/markqvist/nomadnet` (only needed if you want to
  re-extract/audit the copied functions against upstream).

## Files

| File | Captures | Source lines |
|---|---|---|
| `micron_inline.py` | `make_output` inline part list: text spans, link specs, field dicts. | MicronParser.py:593-855 (+ default_state 39-68) |
| `micron_parseline.py` | `parse_line` line classification: heading level, divider char, comment, section-reset, table start/end config, partial. Depends on `micron_inline`. | MicronParser.py:220-416 |

Both scripts read fixture lines from **stdin** (one micron markup line per
line) and emit a JSON array of `{"input": ..., "output": [...]}` objects.

## Quick start

```bash
cd tooling/micron-parity

# Inline (text/link/field) structure for a few markup lines:
printf '`[Label`url]\n`<field`data>\nplain text\n' | python3 micron_inline.py

# Line-level (heading/divider/comment/section-reset/partial/table) structure:
printf '>Heading\n>>Deeper\n-\n# comment\n<reset\n`{partial_url`5`f1|f2}\n' \
    | python3 micron_parseline.py
```

## Parity workflow

The recommended loop while porting a micron parsing feature:

1. **Decide the cases** — the markup lines that exercise the feature (e.g.
   field type-indicators, link component counts, heading sanitization).
2. **Capture the Python golden values:**

   ```bash
   printf '`<?checkbox`name|label>\n`[^radio`name|value|label|*>\n' \
       | python3 micron_inline.py
   ```

3. **Encode them** in a Go table-driven test (see
   `nomadnet/micron/field-parity_test.go`, `link-parity_test.go`,
   `heading-parity_test.go`, `linelevel-parity_test.go`, `escape-parity_test.go`
   for the established pattern).
4. **Implement** the Go parser until the test passes.
5. **Run the suite green:**

   ```bash
   GOCACHE=/tmp/go-cache go test ./...
   ```

## What is and isn't comparable

These harnesses capture the **parsing/structure layer**. They deliberately do
not capture styling, because Python expresses it as styled text spans / state
mutations while Go emits discrete AST nodes (`NodeBold`, `NodeColor`,
`NodeAlign`, `NodeAnchor`, `NodeLiteral`). Comparable at the AST layer:

- text segmentation & escapes
- links (url / label / fields / component-count rules)
- fields (name / type / width / masked / data / value / label / prechecked)
- headings (level + text; level is **unbounded**, not clamped)
- dividers (char), comments (no node), section-reset (depth 0 + text)
- partials (url / refresh / fields / id), table align/maxwidth config

**Not comparable here** (rendering-layer, urwid-blocked): bold/underline/italic
toggles, color application, alignment, anchors, literal-mode toggles, and the
table *row* syntax (Python uses markdown `|cell|cell|` via `MarkdownToMicron`,
which is RNS-dependent and hard to exercise headlessly).

A lone `` `=``, `` `c ``, or `` `:anchor `` produces **no output part** in
Python — that is a model difference, not a parity bug.

## Re-auditing against upstream

The copied functions are verbatim snapshots. If you suspect drift, diff the
region against the original:

```bash
diff <(sed -n '593,855p' /Users/glenn/src/github.com/markqvist/nomadnet/nomadnet/ui/textui/MicronParser.py) \
     <(sed -n '/^def make_output/,/^def serialize/p' micron_inline.py)
```

(The harness version adds stub helpers and a `convert`/`run` wrapper; the
`make_output` body itself should match.)