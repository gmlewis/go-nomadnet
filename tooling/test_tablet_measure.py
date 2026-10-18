#!/usr/bin/env python3
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Tests for tooling/tablet-measure.py — the color scan over a raw screencap.
#
# The captures these read are the tablet's own, so the tests build synthetic ones:
# a header, then RGBA_8888 pixels painted with the client's two greens in the
# places the real client puts them. What is pinned is the reading of that scan —
# the rows between the rules, the pane count a set of divider lines bounds, and
# the two checks that fail a rotation which did not take.

import contextlib
import importlib.util
import io
import os
import struct
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
# The tool's own name is hyphenated (the repository's convention for a file that
# is run, not imported), so it is loaded from its path rather than by module name.
_spec = importlib.util.spec_from_file_location("tablet_measure", os.path.join(HERE, "tablet-measure.py"))
tablet_measure = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(tablet_measure)

# What the real captures measure on the SM-X210: a 1920x1200 landscape console
# with its rules at y=80 and y=1088, its two panes split at x=426/438, and its
# dividers at the console's own margins (6 px).
LANDSCAPE = (1920, 1200)


def screen(width, height, rules=(), dividers=(), extra_rule=None):
    """A screencap of a width x height screen with the client's chrome painted on it."""
    pixels = bytearray(b"\x00\x00\x00\xff" * width * height)
    rule = bytes(tablet_measure.RULE)
    divider = bytes(tablet_measure.DIVIDER)
    for y in rules:
        for x in range(2, width - 2):
            at = (y * width + x) * 4
            pixels[at : at + 4] = rule + b"\xff"
    for x in dividers:
        for y in range(min(rules), max(rules)):
            at = (y * width + x) * 4
            pixels[at : at + 4] = divider + b"\xff"
    if extra_rule is not None:
        for x in range(2, width - 2):
            at = (extra_rule * width + x) * 4
            pixels[at : at + 4] = rule + b"\xff"
    header = struct.pack("<IIII", width, height, 1, 1)
    return header + bytes(pixels)


def written(tmp_path, width, height, **kw):
    """Write one synthetic capture and return its path."""
    path = os.path.join(tmp_path, "shot.raw")
    with open(path, "wb") as f:
        f.write(screen(width, height, **kw))
    return path


def checked(path, expect_panes=None):
    """Measure a capture the way the command line does, and return its failure count."""
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        failures = tablet_measure.measure(path, expect_panes)
    globals().setdefault("REPORTS", {})[os.path.basename(path)] = out.getvalue()
    return failures


TMP_DIRS = []


def tmpdir(name):
    """A directory under /tmp, which is where an inspected temp dir must live on macOS.

    The fixtures are megabytes of synthetic pixels (one of them at the tablet's
    real 1920x1200), so the runner removes them when the tests are done rather
    than leaving them for the next person to wonder about.
    """
    import tempfile

    path = os.path.join("/tmp" if sys.platform == "darwin" else tempfile.gettempdir(), f"{name}-{os.getpid()}")
    if path not in TMP_DIRS:
        TMP_DIRS.append(path)
    os.makedirs(path, exist_ok=True)
    return path


def test_reads_the_geometry_of_a_landscape_console():
    """The pair of rules and the splitter a landscape page draws are read back exactly."""
    width, height = LANDSCAPE
    path = written(tmpdir("tablet-measure-test"), width, height, rules=(80, 1088), dividers=(6, 426, 438, 1914))
    w, h, rgba = tablet_measure.load(path)
    assert (w, h) == (width, height), (w, h)
    assert tablet_measure.merge(tablet_measure.rule_rows(w, h, rgba)) == [(80, 80), (1088, 1088)]
    lines = [c[0] for c in tablet_measure.merge(tablet_measure.divider_columns(w, rgba, 83, 1085))]
    assert lines == [6, 426, 438, 1914], lines
    assert tablet_measure.pane_count(lines) == 2, lines


def test_a_portrait_screen_shows_one_pane_and_landscape_two():
    """One pane is what a portrait page must show; the same page in landscape shows two."""
    width, height = 1200, 1920
    path = written(tmpdir("tablet-measure-test"), width, height, rules=(80, 1784), dividers=(6, 1194))
    w, h, rgba = tablet_measure.load(path)
    lines = [c[0] for c in tablet_measure.merge(tablet_measure.divider_columns(w, rgba, 83, 1781))]
    assert tablet_measure.pane_count(lines) == 1, lines


def test_a_portrait_screen_with_a_side_panel_open_is_a_failure():
    """The defect this tool exists for: a side panel that stayed open in portrait."""
    width, height = 1200, 1920
    # A list pane that did not collapse: two panes on a screen standing up.
    path = written(tmpdir("tablet-measure-test"), width, height, rules=(80, 1784), dividers=(6, 426, 438, 1194))
    assert checked(path) == 1
    assert "portrait must show one pane" in REPORTS["shot.raw"], REPORTS["shot.raw"]


def test_a_client_that_stops_short_of_the_screen_is_a_failure():
    """A client laid out for a wider screen does not reach this one's right edge."""
    width, height = LANDSCAPE
    path = written(tmpdir("tablet-measure-test"), width, height, rules=(80, 1088), dividers=(6, 900))
    assert checked(path) == 1
    assert "stops drawing" in REPORTS["shot.raw"], REPORTS["shot.raw"]


def test_a_frame_without_the_clients_rules_is_not_a_console():
    """The controls page has none of this chrome, and must not be read as a console."""
    path = written(tmpdir("tablet-measure-test"), 1200, 1920)
    assert checked(path) == 1
    assert "not in this frame" in REPORTS["shot.raw"], REPORTS["shot.raw"]


def test_the_key_strip_rule_is_measured_as_a_third_rule():
    """With the soft keyboard up, the strip's own rule is the view-height probe."""
    width, height = LANDSCAPE
    path = written(tmpdir("tablet-measure-test"), width, height, rules=(80, 512), dividers=(6, 426, 438, 1914), extra_rule=549)
    w, h, rgba = tablet_measure.load(path)
    assert tablet_measure.merge(tablet_measure.rule_rows(w, h, rgba)) == [(80, 80), (512, 512), (549, 549)]


def test_a_truncated_capture_is_refused_rather_than_measured():
    """A short read is a capture to take again, not a measurement to make."""
    width, height = LANDSCAPE
    path = os.path.join(tmpdir("tablet-measure-test"), "short.raw")
    with open(path, "wb") as f:
        f.write(screen(width, height, rules=(80, 1088))[:-4000])
    try:
        tablet_measure.load(path)
    except ValueError as e:
        assert "truncated" in str(e) or "not a" in str(e), str(e)
        return
    raise AssertionError("a truncated capture was loaded")


def test_a_png_is_either_read_or_refused_with_a_reason():
    """A PNG needs Pillow, and a PNG Pillow cannot read is a reported error, not a crash.

    The tablet's captures are taken raw and need nothing; a PNG is what an older
    session left behind, so both halves have to end in a message that says which.
    """
    path = os.path.join(tmpdir("tablet-measure-test"), "shot.png")
    with open(path, "wb") as f:
        f.write(b"\x89PNG\r\n\x1a\n" + b"\x00" * 64)
    try:
        tablet_measure.load_png(path)
    except ValueError as e:
        message = str(e)
        assert "Pillow" in message or "not a readable PNG" in message, message
        return
    raise AssertionError("a body of zeroes was loaded as a PNG")


if __name__ == "__main__":
    import shutil

    fns = [v for k, v in sorted(globals().items()) if k.startswith("test_") and callable(v)]
    passed = failed = 0
    try:
        for fn in fns:
            try:
                fn()
                print("PASS", fn.__name__)
                passed += 1
            except Exception as e:  # noqa: BLE001
                print("FAIL", fn.__name__, "->", type(e).__name__, str(e)[:140])
                failed += 1
    finally:
        for path in TMP_DIRS:
            shutil.rmtree(path, ignore_errors=True)
    print(f"\n{passed} passed, {failed} failed")
    sys.exit(1 if failed else 0)
