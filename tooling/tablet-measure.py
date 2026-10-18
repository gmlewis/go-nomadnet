#!/usr/bin/env python3
# Copyright 2026 Glenn Lewis. All rights reserved.
#
"""Measure what the gonomadnet client drew on the tablet, from a screenshot.

The appliance's console is full of pure green: the client's menu-bar rule and
shortcut-bar rule are #00ff00 across their whole width, and its pane dividers
are #00f100. Scanning a capture for those two colors measures three things at
once, and all three are what a screen-rotation defect moves:

  - the body between the two rules -> the rows the client believes it has
    (divide by 24 px, the console cell at 13 dp and density 240);
  - the columns that are divider-green -> how many panes the client laid out.
    Held in portrait the Conversations, Network, Channels and Guide screens
    must show ONE pane (the side panel hides), and two in landscape;
  - the key strip's own top rule (a third #00ff00 row, 90 px above the view's
    bottom at FONT_SCALE 1.5) -> the height of the view the appliance gave the
    client, which is the number to compare the client's grid against.

A capture is what `adb exec-out screencap` writes: a 12- or 16-byte header
(width, height, format, colorspace) followed by raw RGBA_8888 pixels, as
`tooling/tablet-drive.sh shot` takes it. A PNG is accepted too when Pillow is
installed (the appliance's captures are RGBA or RGB); the raw form needs
nothing but the standard library and is what the driver takes.

Usage:
  tablet-measure.py [--panes N] <capture> [<capture>...]

Exit status is 1 when any capture fails a check: a portrait screen with more
than one pane, a client that does not reach its screen's right edge, or a pane
count that disagrees with --panes.

See tooling/README.md ("Driving the tablet") and the android-tablet skill.
"""

import struct
import sys
import zlib

# Android 9 added a colorspace field to the screencap header.
HEADER_LONG = 16
HEADER_SHORT = 12

# The two colors the client's chrome is drawn in: the palette's pure green for
# the menu bar's and footer's rules, and the cube-quantized border green for the
# pane dividers. Both are pinned by the tests.
RULE = (0x00, 0xFF, 0x00)
DIVIDER = (0x00, 0xF1, 0x00)

# The console's cell in pixels: a 13 dp monospace font at density 240.
CELL_WIDTH = 12
CELL_HEIGHT = 24

# How far short of the screen's right edge a client may stop drawing: the
# console's own margins, measured on the tablet as 6 px a side.
MARGIN_SLACK = 40


def load(path):
    """Load a capture as (width, height, pixels) — RGBA_8888, four bytes a pixel."""
    with open(path, "rb") as f:
        data = f.read()
    if data[:8] == b"\x89PNG\r\n\x1a\n":
        return load_png(path)
    return load_raw(data, path)


def load_raw(data, path):
    """Load one `screencap` dump: a small header, then RGBA_8888 pixels."""
    if len(data) < HEADER_SHORT:
        raise ValueError(f"{path}: {len(data)} bytes is too short to be a screencap")
    width, height, pixel_format = struct.unpack("<III", data[:12])
    # The size says which header this is, and is at the same time what proves the
    # pixel data is all there: a short read is a capture to take again, not a
    # measurement to make.
    header = HEADER_SHORT if len(data) == HEADER_SHORT + width * height * 4 else HEADER_LONG
    if len(data) != header + width * height * 4:
        raise ValueError(
            f"{path}: {len(data)} bytes is not a {width}x{height} screencap with a "
            f"{HEADER_SHORT}- or {HEADER_LONG}-byte header (a PNG, or a truncated capture)"
        )
    if pixel_format != 1:
        raise ValueError(f"{path}: pixel format {pixel_format}, and only RGBA_8888 (1) is understood")
    return width, height, data[header:]


def load_png(path):
    """Load a PNG through Pillow, for captures taken before the raw form was used."""
    try:
        from PIL import Image  # noqa: PLC0415
    except ImportError:
        raise ValueError(
            f"{path}: a PNG needs Pillow (`python3 -m pip install pillow`); "
            "take captures as raw with `tooling/tablet-drive.sh shot` instead"
        ) from None
    try:
        image = Image.open(path).convert("RGBA")
    except Exception as e:  # Pillow raises its own exception types
        raise ValueError(f"{path}: not a readable PNG: {e}") from None
    return image.width, image.height, image.tobytes()


def channel(rgba, offset, lo, hi, width):
    """One channel of the pixels of every row lo..hi at one x, as a bytes object."""
    stride = width * 4
    at = lo * stride + offset
    return rgba[at : at + (hi - lo + 1) * stride : stride]


def rule_rows(width, height, rgba):
    """Return the y of every row that is the pure-green rule across its width."""
    stride = width * 4
    needle = bytes(RULE) + b"\xff"
    rows = []
    for y in range(height):
        if rgba[y * stride : (y + 1) * stride].count(needle) > width // 2:
            rows.append(y)
    return rows


def divider_columns(width, rgba, lo, hi):
    """Return the x of every column that is divider-green over most of rows lo..hi."""
    spans = hi - lo + 1
    cols = []
    for x in range(width):
        if channel(rgba, x * 4 + 1, lo, hi, width).count(DIVIDER[1]) * 3 < spans * 2:
            continue
        if channel(rgba, x * 4, lo, hi, width).count(DIVIDER[0]) * 3 < spans * 2:
            continue
        if channel(rgba, x * 4 + 2, lo, hi, width).count(DIVIDER[2]) * 3 < spans * 2:
            continue
        cols.append(x)
    return cols


def merge(values, gap=3):
    """Group near values into (first, last) runs, so one drawn line is one run."""
    runs = []
    for v in values:
        if runs and v - runs[-1][1] <= gap:
            runs[-1][1] = v
        else:
            runs.append([v, v])
    return [tuple(r) for r in runs]


def pane_count(lines):
    """How many panes a list of divider lines bounds.

    A pane is a gap wider than a cell between two divider lines: the splitter
    between two panes puts their borders a cell apart, so `[6, 630, 642, 1914]`
    is two panes and `[18, 1194]` is one.
    """
    if len(lines) < 2:
        return len(lines)
    return max(1, sum(1 for a, b in zip(lines, lines[1:]) if b - a > 2 * CELL_WIDTH))


def measure(path, expect_panes=None):
    """Measure one capture, report it, and return the number of failed checks."""
    width, height, rgba = load(path)
    orientation = "portrait" if height > width else "landscape"
    print(f"{path}")
    print(f"  screen     {width}x{height}  {orientation}")

    rules = merge(rule_rows(width, height, rgba))
    if len(rules) < 2:
        print(f"  rules      {rules} — the client's menu-bar and footer rules are not in this frame")
        return 1
    print(f"  rules      y = {', '.join(str(r[0]) for r in rules)}")

    top, bottom = rules[0][0], rules[-1][0]
    print(f"  body       y {top}..{bottom}  ({bottom - top} px = {(bottom - top) // CELL_HEIGHT} rows)")

    lines = [c[0] for c in merge(divider_columns(width, rgba, top + 3, bottom - 3))]
    panes = pane_count(lines)
    print(f"  panes      {panes}  (dividers at x = {', '.join(str(x) for x in lines)})")

    failures = 0
    # The appliance gives the client the whole view, so a client that stops short
    # of the screen's right edge was laid out for a screen it is not on.
    if lines and lines[-1] < width - MARGIN_SLACK:
        print(f"  CHECK      FAIL: the client stops drawing at x={lines[-1]} on a {width} px screen")
        failures += 1
    if orientation == "portrait" and panes > 1:
        print(f"  CHECK      FAIL: portrait must show one pane, and this shows {panes}")
        failures += 1
    if expect_panes is not None and panes != expect_panes:
        print(f"  CHECK      FAIL: asked for {expect_panes} panes, counted {panes}")
        failures += 1
    if not failures:
        print("  CHECK      ok")
    return failures


def main(argv):
    expect_panes = None
    paths = []
    i = 0
    while i < len(argv):
        arg = argv[i]
        if arg == "--panes":
            i += 1
            try:
                expect_panes = int(argv[i])
            except (IndexError, ValueError):
                print("--panes needs the number of panes to expect", file=sys.stderr)
                return 2
        elif arg in ("-h", "--help"):
            print(__doc__.strip())
            return 0
        else:
            paths.append(arg)
        i += 1

    if not paths:
        print(__doc__.strip())
        return 2

    failures = 0
    for n, path in enumerate(paths):
        if n:
            print()
        try:
            failures += measure(path, expect_panes)
        except (OSError, ValueError, zlib.error) as e:
            print(f"{path}\n  ERROR      {e}")
            failures += 1
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
