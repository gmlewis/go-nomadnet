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

import ApplianceKit
import Combine
import CoreLocation
import Foundation
import TerminalKit

/// ApplianceModel is the appliance as the interface sees it.
///
/// It is what a screen can observe: the state of the stack, where the console is, whether
/// there is a position and a heading, and the last few lines of what happened. Everything
/// it does is a call into `StackSupervisor` or into a device framework, so the decisions
/// themselves stay in `ApplianceKit` where they are tested on a machine with no device.
@MainActor
public final class ApplianceModel: ObservableObject {
    /// What the stack is doing.
    @Published public private(set) var state: StackSupervisor.State = .stopped

    /// The console's socket, once one has been started.
    @Published public private(set) var consolePath: String?

    /// Whether the sensors are running.
    @Published public private(set) var sensorsRunning = false

    /// Whether the appliance has a position, which is `has a fix`, not `was asked for one`.
    @Published public private(set) var hasFix = false

    /// Whether the appliance has a heading it can publish.
    @Published public private(set) var hasHeading = false

    /// The last few lines worth showing the reader.
    @Published public private(set) var log: [String] = []

    /// Which way the device measures the world, saved as it is changed.
    @Published public var headingAxis: HeadingAxis = .screenBack {
        didSet {
            guard headingAxis != oldValue else { return }
            // Through the supervisor, because it is the copy the next start renders the
            // configuration from: a heading axis saved only here is one the appliance
            // forgets the moment it is started again.
            supervisor.updateSettings { $0.headingAxis = headingAxis }
            settings = supervisor.settings
            motion.axis = headingAxis
            // The heading the old axis measured is not an answer to a question about the new
            // one, so it stops being reported until the sensor has one for this axis.
            hasHeading = false
        }
    }

    /// Whether the appliance publishes a position and a heading on its own.
    ///
    /// It is the reader's standing answer to a question the system keeps asking: a
    /// device that is left on a stand should come back to publishing its position
    /// without being told again, and one whose reader turned the sensors off should stay
    /// off.
    @Published public var enableSensors: Bool = true {
        didSet {
            guard enableSensors != oldValue else { return }
            supervisor.updateSettings { $0.enableSensors = enableSensors }
            settings = supervisor.settings
            if !enableSensors, sensorsRunning {
                toggleSensors()
            } else if enableSensors, !sensorsRunning {
                toggleSensors()
            }
        }
    }

    /// Whether the appliance runs its stack, as opposed to being a bare client.
    @Published public var enableStack: Bool = true {
        didSet {
            guard enableStack != oldValue else { return }
            supervisor.updateSettings { $0.enableStack = enableStack }
            settings = supervisor.settings
            if !enableStack, state.isRunning {
                toggleStack()
            } else if enableStack, !state.isRunning {
                toggleStack()
            }
        }
    }

    /// The console's controller, once a console has been opened.
    @Published public private(set) var terminal: TerminalController?

    /// The appliance's persisted settings.
    public private(set) var settings: ApplianceSettings

    /// The supervisor that owns the Go half.
    public let supervisor: StackSupervisor

    /// The Bluetooth radio, which the transport dials through the appliance.
    public let radio = RadioManager()

    /// The radios a scan has found, which is what the controls offer to pair with.
    public var radiosFound: [RadioManager.Found] { radio.found }

    /// What the radio is doing.
    public var radioStatus: String { radio.state.description }

    /// Whether a radio is paired.
    public var radioPaired: Bool { radio.state.isPaired }

    /// scanForRadios begins looking for an RNode, asking for Bluetooth permission the first
    /// time it is used.
    public func scanForRadios() {
        radio.startScanning()
    }

    /// pair pairs with a radio the scan found.
    ///
    /// A radio the transport is already using is one the configuration has to be rendered
    /// for, so pairing while the stack is running is reported rather than quietly left until
    /// the next start: the interface is written when the stack starts.
    public func pair(_ found: RadioManager.Found) {
        radio.pair(found.id)
        append("pairing with \(found.name); the transport dials it on the next start")
    }

    /// unpair releases the radio, and the interface it was rendered as with it.
    public func unpairRadio() {
        radio.unpair()
        append("the radio is unpaired; the transport leaves it out on the next start")
    }

    private let settingsStore: SettingsStore
    private let location = LocationManager()
    private let motion = MotionManager()
    private let sink = ReadingSink()
    private lazy var pipeline = SensorPipeline(publish: { [sink] reading in sink.accept(reading) })

    /// Creates the model over a supervisor and a settings store.
    /// A supervisor is built here rather than by a default argument, because a default
    /// argument is evaluated outside the main actor and this whole type lives on it.
    public init(supervisor: StackSupervisor? = nil,
                settingsStore: SettingsStore = UserDefaultsSettingsStore()) {
        let supervisor = supervisor ?? StackSupervisor()
        self.supervisor = supervisor
        self.settingsStore = settingsStore
        self.settings = ApplianceSettings.load(from: settingsStore)
        self.headingAxis = settings.headingAxis
        self.enableSensors = settings.enableSensors
        self.enableStack = settings.enableStack

        supervisor.onStateChange = { [weak self] state in
            self?.state = state
        }
        supervisor.onLog = { [weak self] line in
            self?.append(line)
        }

        // The transport is rendered for whatever radio is paired *at the time the stack
        // starts*, which is why this is asked rather than remembered.
        supervisor.radioInterface = { [weak radio] in radio?.radioSpec }
        radio.onLog = { [weak self] line in
            self?.append(line)
        }

        location.onSample = { [weak self] sample in
            self?.feed(sample)
        }
        motion.onReading = { [weak self] reading in
            self?.feed(reading)
        }
        motion.location = { [weak self] in self?.location.lastFix }
    }

    /// transportStatus is what to call the transport in the status list.
    public var transportStatus: String {
        switch state {
        case .stopped: return "Stopped"
        case .starting: return "Starting"
        case .running: return "Running"
        case .stopping: return "Stopping"
        case .failed: return "Failed"
        }
    }

    /// consoleStatus is what to call the console in the status list.
    public var consoleStatus: String {
        consolePath == nil ? "Closed" : "Open"
    }

    /// locationStatus is what the appliance knows about its own position.
    public var locationStatus: String {
        guard sensorsRunning else { return "Off" }
        if let fix = location.lastFix {
            return String(format: "%.5f, %.5f", fix.latitude, fix.longitude)
        }
        switch location.authorization {
        case .denied, .restricted: return "Permission refused"
        case .notDetermined: return "Waiting for permission"
        default: return "No fix yet"
        }
    }

    /// headingStatus is what the appliance knows about which way it points.
    public var headingStatus: String {
        guard sensorsRunning else { return "Off" }
        guard motion.isAvailable else { return "No sensor" }
        return hasHeading ? headingAxis.rawValue : "Withheld"
    }

    // MARK: - Stack

    /// start brings the appliance up.
    public func prepare() {
        supervisor.prepare()
    }

    /// toggleStack starts the appliance, or stops it if it is running.
    public func toggleStack() {
        if state.isRunning {
            supervisor.stop()
            consolePath = nil
            terminal = nil
        } else {
            supervisor.start()
        }
        state = supervisor.state
    }

    /// openConsole attaches the client to a console.
    ///
    /// The size is a starting point only: the view tells the client its real window as soon
    /// as it is laid out, and the client reflows for it.
    public func openConsole() {
        guard state.isRunning else {
            append("start the stack before opening the console")
            return
        }
        do {
            let path = try supervisor.startConsole(cols: 80, rows: 24)
            let controller = TerminalController(socketPath: path, cols: 80, rows: 24)
            controller.onEnd = { [weak self] end in
                self?.append("the console ended: \(end)")
                self?.consolePath = nil
                self?.terminal = nil
            }
            controller.start()
            terminal = controller
            consolePath = path
        } catch {
            append("the console did not open: \(error)")
        }
    }

    // MARK: - Configuration

    /// setHub changes the hub the transport dials.
    ///
    /// The hub is resolved and rendered before the settings are saved, because the values
    /// are what the next start renders from: an edit that would not render is one the
    /// transport would be started without, or started with no hub at all.
    ///
    /// - Returns: why the hub has no interface, when it resolved to no address. A hub that
    ///   did not resolve is not refused — an appliance with no hub still runs — but it is
    ///   reported, because a transport that is up with nothing to connect through is the
    ///   least debuggable state the appliance has.
    ///
    /// - Throws: `NodeConfigError.nonLiteralAddress` when a name reaches the renderer, which
    ///   is what a configuration the renderer will not write produces.
    @discardableResult
    public func setHub(host: String, port: Int) throws -> String? {
        let rendered = try SettingsEdit.render(
            hubHost: host,
            hubPort: port,
            rpcKey: settings.rpcKey
        )
        supervisor.updateSettings {
            $0.hubHost = host
            $0.hubPort = port
        }
        settings = supervisor.settings

        if let problem = rendered.hubProblem {
            append(problem)
        } else if let address = rendered.hubAddress {
            append("the hub is now \(host) (\(address)), port \(port)")
        }
        // The transport is started from the configuration the settings render, so the
        // new hub is dialled only once it has been started from the new file.
        if state.isRunning {
            append("stop and start the stack to dial it")
        }
        return rendered.hubProblem
    }

    /// restartStack brings the transport down and up, so a configuration edit is the
    /// configuration the transport is running.
    public func restartStack() {
        guard state.isRunning else {
            supervisor.start()
            state = supervisor.state
            return
        }
        supervisor.stop()
        consolePath = nil
        terminal = nil
        supervisor.start()
        state = supervisor.state
    }

    /// clientConfigText is the client's own configuration, as the editor reads it.
    public func clientConfigText() throws -> String {
        try supervisor.clientConfigText()
    }

    /// saveClientConfig writes an edited client configuration, and returns what was
    /// written — which is the text with the appliance's own values stated over it.
    ///
    /// - Throws: `ClientConfigError.missingKeys` when the edit dropped a key the
    ///   appliance has to state.
    @discardableResult
    public func saveClientConfig(_ text: String) throws -> String {
        try supervisor.saveClientConfig(text)
        let written = try supervisor.clientConfigText()
        append("the client's configuration was saved")
        return written
    }

    /// renderedConfiguration is what the current settings produce, or the reason they
    /// produce nothing.
    public func renderedConfiguration() -> Result<SettingsEdit.Rendered, Error> {
        Result {
            try SettingsEdit.render(
                hubHost: settings.hubHost,
                hubPort: settings.hubPort,
                rpcKey: settings.rpcKey
            )
        }
    }

    // MARK: - Sensors

    /// toggleSensors starts or stops the position and heading feed.
    public func toggleSensors() {
        if sensorsRunning {
            location.stop()
            motion.stop()
            sensorsRunning = false
            hasFix = false
            hasHeading = false
            append("sensors stopped")
            return
        }

        if location.authorization == .notDetermined {
            location.requestPermission()
        }
        let position = location.start()
        let heading = motion.start()
        motion.axis = headingAxis
        sensorsRunning = position || heading
        append(sensorsRunning
               ? "sensors started (position: \(position), heading: \(heading))"
               : "no sensors could be started on this device")
    }

    /// feed offers one position to the appliance.
    ///
    /// `SensorPipeline` decides whether a sample is worth sending — it owns the rate limit
    /// and the range checks, and its own tests prove them — and the reading it accepts is
    /// the reading that is published. Nothing here re-decides what the pipeline decided or
    /// reshapes what it produced: the fix that goes to the Go half is the fix the pipeline
    /// approved, stamped with the instant the pipeline stamped it with.
    private func feed(_ sample: LocationManager.Sample) {
        guard sensorsRunning else { return }
        hasFix = true

        sink.reset()
        pipeline.location(
            provider: "gps",
            lat: sample.latitude,
            lng: sample.longitude,
            altitude: sample.altitude,
            accuracyMeters: sample.horizontalAccuracy,
            speedKnots: sample.speedKnots,
            courseDeg: sample.courseDegrees,
            valid: true
        )
        guard case .fix(let fix)? = sink.accepted else { return }

        do {
            try ApplianceBridge.feedLocation(
                latitude: fix.latitude,
                longitude: fix.longitude,
                altitude: fix.altitude,
                horizontalAccuracy: fix.accuracyMeters,
                speedKnots: fix.speedKnots,
                courseDegrees: fix.courseDeg,
                timestamp: fix.time
            )
        } catch {
            append("the position was not published: \(error)")
        }
    }

    /// feed offers one orientation reading to the appliance.
    private func feed(_ reading: MotionManager.Reading) {
        guard sensorsRunning else { return }

        let frame: HeadingFrame = reading.trueHeading != nil ? .true : .magnetic
        // A withheld heading is offered as a sample with no heading in it, which is how the
        // pipeline says "not confident"; the reading it accepts back then carries no angle,
        // and the Go half publishes nothing at all rather than a heading of due north.
        hasHeading = !reading.illConditioned && (reading.magnetic != nil || reading.trueHeading != nil)

        sink.reset()
        pipeline.heading(
            reading.illConditioned ? nil : (reading.trueHeading ?? reading.magnetic),
            frame: frame
        )
        guard case .heading(let heading)? = sink.accepted else { return }

        do {
            try ApplianceBridge.feedHeading(
                magnetic: heading.frame == .magnetic ? heading.headingDeg : nil,
                trueHeading: heading.frame == .true ? heading.headingDeg : nil,
                yaw: 0,
                pitch: reading.pitch,
                roll: reading.roll,
                isBackAxis: headingAxis == .screenBack
            )
        } catch {
            append("the heading was not published: \(error)")
        }
    }

    /// append adds a line to the log the controls show.
    private func append(_ line: String) {
        log.append(line)
        if log.count > 200 { log.removeFirst(log.count - 200) }
    }
}

/// ReadingSink holds the reading the pipeline just accepted.
///
/// `SensorPipeline` publishes through a closure fixed when it is created, and it is created
/// once for the life of the appliance. The closure therefore writes to this box rather than
/// capturing the reading that is in flight, and the caller reads the box back immediately
/// after, on the same thread: the pipeline publishes synchronously, from the device callback
/// that offered the sample.
///
/// The box is what carries the pipeline's decision, whole. There is one set of rules on the
/// path — the pipeline's — and the appliance publishes exactly the reading they produced.
private final class ReadingSink: @unchecked Sendable {
    private let lock = NSLock()
    private var reading: SensorReading?

    func reset() {
        lock.lock()
        reading = nil
        lock.unlock()
    }

    func accept(_ accepted: SensorReading) {
        lock.lock()
        reading = accepted
        lock.unlock()
    }

    var accepted: SensorReading? {
        lock.lock()
        defer { lock.unlock() }
        return reading
    }
}
