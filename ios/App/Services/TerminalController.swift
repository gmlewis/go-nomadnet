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
import TerminalKit

/// TerminalController owns one console session and the terminal it draws into.
///
/// It is the join between the socket and the screen, and it is deliberately the only
/// place in the app that knows about both. The session speaks frames to the Go half; the
/// parser turns the client's output into screen state; the screen is what the view draws.
/// Every one of those three is tested on its own, and this is the wiring between them.
///
/// The session delivers its output on a background queue, because that is where a socket
/// read belongs. The screen and the view are the main actor's, so the output is hopped
/// there once, at the boundary, rather than each of the three guarding itself.
@MainActor
public final class TerminalController {
    /// The terminal's screen, which the view draws and the parser fills.
    public let screen: TerminalScreen

    /// The escape-sequence parser, which turns the client's output into screen state.
    public let parser: TerminalParser

    /// The session with the Go bridge, which carries the frames.
    public let session: ConsoleSession

    /// Called on the main actor when the client has drawn something.
    public var onScreenChanged: (() -> Void)?

    /// Called on the main actor when the session ends.
    public var onEnd: ((ConsoleEnd) -> Void)?

    /// Called when the client sets the window title, which is the page it is showing.
    public var onTitle: ((String) -> Void)?

    /// Creates a controller for a console socket.
    ///
    /// The screen is created at the size the app asks for rather than at a default: the
    /// client reflows for whatever the app tells it, and a wrong first size would show as
    /// a reflow on the reader's screen.
    public init(socketPath: String, cols: Int, rows: Int) {
        let screen = TerminalScreen(cols: cols, rows: rows)
        self.screen = screen
        self.parser = TerminalParser(screen: screen)
        self.session = ConsoleSession(
            socketPath: socketPath,
            cols: UInt16(clamping: cols),
            rows: UInt16(clamping: rows)
        )
        self.parser.titleSink = { [weak self] title in
            Task { @MainActor in self?.onTitle?(title) }
        }
        self.parser.clipboardSink = { text in
            // The client asked for text to be put on the clipboard, which it does for a
            // mouse selection. Nothing is copied without the reader asking: the appliance
            // does not put what is on a screen onto a shared clipboard on its own.
            _ = text
        }
        self.session.onOutput = { [weak self] bytes in
            Task { @MainActor in self?.consume(bytes) }
        }
        self.session.onEnd = { [weak self] end in
            Task { @MainActor in self?.onEnd?(end) }
        }
    }

    /// start attaches to the console and begins reading the client's output.
    public func start() {
        session.start()
    }

    /// stop ends the session. It is safe to call more than once.
    public func stop() {
        session.stop()
    }

    /// send gives the client keystrokes or a mouse report.
    public func send(_ bytes: [UInt8]) {
        session.send(bytes)
    }

    /// resize tells the client its window has changed.
    ///
    /// This is the app->bridge half of the protocol, and the client reflows for it: a
    /// console whose grid is not what the client believes it is draws its interface in the
    /// wrong place, which after a rotation is every line on the screen.
    public func resize(cols: Int, rows: Int) {
        screen.resize(cols: cols, rows: rows)
        session.resize(cols: UInt16(clamping: cols), rows: UInt16(clamping: rows))
        onScreenChanged?()
    }

    /// consume parses a chunk of the client's output and reports that the screen changed.
    private func consume(_ bytes: [UInt8]) {
        parser.parse(bytes)
        onScreenChanged?()
    }
}
