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

import XCTest

/// AcceptanceTests are the appliance's device acceptance: the things that cannot be tested on
/// the machine that builds it.
///
/// Everything below the interface is unit-tested, and `./scripts/build-ios-app.sh
/// --test-only` runs those tests wherever Go and Swift run. What none of them can do is
/// stand in for iOS itself: the sandbox an app writes its configuration into, the kernel's
/// limit on a socket path, permission for a sensor, and a real Reticulum transport binding
/// its ports inside the sandbox. Those are what this target is for, and it is why it needs a
/// device and is not part of any gate that runs on the machine that builds the appliance.
///
/// The runner is itself an app on the device, so it can do the two things the acceptance
/// needs: drive the interface, and connect to the appliance's loopback ports to see whether
/// a stack that reports itself running is one a client can actually reach.
///
///     GONOMADNET_TEAM=<team> GONOMADNET_DEVICE=<udid> \
///       ./scripts/build-ios-app.sh --acceptance
///
/// Screenshots are attached to every step and kept in the result bundle, which is what
/// makes "the console renders" something a reader can look at rather than something this
/// file asserts about itself.
final class AcceptanceTests: XCTestCase {
    /// The shared instance the transport binds, which is the port everything else attaches
    /// to. It is pinned rather than discovered: a stack that came up on another port is one
    /// the client would never find.
    private let sharedInstancePort: UInt16 = 37428

    /// The sensor feed the appliance serves its position and heading on.
    private let feedPort: UInt16 = 37430

    private var app: XCUIApplication!

    /// Whether this run is in the Simulator, where the loopback assertions do not hold.
    ///
    /// A Simulator shares the Mac's network stack, so 127.0.0.1 is the *Mac's* loopback: a
    /// Reticulum shared instance running on the build machine is on 37428 there, and the
    /// appliance cannot take the port itself. The UI half of the acceptance is still worth
    /// running there — it is how the harness is exercised without a device — but the port
    /// assertions would be about the Mac, so they are skipped rather than made to lie.
    private var onSimulator: Bool {
        #if targetEnvironment(simulator)
        return true
        #else
        return false
        #endif
    }

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
        // Every test starts in landscape, where the controls are on screen. In portrait iPadOS
        // hides the sidebar behind the detail — the console — so a test that ran after one that
        // left the device turned would find no buttons at all, and report the appliance as
        // broken rather than itself as out of position.
        XCUIDevice.shared.orientation = .landscapeLeft
        // iOS asks its own questions the first time the appliance does something: the local
        // network, when the transport binds its first socket, and location, when the sensors
        // start. An alert on screen is a tap that lands nowhere — the acceptance's own tap on
        // Open gonomadnet was eaten by one, and the console it was supposed to open looked
        // like a defect in the appliance rather than a question nobody answered.
        addUIInterruptionMonitor(withDescription: "the system's own questions") { alert in
            let allow = ["Allow", "OK", "Continue", "Allow While Using App", "Allow Once"]
            for button in alert.buttons.allElementsBoundByIndex where allow.contains(button.label) {
                button.tap()
                return true
            }
            return false
        }
        app = XCUIApplication()
        app.launch()
    }

    override func tearDown() {
        // The device is left as it was found, so a failure in one test is not a failure in the
        // next: these run in alphabetical order, in one process after another, on one iPad.
        XCUIDevice.shared.orientation = .landscapeLeft
        app = nil
        super.tearDown()
    }

    // MARK: - The tests

    /// The first launch writes every configuration and starts nothing.
    ///
    /// "Starts nothing" is the part worth asserting: a first launch that quietly brought a
    /// transport up would make the appliance's ports answer before a reader had asked for
    /// anything, and the button that is supposed to bring the stack up would have nothing
    /// left to do.
    func testFirstLaunchWritesTheConfigurationAndStartsNothing() throws {
        XCTAssertTrue(app.buttons["Start Stack"].waitForExistence(timeout: 20),
                      "the controls did not appear")
        attach("the controls, on first launch")

        // Nothing is listening because the appliance has not started anything. Only the
        // device can be asked: see `onSimulator`.
        if !onSimulator {
            XCTAssertEqual(trafficOn(port: sharedInstancePort), false,
                           "the first launch is not supposed to start the transport")
            XCTAssertEqual(trafficOn(port: feedPort), false,
                           "the first launch is not supposed to start the sensor feed")
        }
    }

    /// Start Stack brings up a transport a client can reach.
    func testStartingTheStackBringsUpBothLoopbackPorts() throws {
        startStack()
        attach("the stack running")

        XCTAssertTrue(waitForPort(sharedInstancePort, open: true, within: 30),
                      "the shared instance never accepted a connection on \(sharedInstancePort)")
        XCTAssertTrue(waitForPort(feedPort, open: true, within: 30),
                      "the sensor feed never accepted a connection on \(feedPort)")
    }

    /// Stop Stack releases them.
    ///
    /// This is what `Host.Close` waiting for the console exists for: a stopped appliance
    /// that still holds its ports is one that cannot be started again, and on iOS there is
    /// no second chance to clean up after the process has gone.
    func testStoppingTheStackReleasesThePorts() throws {
        startStack()
        XCTAssertTrue(waitForPort(sharedInstancePort, open: true, within: 30),
                      "the shared instance never came up")

        app.buttons["Stop Stack"].tap()
        XCTAssertTrue(waitForPort(sharedInstancePort, open: false, within: 30),
                      "the shared instance still holds \(sharedInstancePort) after Stop Stack")
        XCTAssertTrue(waitForPort(feedPort, open: false, within: 30),
                      "the sensor feed still holds \(feedPort) after Stop Stack")

        // And it can be brought back up: a stop that leaves a stale socket behind passes the
        // assertions above and still fails the reader on the next start.
        startStack()
        XCTAssertTrue(waitForPort(sharedInstancePort, open: true, within: 30),
                      "the stack did not come back up after a clean stop")
    }

    /// The console renders, and the client is a real one on the device.
    func testTheConsoleRendersTheClient() throws {
        startStack()
        openConsole()

        let console = self.console
        guard console.waitForExistence(timeout: 30) else {
            // The appliance reports what went wrong on the same screen, and a test that says
            // only "the console did not appear" throws that away: the lines under the
            // Console row are what turn a failure here into a diagnosis.
            XCTFail("the console did not appear; the appliance logged:\n\(applianceLog())")
            return
        }
        // The client draws its interface as soon as the terminal is handed to it; a moment
        // is what it takes to arrive as escape sequences and be parsed back into cells.
        Thread.sleep(forTimeInterval: 8)
        attach("the console after the client started")
        note("the console's geometry", geometry())

        // And it is drawn for the window it actually has, which is a different thing from
        // being drawn at all: the client starts at 80 by 24 and draws for that size until it is
        // told the pane's size, leaving the bottom of the console black.
        XCTAssertTrue(waitForConsoleSize(timeout: 15),
                      "the client was never told the size of the window it is in; it is still on \(consoleSize() ?? "nothing") while the pane is \(console.frame.size). The appliance logged:\n\(applianceLog())")

        // How much of the console's own screenshot is drawn on, as evidence beside the
        // screenshots: a client that was never told leaves the bottom of its pane black.
        note("the lit fraction of the console's last rows",
             String(format: "%.4f", litFraction(of: console.screenshot(), rows: 0.94...1.0)))

        // Tapping the console is what raises the software keyboard, and the key strip with it:
        // both are needed below, and a tap that reaches the client is what the client's own
        // menu and lists are reachable by.
        console.tap()
        Thread.sleep(forTimeInterval: 1)
        attach("the console with the key strip")

        // Open the Log, which fills itself with what the client has done and is therefore
        // something to scroll. The menu bar is walked with Tab and the arrows and entered with
        // Return: the strip carries the Tab and the arrows, the software keyboard the Return.
        // The Log is the fourth of the eight menu entries.
        app.buttons["TAB"].tap()
        Thread.sleep(forTimeInterval: 1)
        for _ in 0..<3 {
            app.buttons["→"].tap()
            Thread.sleep(forTimeInterval: 0.4)
        }
        app.typeText("\n")
        Thread.sleep(forTimeInterval: 3)
        attach("the client's log")

        guard let size = consoleSize(), let (cols, rows) = cellSize(size) else {
            XCTFail("the controls do not say what size the client is: \(applianceLog())")
            return
        }

        // A drag over the reader scrolls it, and the client is what does the scrolling: a
        // terminal has no gesture, so the appliance reports the drag as the mouse wheel a real
        // terminal would have sent, at the cell the finger is on. The client gives a wheel to
        // whatever it has under that cell, so a report at the cursor's cell instead scrolls
        // whichever panel holds the focus — the menu bar — and a drag anywhere looks like a
        // console that cannot be dragged.
        let before = console.screenshot()
        console.coordinate(withNormalizedOffset: cell(34, 12, cols: cols, rows: rows))
            .press(forDuration: 0.1,
                   thenDragTo: console.coordinate(withNormalizedOffset: cell(34, 3, cols: cols, rows: rows)))
        Thread.sleep(forTimeInterval: 2)
        let after = console.screenshot()
        attach("the log after the drag")

        // A scroll redraws the panel's text; the cursor's blink redraws one cell, which is a
        // tenth of a percent of it. The line is 2% rather than more because a log's lines look
        // like each other — the text moves without the picture changing much — and what this
        // has to tell apart is "scrolled" from "nothing happened at all".
        let changed = differingFraction(of: before, and: after)
        note("the fraction of the console the drag changed", String(format: "%.3f", changed))
        XCTAssertGreaterThan(changed, 0.02,
                             "the drag scrolled nothing: the log is unchanged. The appliance logged:\n\(applianceLog())")
    }

    /// cell is the coordinate of a cell of the client's screen, given the size it was told.
    ///
    /// The console is a grid of `cols` by `rows`, so a cell in it is at that fraction of the
    /// element — which is what makes a tap land on the row a test means rather than near it.
    private func cell(_ col: Int, _ row: Int, cols: Int, rows: Int) -> CGVector {
        CGVector(dx: (Double(col) + 0.5) / Double(cols), dy: (Double(row) + 0.5) / Double(rows))
    }

    /// cellSize is the client's screen size, parsed from the `69×14` the controls show.
    private func cellSize(_ size: String) -> (cols: Int, rows: Int)? {
        let parts = size.split(separator: "×")
        guard parts.count == 2, let cols = Int(parts[0]), let rows = Int(parts[1]) else { return nil }
        return (cols, rows)
    }

    /// Rotating the device reflows the client for the new grid.
    ///
    /// It waits for the console before it rotates. It used to rotate whatever was there,
    /// which meant three screenshots of a console that had never opened and a test that
    /// passed: an appliance whose console is broken is not one to take pictures of.
    func testRotatingReflowsTheConsole() throws {
        startStack()
        openConsole()
        XCTAssertTrue(console.waitForExistence(timeout: 30),
                      "the console did not appear to be rotated; the appliance logged:\n\(applianceLog())")
        Thread.sleep(forTimeInterval: 8)
        attach("the console in landscape")
        note("the console's geometry", geometry())
        note("the client's size in landscape", consoleSize() ?? "(the controls are not on screen)")

        XCUIDevice.shared.orientation = .portrait
        Thread.sleep(forTimeInterval: 4)
        attach("the console after rotating to portrait")

        // The client is told, and the proof is what it draws. In portrait the pane is taller and
        // narrower, so a client still drawing the landscape grid fills half the height and
        // leaves the bottom of the pane black — which is what the appliance did for a whole
        // session before it ever sent a RESIZE. The controls are not on screen in portrait (the
        // sidebar is behind the console there), so the client's size cannot be read from them;
        // what it drew can.
        XCTAssertGreaterThan(litFraction(of: console.screenshot(), rows: 0.94...1.0), 0.01,
                             "the client did not reflow for the rotation: the bottom of the console is empty (\(geometry()))")

        XCUIDevice.shared.orientation = .landscapeLeft
        Thread.sleep(forTimeInterval: 4)
        attach("the console after rotating back")
    }

    /// The console takes the keyboard when it appears, without being touched.
    ///
    /// A key goes to the first responder: a hardware keyboard's keys arrive there and the
    /// software keyboard is served to it, so a console that is drawn but is not the first
    /// responder receives nothing at all — every list, every button and the menu of the client
    /// go quiet, with nothing on the screen to say why, and a reader finds that the client's
    /// shortcuts do nothing until they happen to tap the console. Ctrl-Q is the key that proves
    /// it here because its effect is the whole console: the client quits, the session ends and
    /// the controls come back.
    ///
    /// No tap on the console happens anywhere in this test, which is the whole of it: every
    /// other console test taps first, because it needs the key strip that the tap raises, and
    /// that tap is what hides this defect.
    func testTheConsoleTakesTheKeyboardWithoutATap() throws {
        // A console cannot be opened in the Simulator at all, and this test is only about the
        // console's keyboard. The appliance refuses a socket path longer than the kernel's 103
        // bytes rather than truncating it, and a Simulator's own application container is 171
        // bytes deep before the socket's name is added to it — so the appliance reports that it
        // could not open the console, and there is nothing here to type into. The console is a
        // device's, and this test is a device's with it.
        try XCTSkipIf(onSimulator,
                      "the appliance's console socket path is 171 bytes in a Simulator, and the kernel allows 103")

        startStack()
        openConsole()
        XCTAssertTrue(console.waitForExistence(timeout: 30),
                      "the console did not appear; the appliance logged:\n\(applianceLog())")
        Thread.sleep(forTimeInterval: 8)
        attach("the console, untouched")

        // This keystroke is the first thing the console is asked to do, and it can only be
        // delivered to a view that holds the keyboard.
        console.typeKey("q", modifierFlags: .control)

        let open = app.buttons["Open gonomadnet"]
        XCTAssertTrue(open.waitForExistence(timeout: 30),
                      "Ctrl-Q did not reach the client: the console is still up, so either the " +
                          "console never took the keyboard or the key was dropped. The appliance logged:\n\(applianceLog())")
        attach("the controls, after the client was quit from the console's own keyboard")
    }

    /// The Scan button in a dialog asks this appliance for a camera, which is the one part of
    /// the feature that no test on a Mac can reach.
    ///
    /// A dialog that takes a typed hex address offers to read one from a QR code instead, and
    /// the client has no camera and cannot have one — an iOS application may not start a
    /// program. So it asks the app, over a loopback TCP connection the app listens on, and the
    /// app answers with what its camera read. Both halves of that are unit-tested against a
    /// fake; what needs a device is that the two halves of *this* app are talking: the
    /// endpoint and the token reach the client, the client dials, and the app presents a
    /// camera.
    ///
    /// The proof is a change on the console, and it is asserted that way because a change is the
    /// only thing this test can see for itself. The client's whole interface is cells drawn into
    /// one custom view: the dialog, its address field and the `< Scan >` button are not UIKit
    /// elements, so there is nothing to query and nothing whose text can be read. What a button
    /// that rendered and did nothing leaves behind is the console it started with, apart from the
    /// caret blinking; everything else it can do — opening a camera, filling the field with what
    /// the camera read, saying why it cannot — is a change a reader would see. It is the same
    /// measurement the drag test above makes, for the same reason.
    ///
    /// The camera may already have read the code in front of it before the test can look, in
    /// which case the scanner has closed itself and the dialog behind it keeps the address. That
    /// is the feature working, and the test passes either way: what it is asserting is that the
    /// client asked, not what the camera saw.
    func testTheDialogsScanButtonAsksTheApplianceForTheCamera() throws {
        // The console cannot be opened in the Simulator at all — its socket path is longer
        // than the kernel allows there, which `testTheConsoleTakesTheKeyboardWithoutATap`
        // explains — and this test is entirely about the console's own dialogs.
        try XCTSkipIf(onSimulator,
                      "the appliance's console socket path is 171 bytes in a Simulator, and the kernel allows 103")

        startStack()
        openConsole()
        XCTAssertTrue(console.waitForExistence(timeout: 30),
                      "the console did not appear; the appliance logged:\n\(applianceLog())")
        Thread.sleep(forTimeInterval: 8)

        // A tap raises the software keyboard and the key strip with it, and the strip is what
        // sends the keys this needs.
        console.tap()
        Thread.sleep(forTimeInterval: 1)

        // Ctrl-N is New Conversation (the client's own shortcut, and the last entry of its
        // Conversations shortcut bar).
        app.buttons["Ctrl"].tap()
        app.typeText("n")
        Thread.sleep(forTimeInterval: 2)
        attach("the New Conversation dialog, with its Scan button")

        // Tab walks from the Addr field to the Scan button directly under it — the dialog's own
        // Pile traversal. Focus is where it will be when the button is pressed, so the caret
        // moving is not part of what the press is measured by.
        app.buttons["TAB"].tap()
        Thread.sleep(forTimeInterval: 1)
        attach("the dialog, with the Scan button about to be pressed")
        let before = console.screenshot()

        app.typeText("\n")
        Thread.sleep(forTimeInterval: 6)
        attach("what the Scan button produced")
        let after = console.screenshot()

        let changed = differingFraction(of: before, and: after)
        note("the fraction of the console the Scan button changed", String(format: "%.4f", changed))
        XCTAssertGreaterThan(
            changed, 0.002,
            "pressing Scan changed nothing on the console: either the button was not drawn, or " +
                "the request it sent never reached the appliance — the endpoint and token the " +
                "client was given are not the scanner's, or the listener answered nothing. " +
                "The appliance logged:\n\(applianceLog())",
        )

        // The camera, if it is still waiting for a code. A code in front of the camera closes
        // the scanner by itself, and the dialog behind it keeps the address.
        let cancel = app.buttons["Cancel"]
        if cancel.waitForExistence(timeout: 10) {
            attach("the scanner, waiting for a code")
            cancel.tap()
            Thread.sleep(forTimeInterval: 2)
            XCTAssertTrue(console.waitForExistence(timeout: 20),
                          "the console did not come back after the scan was cancelled")
        } else {
            note("the scanner was not waiting", "a code was read before the test could see the camera")
        }
    }

    /// Start Sensors turns the feed on, which is the one part of the sensor path that needs
    /// iOS rather than a unit test: the permission it asks for, and the location and motion
    /// services behind it, are the platform's.
    ///
    /// It asserts the button, not a fix. A Wi-Fi-only iPad indoors has no GNSS at all and may
    /// take a while to place itself, so a test that waited for a position would be a test
    /// that fails in a basement and passes by the window. What the appliance promises here is
    /// that it turned the sensors on and says so.
    func testStartSensorsTurnsTheSensorPathOn() throws {
        startStack()

        let start = app.buttons["Start Sensors"]
        XCTAssertTrue(start.waitForExistence(timeout: 20), "the Start Sensors button did not appear")
        start.tap()
        attach("the controls after starting the sensors")

        XCTAssertTrue(app.buttons["Stop Sensors"].waitForExistence(timeout: 30),
                      "the appliance never reported the sensors running; it logged:\n\(applianceLog())")
    }

    /// consoleSize is the size the appliance says the client was told, or nil while the Console
    /// row does not name one, or is not on screen.
    ///
    /// The row reads `Open · 69×35`, and SwiftUI gives a `LabeledContent` one accessibility
    /// element rather than two: what is on the screen is `Console, Open · 69×35`, with a
    /// multiplication sign in it that nothing else on the controls has. In portrait the sidebar
    /// is behind the console, so this nil is the orientation and not the appliance.
    private func consoleSize() -> String? {
        let row = app.staticTexts
            .matching(NSPredicate(format: "label CONTAINS %@", "Open · "))
            .firstMatch
        guard row.exists else { return nil }
        return row.label.components(separatedBy: "Open · ").last
    }

    /// waitForConsoleSize waits for the client to have been told a size other than the one the
    /// appliance started it at.
    ///
    /// The appliance starts every console at 80 by 24 — a size no pane has ever been — and the
    /// view tells the client the real one as soon as it has been laid out. A client still on
    /// 80 by 24 after that is a client drawing for a window it is not in.
    private func waitForConsoleSize(timeout: TimeInterval) -> Bool {
        let startedAt = "80×24"
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if let size = consoleSize(), size != startedAt { return true }
            Thread.sleep(forTimeInterval: 0.25)
        } while Date() < deadline
        return false
    }

    // MARK: - Steps

    /// console is the terminal the client draws on.
    ///
    /// It is found by its identifier and not by its type: the terminal is a view of the
    /// appliance's own, and which element type Xcode reports a view like that as is Xcode's
    /// business rather than the appliance's. Asking for any type is what keeps this test
    /// about the console rather than about a query.
    private var console: XCUIElement {
        app.descendants(matching: .any).matching(identifier: "gonomadnet-console").firstMatch
    }

    /// startStack presses Start Stack and waits for the appliance to say it is running.
    ///
    /// The button turning into Stop Stack is the assertion rather than the status line,
    /// because the button *is* the state: it is what the reader presses next, and it is one
    /// control rather than a label that a status row may or may not expose on its own.
    private func startStack() {
        let start = app.buttons["Start Stack"]
        XCTAssertTrue(start.waitForExistence(timeout: 20), "the Start Stack button did not appear")
        start.tap()

        let stop = app.buttons["Stop Stack"]
        XCTAssertTrue(stop.waitForExistence(timeout: 30),
                      "the appliance never reported the stack running; it logged:\n\(applianceLog())")
    }

    /// openConsole presses Open gonomadnet and waits for the terminal to be on screen.
    private func openConsole() {
        let open = app.buttons["Open gonomadnet"]
        XCTAssertTrue(open.waitForExistence(timeout: 20), "the Open gonomadnet button did not appear")
        open.tap()
    }

    // MARK: - The ports

    /// trafficOn reports whether anything is listening on a loopback port, asked without
    /// waiting: it is used to assert that nothing is, which is a fact about the moment.
    private func trafficOn(port: UInt16) -> Bool {
        waitForPort(port, open: true, within: 1.0)
    }

    /// waitForPort waits for a loopback port to reach the expected state.
    ///
    /// A transport that has just been told to start binds its ports a moment later, and one
    /// that has just been stopped releases them a moment later, so both directions are
    /// waited for rather than assumed.
    private func waitForPort(_ port: UInt16, open: Bool, within timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if connected(port: port) == open { return true }
            Thread.sleep(forTimeInterval: 0.25)
        } while Date() < deadline
        return connected(port: port) == open
    }

    /// connected reports whether a TCP connection to 127.0.0.1:port can be made.
    ///
    /// This is the acceptance's own question asked the way the client asks it: not "did the
    /// transport say it started" but "is there something on the port a client attaches to".
    private func connected(port: UInt16) -> Bool {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else { return false }
        defer { close(fd) }

        var timeout = timeval(tv_sec: 1, tv_usec: 0)
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, socklen_t(MemoryLayout<timeval>.size))
        setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, socklen_t(MemoryLayout<timeval>.size))

        var address = sockaddr_in()
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = port.bigEndian
        address.sin_addr.s_addr = inet_addr("127.0.0.1")

        let result = withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) { socketAddress in
                connect(fd, socketAddress, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        return result == 0
    }

    /// applianceLog is what the controls are showing under *Log*, which is where the
    /// appliance says what it did and why it could not do something.
    private func applianceLog() -> String {
        let lines = app.staticTexts.allElementsBoundByIndex
            .map(\.label)
            .filter { !$0.isEmpty }
        return lines.isEmpty ? "(the appliance shows no lines at all)" : lines.joined(separator: "\n")
    }

    // MARK: - Evidence

    /// attach keeps a screenshot in the result bundle, which is how a passing run is still
    /// something a reader can check.
    private func attach(_ name: String) {
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }

    /// note keeps a line of measurements beside the screenshots.
    ///
    /// A screenshot says what the appliance drew; it does not say how big it thought it
    /// was, and "the window does not fill the screen" is a question about the second one.
    private func note(_ name: String, _ text: String) {
        let attachment = XCTAttachment(string: text)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    /// geometry is where the appliance's window is and how big the console is inside it.
    private func geometry() -> String {
        let window = app.windows.firstMatch.frame
        let consoleFrame = console.exists ? "\(console.frame)" : "(no console)"
        return """
        app:     \(app.frame)
        window:  \(window)
        console: \(consoleFrame)
        """
    }

    /// differingFraction is how much of two screenshots of the same thing differs.
    ///
    /// It is how a test that is not a person sees a scroll: what a terminal draws is cells, and
    /// there is no element inside a cell to ask.
    private func differingFraction(of first: XCUIScreenshot, and second: XCUIScreenshot) -> Double {
        guard let a = first.image.cgImage, let b = second.image.cgImage,
              a.width == b.width, a.height == b.height, a.width > 0, a.height > 0
        else { return 0 }

        let width = a.width
        let height = a.height
        guard let bytesA = pixels(of: a), let bytesB = pixels(of: b) else { return 0 }

        var differing = 0
        for index in stride(from: 0, to: width * height * 4, by: 4) {
            let delta = abs(Int(bytesA[index]) - Int(bytesB[index]))
                + abs(Int(bytesA[index + 1]) - Int(bytesB[index + 1]))
                + abs(Int(bytesA[index + 2]) - Int(bytesB[index + 2]))
            if delta > 40 { differing += 1 }
        }
        return Double(differing) / Double(width * height)
    }

    /// pixels is an image's pixels, top row first.
    private func pixels(of image: CGImage) -> [UInt8]? {
        let width = image.width
        let height = image.height
        var data = [UInt8](repeating: 0, count: width * height * 4)
        guard let space = CGColorSpace(name: CGColorSpace.sRGB),
              let context = CGContext(
                  data: &data,
                  width: width,
                  height: height,
                  bitsPerComponent: 8,
                  bytesPerRow: width * 4,
                  space: space,
                  bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
              ) else { return nil }
        context.draw(image, in: CGRect(x: 0, y: 0, width: CGFloat(width), height: CGFloat(height)))
        return data
    }

    /// litFraction is how much of a band of a screenshot is not the terminal's black.
    ///
    /// It is the only way a test that is not a person can answer "does the client fill the
    /// window it is in": what a terminal draws is cells, and there is no element inside a cell
    /// to ask. `rows` is a fraction of the image's height, measured from the top, and the
    /// screenshot is of the console element alone.
    private func litFraction(of screenshot: XCUIScreenshot, rows: ClosedRange<Double>) -> Double {
        guard let image = screenshot.image.cgImage, let data = pixels(of: image) else { return 0 }

        let width = image.width
        let height = image.height
        let first = max(0, Int(Double(height) * rows.lowerBound))
        let last = min(height, Int(Double(height) * rows.upperBound))
        guard first < last, width > 0 else { return 0 }

        var lit = 0
        for y in first..<last {
            for x in 0..<width {
                let index = (y * width + x) * 4
                if Int(data[index]) + Int(data[index + 1]) + Int(data[index + 2]) > 60 {
                    lit += 1
                }
            }
        }
        return Double(lit) / Double(width * (last - first))
    }
}
