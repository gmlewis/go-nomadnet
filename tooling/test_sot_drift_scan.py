#!/usr/bin/env python3
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program. If not, see <https://www.gnu.org/licenses/>.
"""Tests for tooling/sot_drift_scan.py.

These pin the two properties the scanner exists for, both of which a naive
implementation gets wrong:

  1. Python's adjacent string literals must be folded before comparison, so a
     message the reference spread over several quoted fragments is compared as
     the single string a caller would see.
  2. Matching must be VERBATIM, and must tolerate the Go port breaking the same
     text at different points with `+`. A fuzzy (n-gram) match reports a long
     block that merely resembles the old release — which is how the scan first
     flagged the default-config blob, a different finding entirely.

The regression these tests guard: the identity-unknown banner was transcribed
from nomadnet 1.2.3, 1.2.8 reworded it, and because a test asserted the copied
literal it enforced the superseded wording instead of catching it.
"""

import os
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, os.path.dirname(__file__))
from sot_drift_scan import (  # noqa: E402
    as_package_dir,
    decode_go_interpreted,
    go_folded_strings,
    main,
    scan,
    superseded_literals,
)

# The real divergence, reduced: 1.2.3 said "should arrive shortly… Close this
# conversation and reopen it to try again."; 1.2.8 replaced it.
STALE_PY = '''
MSG = ("You cannot currently message this peer, since its identity keys are not known. "
       "The keys have been requested from the network and should arrive shortly, if available. "
       "Close this conversation and reopen it to try again.\\n")
'''

SOT_PY = '''
MSG = ("You cannot currently message this peer, since its identity keys are not known. "
       "The keys have been requested from the network, and you will be able to send messages "
       "as soon as they arrive.\\n")
'''

# Go spreads the same paragraph over two literals joined with `+`.
GO_STALE = """package x

const banner = "\\n\\nYou cannot currently message this peer, since its identity keys are not known. " +
\t"The keys have been requested from the network and should arrive shortly, if available. " +
\t"Close this conversation and reopen it to try again.\\n"
"""

GO_SOT = """package x

const banner = "\\n\\nYou cannot currently message this peer, since its identity keys are not known. " +
\t"The keys have been requested from the network, and you will be able to send messages " +
\t"as soon as they arrive.\\n"
"""

STALE_SENTENCE = (
    "The keys have been requested from the network and should arrive shortly, if available. "
    "Close this conversation and reopen it to try again."
)


def _write(root, rel, content):
    path = Path(root) / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    return path


def _trees(tmp, stale_py=STALE_PY, sot_py=SOT_PY, go=GO_SOT):
    """Build a stale tree, a SOT tree and a Go tree under tmp; return their paths."""
    stale, sot, go_root = Path(tmp, "stale"), Path(tmp, "sot"), Path(tmp, "go")
    _write(stale, "_version.py", '__version__ = "1.2.3"\n')
    _write(sot, "_version.py", '__version__ = "1.2.8"\n')
    _write(stale, "ui/textui/Conversations.py", stale_py)
    _write(sot, "ui/textui/Conversations.py", sot_py)
    if go is not None:
        _write(go_root, "tui/widget.go", go)
    return stale, sot, go_root


def test_decode_go_interpreted_handles_message_escapes():
    """The escapes that actually occur in reference prose, incl. the hex form."""
    assert decode_go_interpreted("a\\nb") == "a\nb"
    assert decode_go_interpreted("quote\\\" and back\\\\slash") == 'quote" and back\\slash'
    assert decode_go_interpreted("\\x41\\u00e9") == "Aé"


def test_go_folded_strings_joins_plus_concatenation():
    """A paragraph split over `+`-joined literals reads as one string."""
    folded = go_folded_strings(GO_SOT)
    assert any("as soon as they arrive." in chunk for chunk in folded), folded


def test_go_folded_strings_handles_raw_literals():
    """Raw (backtick) literals are taken verbatim and not escape-decoded."""
    folded = go_folded_strings("package x\n\nvar s = `line one with a \\n kept literal`\n")
    assert "line one with a \\n kept literal" in folded


def test_superseded_literals_folds_python_adjacent_strings():
    """The candidate is the whole FOLDED paragraph, not one of its fragments."""
    with tempfile.TemporaryDirectory() as tmp:
        stale, sot, _ = _trees(tmp)
        superseded = superseded_literals(stale, sot, min_len=20)
    containing = [value for value in superseded if STALE_SENTENCE in value]
    assert len(containing) == 1, superseded
    # Folded across all three Python fragments, so it also carries the opening
    # sentence that only the first fragment holds.
    assert containing[0].startswith("You cannot currently message this peer")
    assert superseded[containing[0]] == "ui/textui/Conversations.py"


def test_scan_flags_frozen_stale_text():
    """The regression case: stale wording still present in Go is reported."""
    with tempfile.TemporaryDirectory() as tmp:
        stale, sot, go_root = _trees(tmp, go=GO_STALE)
        superseded = superseded_literals(stale, sot, min_len=20)
        hits = scan(go_root, superseded)
    assert len(hits) == 1
    go_rel, stale_rel, value = hits[0]
    assert go_rel == "tui/widget.go"
    assert stale_rel == "ui/textui/Conversations.py"
    assert STALE_SENTENCE in value


def test_scan_is_quiet_when_the_port_was_updated():
    """Correctly reworded Go text is not reported."""
    with tempfile.TemporaryDirectory() as tmp:
        stale, sot, go_root = _trees(tmp, go=GO_SOT)
        superseded = superseded_literals(stale, sot, min_len=20)
        hits = scan(go_root, superseded)
    assert hits == []


def test_scan_requires_a_verbatim_match():
    """Text that merely resembles the old release must not be reported.

    This is the property that separates "the port froze this sentence" from "the
    port holds a long block resembling the old one while missing what the SOT
    added" — the second is a real but different finding, and fuzzy matching
    conflates them.
    """
    near = GO_STALE.replace("should arrive shortly", "should arrive very shortly")
    with tempfile.TemporaryDirectory() as tmp:
        stale, sot, go_root = _trees(tmp, go=near)
        superseded = superseded_literals(stale, sot, min_len=20)
        hits = scan(go_root, superseded)
    assert hits == []


def test_scan_ignores_a_literal_the_go_tree_never_had():
    """A superseded literal absent from Go is not drift."""
    with tempfile.TemporaryDirectory() as tmp:
        stale, sot, go_root = _trees(tmp, go="package x\n\nconst other = \"unrelated text here\"\n")
        superseded = superseded_literals(stale, sot, min_len=20)
        hits = scan(go_root, superseded)
    assert hits == []


def test_as_package_dir_accepts_both_layouts():
    """An installed package dir and a checkout root both resolve."""
    with tempfile.TemporaryDirectory() as tmp:
        installed = _write(tmp, "pkg/_version.py", "x")
        checkout = _write(tmp, "repo/nomadnet/_version.py", "x")
        assert as_package_dir(installed.parent) == installed.parent
        assert as_package_dir(Path(tmp, "repo")) == checkout.parent
        assert as_package_dir(Path(tmp, "absent")) is None


def test_main_reports_drift_with_exit_1():
    with tempfile.TemporaryDirectory() as tmp:
        stale, sot, go_root = _trees(tmp, go=GO_STALE)
        rc = main(["--stale", str(stale), "--sot", str(sot), "--go", str(go_root)])
    assert rc == 1


def test_main_reports_clean_with_exit_0():
    with tempfile.TemporaryDirectory() as tmp:
        stale, sot, go_root = _trees(tmp, go=GO_SOT)
        rc = main(["--stale", str(stale), "--sot", str(sot), "--go", str(go_root)])
    assert rc == 0


def test_main_skips_when_the_stale_tree_is_absent():
    """No checkout to compare against is not drift, and not a crash."""
    with tempfile.TemporaryDirectory() as tmp:
        _, sot, go_root = _trees(tmp)
        rc = main(["--stale", str(Path(tmp, "nope")), "--sot", str(sot), "--go", str(go_root)])
    assert rc == 0


def test_main_errors_with_exit_2_when_the_sot_is_missing():
    with tempfile.TemporaryDirectory() as tmp:
        stale, _, go_root = _trees(tmp)
        rc = main(["--stale", str(stale), "--sot", str(Path(tmp, "nope")), "--go", str(go_root)])
    assert rc == 2
