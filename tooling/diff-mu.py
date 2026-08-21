#!/usr/bin/env python3
# diff-mu.py — structured comparator for the logical-line parity JSON schema.
#
# Diffs the JSON documents emitted by `cmd/view-mu -json` (Go) and
# `tooling/render-mu.py --json` (Python) field by field, surfacing every
# rendering divergence: line count, align/indent/heading_level/divider/anchor,
# per-span text/fg/bg/bold/underline/italic, link presence+label+url+fields,
# and field widget name/type/width/masked/data/value/prechecked.
#
# Usage:
#   python3 tooling/diff-mu.py go.json py.json [--json] [--tolerate-color-forms]
#
# Diagnostics go to stderr; the human-readable report (or --json findings) to
# stdout. Exits nonzero on any diff so it is CI-friendly.

import argparse
import json
import sys


def _load(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


# Colors may legitimately round-trip differently between emitters (e.g. "g56"
# vs "#rrggbb"). When --tolerate-color-forms is set, compare colors only by
# their normalized hex value.
def _norm_color(c, tolerate):
    if not tolerate or not isinstance(c, str):
        return c
    if c == "default" or c is None:
        return "default"
    s = c.lstrip("#").lower()
    if s.startswith("g") and len(s) == 3:
        return s  # grayscale gNN — leave as-is
    if len(s) == 3:
        return "".join(ch * 2 for ch in s)
    return s


def _color_eq(a, b, tolerate):
    if not tolerate:
        return a == b
    return _norm_color(a, True) == _norm_color(b, True)


def _span_eq(a, b, tolerate):
    if a.get("text") != b.get("text"):
        return False
    for k in ("bold", "underline", "italic"):
        if bool(a.get(k)) != bool(b.get(k)):
            return False
    for k in ("fg", "bg"):
        if not _color_eq(a.get(k), b.get(k), tolerate):
            return False
    la, lb = a.get("link"), b.get("link")
    if (la is None) != (lb is None):
        return False
    if la is not None:
        for k in ("label", "url", "fields"):
            if la.get(k) != lb.get(k):
                return False
    fa, fb = a.get("field"), b.get("field")
    if (fa is None) != (fb is None):
        return False
    if fa is not None:
        for k in ("name", "type", "width", "masked", "prechecked", "data",
                  "value"):
            if fa.get(k) != fb.get(k):
                return False
    return True


def diff_docs(go, py, tolerate=False):
    findings = []
    gl, pl = go.get("lines", []), py.get("lines", [])
    n = max(len(gl), len(pl))
    if len(gl) != len(pl):
        findings.append({
            "kind": "line_count",
            "go": len(gl),
            "py": len(pl),
            "detail": "line count differs",
        })
    for i in range(n):
        g = gl[i] if i < len(gl) else None
        p = pl[i] if i < len(pl) else None
        if g is None:
            findings.append({"kind": "missing_in_go", "index": i,
                             "py_line": p})
            continue
        if p is None:
            findings.append({"kind": "missing_in_py", "index": i,
                             "go_line": g})
            continue
        for k in ("align", "indent", "heading_level", "divider", "anchor"):
            if g.get(k) != p.get(k):
                findings.append({
                    "kind": "line_attr", "index": i, "field": k,
                    "go": g.get(k), "py": p.get(k),
                })
        # divider_char/divider_right are optional; only compare when present.
        for k in ("divider_char", "divider_right"):
            gv, pv = g.get(k), p.get(k)
            if gv != pv:
                findings.append({
                    "kind": "line_attr", "index": i, "field": k,
                    "go": gv, "py": pv,
                })
        gs, ps = g.get("spans", []), p.get("spans", [])
        if len(gs) != len(ps):
            findings.append({
                "kind": "span_count", "index": i,
                "go": len(gs), "py": len(ps),
            })
            # Still diff the overlapping prefix to report the first real diff.
        for j in range(min(len(gs), len(ps))):
            if not _span_eq(gs[j], ps[j], tolerate):
                findings.append({
                    "kind": "span", "index": i, "span": j,
                    "go": gs[j], "py": ps[j],
                })
    return findings


def _fmt_finding(f):
    k = f.get("kind")
    if k == "line_count":
        return "line count: go=%s py=%s" % (f["go"], f["py"])
    if k in ("missing_in_go", "missing_in_py"):
        return "line %s: %s" % (f["index"], k)
    if k == "line_attr":
        return "line %s %s: go=%r py=%r" % (f["index"], f["field"],
                                            f["go"], f["py"])
    if k == "span_count":
        return "line %s span count: go=%s py=%s" % (f["index"], f["go"],
                                                    f["py"])
    if k == "span":
        g, p = f["go"], f["py"]
        fields = []
        for key in ("text", "fg", "bg", "bold", "underline", "italic"):
            if g.get(key) != p.get(key):
                fields.append("%s go=%r py=%r" % (key, g.get(key),
                                                 p.get(key)))
        gl, pl = g.get("link"), p.get("link")
        if (gl is None) != (pl is None):
            fields.append("link go=%r py=%r" % (gl, pl))
        elif gl is not None:
            for key in ("label", "url", "fields"):
                if gl.get(key) != pl.get(key):
                    fields.append("link.%s go=%r py=%r" %
                                  (key, gl.get(key), pl.get(key)))
        gf, pf = g.get("field"), p.get("field")
        if (gf is None) != (pf is None):
            fields.append("field go=%r py=%r" % (gf, pf))
        elif gf is not None:
            for key in ("name", "type", "width", "masked", "prechecked",
                        "data", "value"):
                if gf.get(key) != pf.get(key):
                    fields.append("field.%s go=%r py=%r" %
                                  (key, gf.get(key), pf.get(key)))
        return "line %s span %s: %s" % (f["index"], f["span"],
                                        "; ".join(fields) or "differs")
    return json.dumps(f)


def main():
    ap = argparse.ArgumentParser(
        description="Diff Go vs Python logical-line parity JSON.")
    ap.add_argument("go", help="Go view-mu -json document")
    ap.add_argument("py", help="Python render-mu.py --json document")
    ap.add_argument("--json", action="store_true",
                    help="emit findings as JSON instead of text")
    ap.add_argument("--tolerate-color-forms", action="store_true",
                    help="treat gNN and #rrggbb forms of the same color as equal")
    args = ap.parse_args()

    go = _load(args.go)
    py = _load(args.py)
    findings = diff_docs(go, py, tolerate=args.tolerate_color_forms)

    if args.json:
        json.dump({"findings": findings,
                   "count": len(findings)}, sys.stdout, indent=2,
                  ensure_ascii=False)
        sys.stdout.write("\n")
    else:
        for f in findings:
            sys.stdout.write(_fmt_finding(f) + "\n")
        if not findings:
            sys.stdout.write("no differences\n")
    sys.exit(1 if findings else 0)


if __name__ == "__main__":
    main()