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
import CoreBluetooth
import Foundation
import Network

/// RadioManager is the appliance's Bluetooth radio: it finds an RNode, connects to it, and
/// serves the transport a loopback address it can dial.
///
/// An iPad cannot open a USB serial adapter, which is how every other platform reaches an
/// RNode. What it can do is talk to one over Bluetooth Low Energy, which the RNode firmware
/// exposes as the Nordic UART Service: one characteristic the appliance writes to and one it
/// is notified on. That is a byte pipe, and the appliance's shape for a byte pipe is the one
/// it already has for its own console — a socket at one end and the device at the other.
///
/// So the transport is not told about Bluetooth at all. It is handed `tcp://127.0.0.1:37431`,
/// which is the `tcp://` RNode mode it already speaks, and this class serves that address:
/// it listens there, and moves every byte between the transport's connection and the radio's
/// two characteristics without looking at any of them.
///
/// The Bluetooth half of that cannot be tested without a radio, so it is deliberately the
/// thinnest half: the scan rule, the identifiers, the address, and what happens to bytes are
/// all in `ApplianceKit`, where they are settled by unit tests, and what is left here is the
/// calling of CoreBluetooth and Network.
@MainActor
public final class RadioManager: NSObject, ObservableObject {
    /// What the radio is doing, as the controls show it.
    public enum State: Equatable {
        /// Nothing has been asked for, and nothing is being scanned for.
        case idle
        /// A scan is running.
        case scanning
        /// A radio is being connected to.
        case connecting(String)
        /// A radio is paired and the transport can dial it.
        case paired(String)
        /// The radio went away, or would not connect.
        case failed(String)

        /// Whether a radio is paired, which is what the transport is rendered for.
        public var isPaired: Bool {
            if case .paired = self { return true }
            return false
        }

        /// What the controls call this state.
        public var description: String {
            switch self {
            case .idle: return "Off"
            case .scanning: return "Scanning"
            case .connecting(let name): return "Connecting to \(name)"
            case .paired(let name): return name
            case .failed(let reason): return reason
            }
        }
    }

    /// A radio the scan has found.
    public struct Found: Identifiable, Equatable {
        /// The peripheral's identifier, which is what pairing names.
        public let id: UUID
        /// The name its advertisement carries.
        public let name: String
    }

    /// What the radio is doing.
    @Published public private(set) var state: State = .idle

    /// The radios the scan has found, in the order they were seen.
    @Published public private(set) var found: [Found] = []

    /// onLog is called with a line worth showing the reader.
    public var onLog: ((String) -> Void)?

    /// The interface the transport is rendered for a paired radio, or nil for none.
    ///
    /// This is the whole of what the transport knows about the radio: an RNode interface whose
    /// port is the loopback address this class serves.
    public var radioSpec: RNodeSpec? {
        state.isPaired ? RadioAddress.interface() : nil
    }

    private let queue = DispatchQueue(label: "gonomadnet.radio")
    private var central: CBCentralManager?
    private var peripheral: CBPeripheral?
    private var writeCharacteristic: CBCharacteristic?
    private var wantsScan = false

    private var listener: NWListener?
    private var connection: NWConnection?
    private var pump: RadioPump?

    /// startScanning begins looking for a radio, asking for Bluetooth permission the first
    /// time it is used rather than at launch.
    public func startScanning() {
        wantsScan = true
        found = []
        if central == nil {
            // The permission alert is shown by the system when the manager is created, which
            // is why this is not done at launch: a reader who never wants a radio is never
            // asked for one.
            central = CBCentralManager(delegate: self, queue: queue, options: nil)
            return
        }
        beginScanIfPossible()
    }

    /// pair connects to a radio the scan found.
    public func pair(_ id: UUID) {
        guard let central, let target = central.retrievePeripherals(withIdentifiers: [id]).first else {
            state = .failed("that radio is no longer there")
            return
        }
        central.stopScan()
        state = .connecting(target.name ?? "the radio")
        peripheral = target
        target.delegate = self
        central.connect(target, options: nil)
    }

    /// unpair disconnects the radio and stops serving the transport's address.
    public func unpair() {
        tearDown()
        state = .idle
        log("the radio is unpaired")
    }

    // MARK: - The transport's address

    /// startServing listens on the loopback address the transport dials.
    private func startServing() {
        stopServing()
        do {
            let parameters = NWParameters.tcp
            parameters.requiredLocalEndpoint = NWEndpoint.hostPort(
                host: "127.0.0.1",
                port: NWEndpoint.Port(rawValue: UInt16(RadioAddress.port))!
            )
            let listener = try NWListener(using: parameters)
            listener.newConnectionHandler = { [weak self] connection in
                Task { @MainActor in self?.accept(connection) }
            }
            listener.stateUpdateHandler = { [weak self] state in
                if case .failed(let error) = state {
                    Task { @MainActor in
                        self?.state = .failed("the radio's address is not available: \(error)")
                    }
                }
            }
            listener.start(queue: queue)
            self.listener = listener
        } catch {
            state = .failed("the radio's address is not available: \(error)")
        }
    }

    /// stopServing releases the address and the transport's connection to it.
    private func stopServing() {
        connection?.cancel()
        connection = nil
        listener?.cancel()
        listener = nil
    }

    /// accept takes the transport's connection to the radio's address.
    ///
    /// The transport is the client here rather than the server, which is the reverse of a
    /// socket's usual direction and is deliberate: the appliance owns the radio, and the
    /// address is a name for it rather than a service in its own right.
    private func accept(_ connection: NWConnection) {
        // A second connection replaces the first: the transport reconnects when its own
        // interface is restarted, and an old connection that has already been abandoned must
        // not hold the address.
        self.connection?.cancel()
        self.connection = connection
        connection.start(queue: queue)
        receive(from: connection)
    }

    /// receive feeds the transport's bytes to the radio.
    private func receive(from connection: NWConnection) {
        connection.receive(minimumIncompleteLength: 1, maximumLength: 4096) {
            [weak self] data, _, isComplete, error in
            Task { @MainActor in
                guard let self, connection === self.connection else { return }
                if let data, !data.isEmpty {
                    self.pump?.receivedFromTransport([UInt8](data))
                    self.pump?.flush()
                }
                if isComplete || error != nil {
                    self.connection = nil
                    return
                }
                self.receive(from: connection)
            }
        }
    }

    /// send puts the radio's bytes on the transport's connection.
    private func send(toTransport bytes: [UInt8]) {
        guard let connection else { return }
        connection.send(content: Data(bytes), completion: .contentProcessed { _ in })
    }

    // MARK: - Lifecycle

    /// tearDown releases everything the radio holds, in the order that leaves nothing behind.
    private func tearDown() {
        pump?.stop()
        pump = nil
        stopServing()
        peripheral?.delegate = nil
        if let peripheral, peripheral.state != .disconnected {
            central?.cancelPeripheralConnection(peripheral)
        }
        peripheral = nil
        writeCharacteristic = nil
    }

    /// log reports a line the reader can see.
    private func log(_ line: String) {
        onLog?(line)
    }

    /// beginScanIfPossible starts a scan once the radio hardware is actually available.
    private func beginScanIfPossible() {
        guard wantsScan, let central, central.state == .poweredOn else { return }
        state = .scanning
        central.scanForPeripherals(
            withServices: [CBUUID(string: RadioService.service)],
            options: nil
        )
    }
}

// MARK: - CoreBluetooth

extension RadioManager: CBCentralManagerDelegate {
    /// centralDidUpdateState starts the scan once Bluetooth is usable, and says plainly when
    /// it is not.
    public nonisolated func centralManagerDidUpdateState(_ central: CBCentralManager) {
        Task { @MainActor in
            switch central.state {
            case .poweredOn:
                self.beginScanIfPossible()
            case .unauthorized:
                self.state = .failed("Bluetooth permission was refused")
            case .poweredOff:
                self.state = .failed("Bluetooth is off")
            case .unsupported:
                self.state = .failed("this device has no Bluetooth")
            default:
                // A state that is not yet settled is not a failure: the manager reports
                // again when it is.
                break
            }
        }
    }

    /// didDiscover collects a radio's advertisement. A device that advertises the service but
    /// is not named like a radio is somebody else's peripheral, and is not offered.
    public nonisolated func centralManager(
        _ central: CBCentralManager,
        didDiscover peripheral: CBPeripheral,
        advertisementData: [String: Any],
        rssi RSSI: NSNumber
    ) {
        let name = peripheral.name ?? advertisementData[CBAdvertisementDataLocalNameKey] as? String
        guard RadioService.isRadioName(name), let name else { return }
        let found = Found(id: peripheral.identifier, name: name)

        Task { @MainActor in
            guard !self.found.contains(where: { $0.id == found.id }) else { return }
            self.found.append(found)
        }
    }

    /// didConnect reads the radio's service, which is where its two characteristics live.
    public nonisolated func centralManager(
        _ central: CBCentralManager,
        didConnect peripheral: CBPeripheral
    ) {
        peripheral.discoverServices([CBUUID(string: RadioService.service)])
    }

    /// didFailToConnect says why rather than leaving a reader waiting.
    public nonisolated func centralManager(
        _ central: CBCentralManager,
        didFailToConnect peripheral: CBPeripheral,
        error: Error?
    ) {
        Task { @MainActor in
            self.tearDown()
            self.state = .failed("the radio would not connect: \(error.map { "\($0)" } ?? "no reason given")")
        }
    }

    /// didDisconnect is a radio that went away: its address stops being served, because the
    /// next radio's bytes are not this one's.
    public nonisolated func centralManager(
        _ central: CBCentralManager,
        didDisconnectPeripheral peripheral: CBPeripheral,
        error: Error?
    ) {
        Task { @MainActor in
            guard self.peripheral === peripheral else { return }
            self.tearDown()
            self.state = .idle
            self.log("the radio disconnected")
        }
    }
}

extension RadioManager: CBPeripheralDelegate {
    /// didDiscoverServices finds the UART service and reads its characteristics.
    public nonisolated func peripheral(_ peripheral: CBPeripheral, didDiscoverServices error: Error?) {
        guard let service = peripheral.services?.first(where: {
            $0.uuid == CBUUID(string: RadioService.service)
        }) else { return }
        peripheral.discoverCharacteristics(
            [CBUUID(string: RadioService.toRadio), CBUUID(string: RadioService.fromRadio)],
            for: service
        )
    }

    /// didDiscoverCharacteristics subscribes to the radio's notifications and opens the
    /// address the transport dials.
    public nonisolated func peripheral(
        _ peripheral: CBPeripheral,
        didDiscoverCharacteristicsFor service: CBService,
        error: Error?
    ) {
        let characteristics = service.characteristics ?? []
        let toRadio = characteristics.first { $0.uuid == CBUUID(string: RadioService.toRadio) }
        let fromRadio = characteristics.first { $0.uuid == CBUUID(string: RadioService.fromRadio) }

        guard let toRadio, let fromRadio else {
            Task { @MainActor in
                self.tearDown()
                self.state = .failed("the radio has no UART service")
            }
            return
        }

        peripheral.setNotifyValue(true, for: fromRadio)

        Task { @MainActor in
            self.writeCharacteristic = toRadio
            let radio = PeripheralRadio(peripheral: peripheral, characteristic: toRadio)
            self.pump = RadioPump(radio: radio, sink: { [weak self] bytes in
                Task { @MainActor in self?.send(toTransport: bytes) }
            })
            self.startServing()
            self.state = .paired(peripheral.name ?? "the radio")
            self.log("the radio is paired; the transport dials \(RadioAddress.loopback)")
        }
    }

    /// didUpdateValue is the radio's bytes, which go to the transport as they arrived.
    public nonisolated func peripheral(
        _ peripheral: CBPeripheral,
        didUpdateValueFor characteristic: CBCharacteristic,
        error: Error?
    ) {
        guard characteristic.uuid == CBUUID(string: RadioService.fromRadio),
              let data = characteristic.value
        else { return }
        let bytes = [UInt8](data)

        Task { @MainActor in
            self.pump?.receivedFromRadio(bytes)
        }
    }
}

/// PeripheralRadio is a connected radio, as the pump writes to it.
///
/// It exists so that the pump does not know what CoreBluetooth is: everything the pump asks
/// for — the write size and a write — is what this answers with, and a test answers with a
/// fake.
private final class PeripheralRadio: RadioPump.Radio {
    private let peripheral: CBPeripheral
    private let characteristic: CBCharacteristic

    /// The largest write the radio's characteristic accepts.
    ///
    /// CoreBluetooth reports it for a write without a response, which is the mode the
    /// reference implementation uses and the one that keeps a radio's data flowing: a write
    /// with a response waits for the radio to acknowledge each one, and the radio has a
    /// packet to send in the meantime.
    let maxWrite: Int

    init(peripheral: CBPeripheral, characteristic: CBCharacteristic) {
        self.peripheral = peripheral
        self.characteristic = characteristic
        let reported = peripheral.maximumWriteValueLength(for: .withoutResponse)
        self.maxWrite = reported > 0 ? reported : RadioPump.defaultMaxWrite
    }

    func write(_ bytes: [UInt8]) {
        // A characteristic that can only be written with a response is written that way
        // rather than not at all: a slower radio is better than a silent one.
        let type: CBCharacteristicWriteType =
            characteristic.properties.contains(.writeWithoutResponse) ? .withoutResponse : .withResponse
        peripheral.writeValue(Data(bytes), for: characteristic, type: type)
    }
}
