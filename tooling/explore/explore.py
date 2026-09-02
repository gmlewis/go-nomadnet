#!/usr/bin/env python3
# Differential TUI explorer for the nomadnet (Python source-of-truth) vs
# gonomadnet (Go port) parity project.
#
# From a deterministically seeded state, this tool explores the TUI state
# machine breadth-first: for every key in the universe it drives BOTH targets
# through the SAME key path and diffs the resulting screen frames (normalized
# for non-parity noise) and on-disk side effects. Because both implementations
# receive identical input from identical states, any divergence is
# definitionally a parity bug — and every finding comes with the exact key
# path that reproduces it.
#
# CONSTRAINT (owner-stated): the two stacks cannot run at the same time. The
# explorer therefore runs ONE target at a time in a DEDICATED tmux server
# (socket "parexp") — boot target A from seed S, drive, capture, kill; restore
# seed S; boot target B; drive; capture; kill; compare.
#
# Phase-2 scope: purely UI-level exploration (no live network). The RNS config
# has no interfaces, so both targets boot into an identical offline state.
# Conversation/message seeding is future work (needs cross-format LXMF file
# synthesis); the ignored-peer seed already reaches the blocked-row flows.
#
# Usage: python3 explore.py --help

import argparse
import difflib
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import time

TMUX_SOCK = "parexp"
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# Keys excluded from the default universe, with reasons:
#   ctrl+c / ctrl+q  — quit the app (ends the branch; --include-quit restores)
#   ctrl+p           — My LXMF QR dialog renders the OWN identity's QR; the two
#                      seeds have different identities, so the QR block pattern
#                      always differs (identity noise, not a port bug)
#   ctrl+r           — LXMF sync waits on a propagation node; with no
#                      interfaces the wait dominates the branch
#   ctrl+z           — SIGTSTP would suspend the app inside the pane
#   ctrl+i / ctrl+m / ctrl+h — terminal aliases of Tab/Enter/Backspace
EXCLUDED_KEYS = {
    "ctrl+c": "quits the app",
    "ctrl+q": "quits the app",
    "ctrl+p": "My-LXMF QR dialog renders the seed's own identity (identity noise)",
    "ctrl+r": "LXMF sync waits on a propagation node; offline seeds never satisfy it",
    "ctrl+z": "SIGTSTP suspends the app",
    "ctrl+i": "terminal alias of Tab",
    "ctrl+m": "terminal alias of Enter",
    "ctrl+h": "terminal alias of Backspace",
}

# tmux send-keys names for the canonical keys (runes are sent with -l).
TMUX_KEY_NAMES = {
    "up": "Up", "down": "Down", "left": "Left", "right": "Right",
    "tab": "Tab", "shift+tab": "BTab", "enter": "Enter", "esc": "Escape",
    "space": "Space", "backspace": "BSpace", "home": "Home", "end": "End",
    "pgup": "PgUp", "pgdn": "PgDn", "f8": "F8",
}

# On-disk locations compared for semantic side effects. Own-identity-dependent
# files (identity, directory, logfile, lxmf caches) are excluded: they differ
# by construction because the seeds have distinct identities. storage/pages is
# excluded too: the Go port provisions a default index.mu at boot (Python keeps
# its pages elsewhere and provisions nothing), a static boot-time difference,
# not key-driven state.
DISK_INCLUDE = ["ignored", "storage/conversations", "storage/rrc_hubs"]

# Non-parity noise masked before comparing frames. Superset of sweep.sh's
# rules: the seeds have different identities, so hashes and timings differ.
NOISE_RULES = [
    (re.compile(r"<[0-9a-fA-F]{16,}>"), "<HASH>"),
    (re.compile(r"\b[0-9a-fA-F]{32}\b"), "<HASH>"),
    (re.compile(r"(just now|yesterday|\d+ (?:seconds?|minutes?|hours?|days?|months?|years?) ago|\d+[smhdw] ago)"), "<RELTIME>"),
    (re.compile(r"\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}"), "<TIME>"),
    (re.compile(r"/tmp/[\w./-]+"), "<TMPPATH>"),
    (re.compile(r"\b\d+ hops\b"), "<N> hops"),
    (re.compile(r"\b\d+ peers\b"), "<N> peers"),
]


def normalize_frame(text):
    """Mask non-parity artifacts so frame diffs flag only genuine divergence."""
    for rx, repl in NOISE_RULES:
        text = rx.sub(repl, text)
    return text


def trim_frame(text):
    """Drop trailing blank lines (render-height artifacts)."""
    lines = text.split("\n")
    while lines and lines[-1].strip() == "":
        lines.pop()
    return "\n".join(lines)


def frame_signature(plain):
    """Canonical state signature: hash of the normalized, trimmed frame."""
    norm = normalize_frame(trim_frame(plain))
    return hashlib.sha1(norm.encode()).hexdigest()[:12]


def tmux_key_arg(key):
    """Map a canonical key to the (args, literal) form for tmux send-keys."""
    if key in TMUX_KEY_NAMES:
        return [TMUX_KEY_NAMES[key]], False
    if key.startswith("ctrl+") and len(key) > 5:
        return ["C-" + key[5:]], False
    if key.startswith("rune:"):
        return [key[5:]], True
    if len(key) == 1:
        return [key], True
    raise ValueError("unknown key: %r" % key)


def default_key_universe(include_quit=False):
    """The default explored key universe (canonical names, sorted)."""
    excluded = set(EXCLUDED_KEYS)
    if include_quit:
        excluded -= {"ctrl+c", "ctrl+q"}
    keys = list(TMUX_KEY_NAMES) + ["f8"]
    keys += ["ctrl+" + c for c in "abcdefghijklmnopqrstuvwxyz"]
    keys += ["rune:a", "rune:x", "rune:1"]
    return sorted(k for k in set(keys) if k not in excluded)


def tmux(*args):
    return subprocess.run(["tmux", "-L", TMUX_SOCK] + list(args),
                          capture_output=True, text=True)


def build_go_bin(path):
    print("==> building Go binary -> %s" % path)
    subprocess.run(["go", "build", "-o", path, "./cmd/gonomadnet"],
                   cwd=REPO_ROOT, check=True)
    return path


def make_isolated_rns(dirpath):
    """Write the no-interface RNS config (fully offline, shared-instance off)."""
    os.makedirs(dirpath, exist_ok=True)
    with open(os.path.join(dirpath, "config"), "w") as f:
        f.write("[reticulum]\n  share_instance = No\n  enable_transport = No\n\n"
                "[logging]\n  loglevel = 4\n\n[interfaces]\n")
    return dirpath


def capture_frame(sess, styled):
    flag = "-e" if styled else "-p"
    res = tmux("capture-pane", "-t", sess, flag, "-p")
    return res.stdout if res.returncode == 0 else ""


def wait_boot(sess, boot_s):
    """Poll until the menubar is up and the frame stops changing."""
    marker = "[ Conversations ]"
    last, stable = None, 0
    t0 = time.time()
    while time.time() - t0 < boot_s:
        frame = capture_frame(sess, styled=False)
        if "Conversations" in frame and "[ Quit ]" in frame:
            if frame == last:
                stable += 1
                if stable >= 2:
                    return frame, True
            else:
                stable = 0
        last = frame
        time.sleep(0.3)
    return last or "", False


def send_key(sess, key):
    argslist, literal = tmux_key_arg(key)
    if literal:
        tmux("send-keys", "-t", sess, "-l", argslist[0])
    else:
        tmux("send-keys", "-t", sess, argslist[0])


def disk_digest(workdir):
    """Semantic side-effect fingerprint of the included on-disk state."""
    out = []
    for rel in DISK_INCLUDE:
        base = os.path.join(workdir, rel)
        if os.path.isfile(base):
            paths = [rel]
        elif os.path.isdir(base):
            paths = []
            for root, _dirs, files in os.walk(base):
                for f in sorted(files):
                    paths.append(os.path.relpath(os.path.join(root, f), workdir))
        else:
            continue
        for p in sorted(paths):
            full = os.path.join(workdir, p)
            try:
                with open(full, "rb") as fh:
                    data = fh.read()
            except OSError:
                continue
            out.append("%s %d %s" % (p, len(data), hashlib.sha1(data).hexdigest()))
    return sorted(out)


def boot_cmd(target, workdir, rnsdir, gobin, pybin):
    if target == "go":
        return "%s -t -config %s -rnsconfig %s" % (gobin, workdir, rnsdir)
    return "%s -t --config %s --rnsconfig %s" % (pybin, workdir, rnsdir)


def run_branch(target, path, args, size, workdir, rnsdir, gobin, pybin, tag):
    """Boot the target in the given work dir, drive the key path, capture the
    final frames + disk digest, and kill the session. Returns a result dict."""
    w, h = size
    sess = "exp-%s" % tag
    tmux("kill-session", "-t", sess)
    cmd = boot_cmd(target, workdir, rnsdir, gobin, pybin)
    tmux_cmd = ("env -u NO_COLOR TERM=xterm-256color COLORTERM=truecolor "
                "TCELL_TRUECOLOR=1 %s; echo __EXIT=$?; sleep 300" % cmd)
    subprocess.run(["tmux", "-L", TMUX_SOCK, "new-session", "-d",
                    "-x", str(w), "-y", str(h), "-s", sess, tmux_cmd],
                   capture_output=True, text=True)
    try:
        frame, booted = wait_boot(sess, args.boot)
        if not booted:
            return {"booted": False, "plain": frame, "styled": "", "digest": []}
        for key in path:
            send_key(sess, key)
            time.sleep(args.settle)
        plain = capture_frame(sess, styled=False)
        styled = capture_frame(sess, styled=True)
        digest = disk_digest(workdir)
        return {"booted": True, "plain": plain, "styled": styled,
                "digest": digest, "workdir": workdir}
    finally:
        tmux("kill-session", "-t", sess)


def unified_diff(a_name, a_lines, b_name, b_lines):
    return "\n".join(difflib.unified_diff(
        a_lines, b_lines, fromfile=a_name, tofile=b_name, lineterm=""))


def ensure_tmux_server(args, size):
    """Start a dedicated tmux server (separate socket) with RGB passthrough."""
    w, h = size
    conf = os.path.join(args.out, "tmux.conf")
    with open(conf, "w") as f:
        f.write("terminal-features RGB\n")
    subprocess.run(["tmux", "-L", TMUX_SOCK, "-f", conf, "new-session", "-d",
                    "-x", str(w), "-y", str(h), "-s", "parexp-init", "sleep 300"],
                   capture_output=True)
    # Clean env so panes never inherit NO_COLOR (the agent-environment trap).
    tmux("set-environment", "-g", "-u", "NO_COLOR")


def raw_boot(target, args, size, workdir, rnsdir, gobin, pybin, tag, hold_s=8.0):
    """Boot the target in the given dir for a fixed hold (no marker wait) and
    kill it. Used for the seed-generation boot, where a fresh config enters
    the first-run flow and the menubar never appears."""
    w, h = size
    sess = "exp-%s" % tag
    tmux("kill-session", "-t", sess)
    cmd = boot_cmd(target, workdir, rnsdir, gobin, pybin)
    tmux_cmd = ("env -u NO_COLOR TERM=xterm-256color COLORTERM=truecolor "
                "TCELL_TRUECOLOR=1 %s; echo __EXIT=$?; sleep 300" % cmd)
    subprocess.run(["tmux", "-L", TMUX_SOCK, "new-session", "-d",
                    "-x", str(w), "-y", str(h), "-s", sess, tmux_cmd],
                   capture_output=True, text=True)
    time.sleep(hold_s)
    tmux("kill-session", "-t", sess)
    return os.path.isdir(workdir)


def generate_seed(target, args, size, rnsdir, gobin, pybin):
    """Boot the target once in its seed dir so it writes its default config
    (clearing the first-run flag), then augment with the ignored-peer seed and
    validate that the SECOND boot reaches the normal (non-modal) UI."""
    seeddir = os.path.join(args.out, "seed-%s" % target)
    if os.path.exists(seeddir):
        shutil.rmtree(seeddir)
    os.makedirs(seeddir)
    if not raw_boot(target, args, size, seeddir, rnsdir, gobin, pybin,
                    tag="seedgen-%s" % target):
        sys.exit("seed boot failed for %s: %s/config was not written"
                 % (target, seeddir))
    cfg = os.path.join(seeddir, "config")
    # Force 24-bit colormode so color values diff directly across targets
    # (the known colormode caveat; both ports honor this knob).
    text = open(cfg).read()
    if "colormode" in text:
        text = re.sub(r"(?m)^( *)colormode *=.*$", r"\1colormode = 24bit", text)
    with open(cfg, "w") as f:
        f.write(text)
    with open(os.path.join(seeddir, "ignored"), "w") as f:
        f.write("bbf3172fdf752ce1afc332ff44119a4f\n")
    os.makedirs(os.path.join(seeddir, "storage", "conversations"), exist_ok=True)
    # Validate the seeded state actually boots to the standard UI.
    res = run_branch(target, [], args, size, seeddir, rnsdir, gobin, pybin,
                     tag="seedchk-%s" % target)
    if not res.get("booted"):
        sys.exit("seed validation boot failed for %s (frame tail: %r)"
                 % (target, res.get("plain", "")[-200:]))
    return seeddir


def fresh_workdir(args, target, seeddir, tag):
    work = os.path.join(args.out, "work", "%s-%s" % (tag, target))
    if os.path.exists(work):
        shutil.rmtree(work)
    shutil.copytree(seeddir, work)
    return work


def save_finding(outdir, idx, label, path, results, diffs):
    fdir = os.path.join(outdir, "findings", "%02d-%s" % (idx, label[:40].replace(" ", "_")))
    os.makedirs(fdir, exist_ok=True)
    with open(os.path.join(fdir, "path.txt"), "w") as f:
        f.write(label + "\n")
    for target, res in results.items():
        with open(os.path.join(fdir, "%s.plain.txt" % target), "w") as f:
            f.write(results[target].get("plain", ""))
        with open(os.path.join(fdir, "%s.styled.txt" % target), "w") as f:
            f.write(results[target].get("styled", ""))
    for name, text in diffs.items():
        with open(os.path.join(fdir, name), "w") as f:
            f.write(text)
    return fdir


def main():
    ap = argparse.ArgumentParser(description="Differential TUI explorer (py vs go)")
    ap.add_argument("--targets", default="py,go", help="comma list: py,go")
    ap.add_argument("--keys", default="", help="comma list of keys (default: full universe)")
    ap.add_argument("--depth", type=int, default=1, help="max key-path depth to explore")
    ap.add_argument("--max-branches", type=int, default=60, help="max A/B branch pairs")
    ap.add_argument("--size", default="100x30", help="WxH of the tmux pane")
    ap.add_argument("--boot", type=float, default=25.0, help="per-boot wait cap (s)")
    ap.add_argument("--settle", type=float, default=0.6, help="settle after each key (s)")
    ap.add_argument("--out", default="", help="artifacts dir (default /tmp/explore-runs/<ts>)")
    ap.add_argument("--go-bin", default="", help="prebuilt gonomadnet binary")
    ap.add_argument("--py-exe", default="", help="path to the nomadnet executable")
    ap.add_argument("--include-quit", action="store_true",
                    help="add ctrl+c/ctrl+q (quit) to the key universe")
    ap.add_argument("--list-keys", action="store_true", help="print the key universe and exit")
    args = ap.parse_args()

    if args.list_keys:
        print("\n".join(default_key_universe(include_quit=args.include_quit)))
        return

    args.targets = [t.strip() for t in args.targets.split(",") if t.strip()]
    if len(args.targets) != 2:
        sys.exit("--targets needs exactly two of: py,go (the explorer diffs a pair)")
    try:
        size = tuple(int(x) for x in args.size.split("x"))
    except ValueError:
        sys.exit("bad --size %r (want WxH)" % args.size)
    if not args.out:
        args.out = "/tmp/explore-runs/%d" % int(time.time())
    os.makedirs(args.out, exist_ok=True)

    gobin = args.go_bin or "/tmp/gonomadnet-explore-bin"
    if "go" in args.targets and not os.path.exists(gobin):
        build_go_bin(gobin)
    pybin = args.py_exe or shutil.which("nomadnet")
    if "py" in args.targets and not pybin:
        sys.exit("nomadnet not found on PATH (install it or pass --py-exe)")

    universe = default_key_universe(include_quit=args.include_quit)
    if args.keys:
        universe = [k.strip() for k in args.keys.split(",") if k.strip()]

    # Triage ledger: reviewed, accepted divergences (deliberate deviations and
    # Python-side crashes) — new divergences count as findings, these do not.
    known = {}
    ledger = os.path.join(os.path.dirname(os.path.abspath(__file__)), "known_divergences.json")
    if os.path.exists(ledger):
        with open(ledger) as f:
            for entry in json.load(f).get("entries", []):
                for p in entry.get("paths", [entry["key"].split("|")[0]]):
                    known[p + "|" + entry["kind"]] = entry["reason"]

    ensure_tmux_server(args, size)

    rnsdir = os.path.join(args.out, "rns")
    make_isolated_rns(rnsdir)
    seeds = {}
    for target in args.targets:
        print("==> generating %s seed" % target)
        seeds[target] = generate_seed(target, args, size, rnsdir, gobin, pybin)
    print("==> exploring: universe=%d keys, depth=%d, cap=%d branches"
          % (len(universe), args.depth, args.max_branches))

    # Worklist: BFS over key paths. A branch = one key path run on BOTH
    # targets (sequentially, per the can't-run-both constraint). Matching
    # branches expand; divergent ones do not (the targets are in different
    # states from there on, so extension would be meaningless). No-op keys
    # (final signature == the signature they were tried from) add no coverage
    # and are not expanded.
    queue = [([], None)]
    tried = set()
    branches, findings = 0, []
    finding_classes = {}
    report = []
    while queue and branches < args.max_branches:
        path, parent_sig = queue.pop(0)
        tag = "b%03d" % branches
        if path:
            tag += "-" + "-".join(re.sub(r"[^a-z0-9]", "", k) for k in path)
        branches += 1
        label = "boot" if not path else " ".join(path)

        results = {}
        for target in args.targets:
            work = fresh_workdir(args, target, seeds[target], tag)
            results[target] = run_branch(target, path, args, size, work,
                                         rnsdir, gobin, pybin, tag)
        a, b = (results[t] for t in args.targets)
        if not a.get("booted") or not b.get("booted"):
            diffs = {"boot.diff": "py booted=%s\ngo booted=%s" % (a.get("booted"), b.get("booted"))}
            fdir = save_finding(args.out, len(findings), label, path, results, diffs)
            findings.append({"path": path, "kind": "boot-failure", "dir": fdir})
            report.append("DIFF  %-40s BOOT FAILURE" % label)
            continue

        a_trim, b_trim = trim_frame(a["plain"]), trim_frame(b["plain"])
        text_diff = unified_diff(
            "py", normalize_frame(a_trim).split("\n"),
            "go", normalize_frame(b_trim).split("\n"))
        style_diff = unified_diff(
            "py", normalize_frame(trim_frame(a["styled"])).split("\n"),
            "go", normalize_frame(trim_frame(b["styled"])).split("\n"))
        ad, bd = set(a["digest"]), set(b["digest"])
        disk_diff = ""
        if ad != bd:
            disk_diff = "\n".join(
                ["--- py disk", "+++ go disk"]
                + ["-" + x for x in sorted(ad - bd)] + ["+" + x for x in sorted(bd - ad)])

        sig = frame_signature(a["plain"])
        if text_diff or disk_diff or style_diff:
            kinds = [k for k, v in (("text", text_diff), ("style", style_diff), ("disk", disk_diff)) if v]
            kind = "+".join(kinds)
            # Triage ledger: reviewed divergences (deliberate deviations,
            # owner-approved) are reported as KNOWN and do not count as bugs.
            ledger_key = label + "|" + kind
            if ledger_key in known:
                report.append("KNOWN %-50s %s  (%s)" % (label, kind, known[ledger_key][:60]))
                print("  [%s] KNOWN (accepted) %s" % (label, kind))
                continue
            diffs = {}
            if text_diff:
                diffs["text.diff"] = text_diff
            if style_diff:
                diffs["style.diff"] = style_diff  # may be the ONLY divergence
            if disk_diff:
                diffs["disk.diff"] = "--- py disk\n+++ go disk\n" + "\n".join(
                    ["-" + x for x in sorted(ad - bd)] + ["+" + x for x in sorted(bd - ad)])
            # Dedup identical divergences: the same underlying bug fires on
            # every screen that shows it (e.g. an attribute-only artifact on a
            # persistent row), so one finding class lists all its key paths.
            diff_key = hashlib.sha1(
                (text_diff + "\x00" + style_diff + "\x00" + disk_diff).encode()
            ).hexdigest()[:12]
            if diff_key in finding_classes:
                f = finding_classes[diff_key]
                f["paths"].append(label)
                with open(os.path.join(f["dir"], "paths.txt"), "a") as f:
                    f.write(label + "\n")
                report.append("DIFF  %-50s %s  (dup of finding %s)" % (label, kind, f["id"]))
                print("  [%s] DIFF %s (dup of finding %s)" % (label, kind, f["id"]))
                continue
            fid = "%s" % diff_key[:6]
            fdir = save_finding(args.out, len(findings), label, path, results, diffs)
            finding_classes[diff_key] = {"id": fid, "kind": kind, "dir": fdir, "paths": [label]}
            findings.append({"id": fid, "path": path, "kind": kind, "dir": fdir})
            report.append("DIFF  %-50s %s  -> %s" % (label, kind, fdir))
            print("  [%s] DIFF %s -> %s" % (label, kind, fdir))
            continue

        report.append("MATCH %-47s sig=%s" % (label, sig))
        print("  [%s] MATCH sig=%s" % (label, sig))
        # Expand only from states where the key DID something (a no-op leads
        # back to the same state, adding no coverage).
        if path and parent_sig is not None and sig == parent_sig:
            continue
        if len(path) < args.depth:
            for k in universe:
                if (sig, k) not in tried:
                    tried.add((sig, k))
                    queue.append((path + [k], sig))

    print()
    print("=== SUMMARY: %d branches, %d findings ===" % (branches, len(findings)))
    for line in report:
        print(line)
    with open(os.path.join(args.out, "report.txt"), "w") as f:
        f.write("\n".join(report) + "\n")
    with open(os.path.join(args.out, "findings.json"), "w") as f:
        json.dump(findings, f, indent=1)
    if findings:
        print("\n%d finding(s) — artifacts under %s/findings/" % (len(findings), args.out))


if __name__ == "__main__":
    main()