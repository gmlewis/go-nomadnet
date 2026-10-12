// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

import Foundation
import XCTest

@testable import TerminalKit

/// The colors the console is drawn in.
///
/// A palette that is off by one entry draws the whole interface in the wrong colors
/// and nothing about it looks like a bug in the code, so every entry is asserted here
/// rather than looked at on a device. The colors the client leaves to the terminal
/// matter most of all, and they are the appliance's published theme: the same
/// `colors.properties` the Android APK carries is the file this suite reads, so the
/// console and the Termux install cannot resolve the operator's theme two ways.
final class TerminalPaletteTests: XCTestCase {

    func testThePaletteIsTheThemeTheAppliancePublishes() throws {
        // The drift guard: the shipped palette must be the file the client installs
        // under Termux. Both the constants and the sixteen entries are checked against
        // the file itself rather than against a second copy of the values.
        let theme = try publishedTheme()

        XCTAssertEqual(
            TerminalPalette.defaultBackground, theme["background"],
            "the theme's own background")
        XCTAssertEqual(
            TerminalPalette.defaultForeground, theme["foreground"],
            "the theme's own foreground")
        // The cursor is drawn as the cell's own colors reversed, so the theme's cursor
        // color is what that block looks like only when it is the theme's foreground.
        XCTAssertEqual(theme["cursor"], theme["foreground"], "the theme's cursor")

        let shipped = TerminalPalette()
        XCTAssertEqual(shipped.background, TerminalPalette.defaultBackground)
        XCTAssertEqual(shipped.foreground, TerminalPalette.defaultForeground)
        XCTAssertEqual(shipped, TerminalPalette.shipped)

        for index in 0..<16 {
            XCTAssertEqual(
                shipped.ansi[index], theme["color\(index)"],
                "palette entry \(index) is not the theme's")
            XCTAssertEqual(
                shipped.argb(.palette(index), isBackground: false), theme["color\(index)"],
                "entry \(index) does not resolve to the theme's color")
        }
    }

    func testTheThemeIsReadFromTermuxFormat() throws {
        // Termux's own format, parsed the way the reader parses it: one key per line,
        // "#rrggbb" values, and comment lines starting with "#" ignored while the "#"
        // inside a value is part of the value.
        let text = """
            # a comment, not a color
            foreground = #00ff00
            background = #000000
            cursor = #00ff00
            color0 = #1d2021
            """
        let partial = TerminalPalette.parseTheme(text)
        XCTAssertNil(partial, "a theme missing eleven entries must not be half-read")

        let full = try publishedThemeText()
        let theme = TerminalPalette.parseTheme(full)
        XCTAssertNotNil(theme, "the published theme is not readable as a Termux theme")
        XCTAssertEqual(theme?.foreground, TerminalPalette.defaultForeground)
        XCTAssertEqual(theme?.background, TerminalPalette.defaultBackground)

        let spaced = full.replacingOccurrences(of: " = ", with: "=")
        XCTAssertEqual(TerminalPalette.parseTheme(spaced)?.ansi, theme?.ansi)

        let commented = full.replacingOccurrences(of: "color3 =", with: "color3: ")
        XCTAssertNil(
            TerminalPalette.parseTheme(commented),
            "a line that names no color must leave the entry missing")
    }

    func testTheRestOfThePaletteIsTheColorCubeAndTheGrayRamp() {
        // Entries 16 to 231 are a 6x6x6 cube whose steps are the ones every terminal
        // uses, and the last twenty-four are the grays. This is the part a program
        // reaches for when it asks for 256 colors, which the client does.
        let palette = TerminalPalette()
        let steps = [0x00, 0x5F, 0x87, 0xAF, 0xD7, 0xFF]

        for r in 0..<6 {
            for g in 0..<6 {
                for b in 0..<6 {
                    let index = 16 + 36 * r + 6 * g + b
                    let want = 0xFF00_0000 | UInt32(steps[r] << 16 | steps[g] << 8 | steps[b])
                    XCTAssertEqual(
                        palette.argb(.palette(index), isBackground: false), want,
                        "the cube entry \(index)")
                }
            }
        }

        // The ramp runs from nearly black to nearly white in twenty-four even steps.
        XCTAssertEqual(palette.argb(.palette(232), isBackground: false), 0xFF08_0808)
        XCTAssertEqual(palette.argb(.palette(255), isBackground: false), 0xFFEE_EEEE)
        for i in 233..<256 {
            let gray = UInt32(8 + 10 * (i - 232))
            let want = 0xFF00_0000 | (gray << 16) | (gray << 8) | gray
            XCTAssertEqual(palette.argb(.palette(i), isBackground: false), want, "the gray \(i)")
        }

        // An index past the end of the table comes from a program that asked for a
        // color that does not exist. It is clamped rather than refused: a terminal
        // that dies on a color escape is worse than one that draws white.
        XCTAssertEqual(
            palette.argb(.palette(999), isBackground: false),
            palette.argb(.palette(255), isBackground: false))
        XCTAssertEqual(
            palette.argb(.palette(-1), isBackground: false),
            palette.argb(.palette(0), isBackground: false))
    }

    func testA24BitColorIsDrawnAsItWasSent() {
        // The client is run with COLORTERM=truecolor, so most of what it draws arrives
        // as 24-bit rather than as a palette entry, and it must reach the screen
        // unchanged: rounding it into the 256-color cube is what makes a themed
        // interface look wrong.
        let palette = TerminalPalette()
        let cases: [(r: Int, g: Int, b: Int, want: UInt32)] = [
            (0x0A, 0x14, 0x1E, 0xFF0A_141E),
            (0x00, 0xA5, 0x33, 0xFF00_A533),
            (0x00, 0x0F, 0xD3, 0xFF00_0FD3),
            (0xFF, 0xFF, 0xFF, 0xFFFF_FFFF),
            (0x00, 0x00, 0x00, 0xFF00_0000),
        ]
        for entry in cases {
            XCTAssertEqual(
                palette.argb(.rgb(r: entry.r, g: entry.g, b: entry.b), isBackground: false),
                entry.want, "the color \(entry.r),\(entry.g),\(entry.b)")
        }

        // Anything above the 24 bits a color has is not a color, and must not leak
        // into the alpha byte: a fully transparent glyph is an invisible one.
        XCTAssertEqual(
            palette.argb(.rgb(r: 0x112, g: 0x345, b: 0x678), isBackground: false),
            palette.argb(.rgb(r: 0x12, g: 0x45, b: 0x78), isBackground: false))
    }

    func testTheTerminalsOwnColorsAreTheThemes() {
        // A cell the client gave no color for is drawn in the terminal's own, and the
        // client leaves most of its chrome to the terminal, so the default pair is the
        // whole look of the interface.
        let palette = TerminalPalette()
        XCTAssertEqual(
            palette.argb(.default, isBackground: false), TerminalPalette.defaultForeground)
        XCTAssertEqual(
            palette.argb(.default, isBackground: true), TerminalPalette.defaultBackground)
        XCTAssertNotEqual(
            TerminalPalette.defaultForeground, TerminalPalette.defaultBackground,
            "the default text is the same color as the default background")
        XCTAssertEqual(
            TerminalPalette.defaultForeground & 0xFF00_0000, TerminalPalette.opaque,
            "an opaque color is the only readable one")
    }

    func testAPenResolvesToTheTwoColorsACellIsDrawnWith() {
        // The pair is a tuple rather than a type of its own, so each half is asserted where
        // it is: a swap in the wrong place is exactly what this is here to catch.
        let palette = TerminalPalette()
        let plainPair = palette.colors(of: .plain)
        XCTAssertEqual(plainPair.foreground, TerminalPalette.defaultForeground)
        XCTAssertEqual(plainPair.background, TerminalPalette.defaultBackground)

        let named = TerminalPen(foreground: .palette(1), background: .palette(4))
        let namedPair = palette.colors(of: named)
        XCTAssertEqual(namedPair.foreground, palette.argb(.palette(1), isBackground: false))
        XCTAssertEqual(namedPair.background, palette.argb(.palette(4), isBackground: true))

        // Reverse video swaps them, which is how a selected list row or a menu bar is
        // drawn.
        let plain = TerminalPen(
            foreground: .rgb(r: 0x11, g: 0x22, b: 0x33),
            background: .rgb(r: 0xFF, g: 0xEE, b: 0x00))
        var reverse = plain
        reverse.reverse = true
        let reversePair = palette.colors(of: reverse)
        XCTAssertEqual(
            reversePair.foreground,
            palette.argb(.rgb(r: 0xFF, g: 0xEE, b: 0x00), isBackground: true))
        XCTAssertEqual(
            reversePair.background,
            palette.argb(.rgb(r: 0x11, g: 0x22, b: 0x33), isBackground: false))

        // Reversing a cell that named no colors swaps the theme's own two.
        var plainReverse = TerminalPen.plain
        plainReverse.reverse = true
        let plainReversePair = palette.colors(of: plainReverse)
        XCTAssertEqual(plainReversePair.foreground, TerminalPalette.defaultBackground)
        XCTAssertEqual(plainReversePair.background, TerminalPalette.defaultForeground)

        // Reverse is the only attribute that changes the pair: bold, italic, underline
        // and blink are the renderer's business and never move a color.
        var decorated = plain
        decorated.bold = true
        decorated.italic = true
        decorated.underline = true
        decorated.blink = true
        let decoratedPair = palette.colors(of: decorated)
        XCTAssertEqual(decoratedPair.foreground, palette.colors(of: plain).foreground)
        XCTAssertEqual(decoratedPair.background, palette.colors(of: plain).background)
    }

    // ------------------------------------------------------------------ the theme file

    /// The APK's terminal theme, as name to opaque color.
    private func publishedTheme() throws -> [String: UInt32] {
        let text = try publishedThemeText()
        var values = [String: UInt32]()
        for raw in text.split(separator: "\n") {
            let line = raw.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !line.isEmpty, !line.hasPrefix("#"), let equals = line.firstIndex(of: "=") else {
                continue
            }
            let key = line[line.startIndex..<equals]
                .trimmingCharacters(in: .whitespacesAndNewlines)
            let value = line[line.index(after: equals)...]
                .trimmingCharacters(in: .whitespacesAndNewlines)
            guard let color = TerminalPalette.themeColor(value) else { continue }
            values[key] = color
        }
        return values
    }

    /// A failure the drift guard cannot be skipped past.
    private struct MissingTheme: Error {}

    /// The bytes of the published theme, found from this file's own path.
    private func publishedThemeText() throws -> String {
        let relative = "android/app/src/main/assets/colors.properties"
        var directory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
        for _ in 0..<6 {
            let candidate = directory.appendingPathComponent(relative)
            if FileManager.default.fileExists(atPath: candidate.path) {
                return try String(contentsOf: candidate, encoding: .utf8)
            }
            directory = directory.deletingLastPathComponent()
        }
        // The guard is the point of the suite: a theme that cannot be read is a failure
        // and not a skip, because a skipped guard guards nothing.
        XCTFail("the published theme is not beside this package: \(relative)")
        throw MissingTheme()
    }
}
