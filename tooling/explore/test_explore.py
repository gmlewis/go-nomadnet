#!/usr/bin/env python3
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Tests for tooling/explore/explore.py — the differential explorer's pure
# logic: frame normalization (noise masking), key-universe construction and
# tmux key mapping, canonical state signatures, and the on-disk digest. These
# run without tmux or either TUI binary (fast; no boots).

import os
import sys

sys.path.insert(0, os.path.dirname(__file__))
import explore  # noqa: E402


def test_normalize_masks_hex_hashes():
    """Identity/destination hashes appear in <hex> and bare 32-hex forms; the
    two seeds have different identities, so both must mask to <HASH>."""
    assert explore.normalize_frame("<2a6105f57145860441a62fe3b2a1352c>") == "<HASH>"
    assert explore.normalize_frame(
        "Node announce received: hash=bbf3172fdf752ce1afc332ff44119a4f"
    ) == "Node announce received: hash=<HASH>"


def test_normalize_masks_relative_and_absolute_times():
    assert explore.normalize_frame("→ 4h ago |") == "→ <RELTIME> |"
    assert explore.normalize_frame("just now") == "<RELTIME>"
    assert explore.normalize_frame("2026-09-01 20:42:27") == "<TIME>"
    assert explore.normalize_frame("6 minutes ago") == "<RELTIME>"


def test_normalize_masks_tmp_paths_and_hops():
    assert explore.normalize_frame("via /tmp/sweep-rns-a1/config") == "via <TMPPATH>"
    assert explore.normalize_frame("via Local shared instance, 5 hops") == "via Local shared instance, <N> hops"


def test_normalize_preserves_real_content():
    # Real UI text must survive normalization untouched.
    line = "[C-x] Delete  [C-r] Sync  [C-n] New  [C-u] Ingest URI"
    assert explore.normalize_frame(line) == line


def test_trim_frame_drops_trailing_blank_lines():
    assert explore.trim_frame("a\nb\n\n\n") == "a\nb"
    assert explore.trim_frame("\n\n") == ""


def test_frame_signature_is_stable_under_noise():
    a = explore.frame_signature("Announced 3 seconds ago <2a6105f57145860441a62fe3b2a1352c>\n")
    b = explore.frame_signature("Announced 7 seconds ago <712ffbfdb82c7fe60d0c5fa163ad2955>\n")
    assert a == b, "different hashes/times must produce the same signature"


def test_frame_signature_distinguishes_states():
    a = explore.frame_signature("No untrusted conversations\n")
    b = explore.frame_signature("No trusted conversations\n")
    assert a != b


def test_default_universe_excludes_dangerous_and_noisy_keys():
    universe = explore.default_key_universe()
    for key in explore.EXCLUDED_KEYS:
        assert key not in universe, key
    # The quit pair returns only with include_quit.
    assert "ctrl+c" not in explore.default_key_universe(include_quit=False)
    assert "ctrl+c" in explore.default_key_universe(include_quit=True)
    assert "ctrl+q" in explore.default_key_universe(include_quit=True)


def test_universe_contains_the_reported_bug_keys():
    # The blocked-row flows must be reachable: Ctrl-X (delete/unblock),
    # arrows, Tab, and Space (Show blocked toggle) are all in the universe.
    universe = explore.default_key_universe()
    for key in ("ctrl+x", "tab", "space", "up", "down", "enter"):
        assert key in universe, key


def test_tmux_key_mapping():
    # Named keys map to tmux names (non-literal); ctrl+X to C-x; runes to
    # literal sends.
    assert explore.tmux_key_arg("up") == (["Up"], False)
    assert explore.tmux_key_arg("ctrl+x") == (["C-x"], False)
    assert explore.tmux_key_arg("shift+tab") == (["BTab"], False)
    assert explore.tmux_key_arg("rune:a") == (["a"], True)
    assert explore.tmux_key_arg("rune:1") == (["1"], True)
    assert explore.tmux_key_arg("space") == (["Space"], False)


def test_disk_digest_tracks_included_files_only():
    import hashlib
    import tempfile

    tmp = tempfile.mkdtemp(prefix="explore-test-")
    try:
        conv = os.path.join(tmp, "storage", "conversations", "a" * 32)
        os.makedirs(conv)
        with open(os.path.join(conv, "unread"), "w") as f:
            f.write("1")
        with open(os.path.join(tmp, "ignored"), "w") as f:
            f.write("bbf3172fdf752ce1afc332ff44119a4f\n")
        with open(os.path.join(tmp, "logfile"), "w") as f:
            f.write("never included\n")
        with open(os.path.join(tmp, "storage", "identity"), "wb") as f:
            f.write(b"\x01" * 64)

        digest = explore.disk_digest(tmp)
        joined = "\n".join(digest)
        # The peer dir (32-hex) is masked to <PEER>; the flag keeps its content.
        assert "storage/conversations/<PEER>/unread 1 " in joined
        assert "ignored" in joined
        assert "logfile" not in joined, "logfile must be excluded (timing noise)"
        assert "storage/identity" not in joined, "identity must be excluded (own-identity dependent)"
        # The digest entry carries the file's content hash.
        expect = hashlib.sha1(b"1").hexdigest()
        assert any(e.endswith(expect) for e in digest)
    finally:
        import shutil
        shutil.rmtree(tmp)


def test_disk_digest_is_order_stable():
    import tempfile
    tmp = tempfile.mkdtemp(prefix="explore-test-")
    try:
        os.makedirs(os.path.join(tmp, "storage", "conversations"))
        d1 = explore.disk_digest(tmp)
        d2 = explore.disk_digest(tmp)
        assert d1 == d2
    finally:
        import shutil
        shutil.rmtree(tmp)


def test_normalize_keeps_blocked_row_label():
    # The [blocked] row label must NOT be masked as noise (hash brackets are
    # angle-wrapped hex of real length).
    line = "× [blocked] <bbf3172fdf752ce1afc332ff44119a4f>  <bbf3172fdf752ce1afc332ff44119a4f>"
    norm = explore.normalize_frame(line)
    assert "[blocked]" in norm
    assert "<HASH>" in norm


if __name__ == "__main__":
    fns = [v for k, v in sorted(globals().items()) if k.startswith("test_") and callable(v)]
    passed = failed = 0
    for fn in fns:
        try:
            fn()
            print("PASS", fn.__name__)
            passed += 1
        except Exception as e:  # noqa: BLE001
            print("FAIL", fn.__name__, "->", type(e).__name__, str(e)[:140])
            failed += 1
    print(f"\n{passed} passed, {failed} failed")
    sys.exit(1 if failed else 0)