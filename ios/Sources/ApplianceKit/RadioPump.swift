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

/// Where the transport dials a paired radio.
///
/// A radio on this platform is a Bluetooth peripheral and not a serial device, so the
/// appliance cannot hand the transport a port to open. What it hands over instead is a
/// loopback address it serves itself: the radio bridge listens there, the transport dials
/// it as though it were a network-attached RNode, and the bridge is what moves the bytes
/// to and from the radio. The transport's `tcp://` RNode mode is the same protocol as its
/// serial mode, configuration commands and all, so the address is the whole of the
/// difference.
///
/// The port is stated here beside the appliance's other pinned ports rather than being
/// bound ephemeral, because the configuration that names it is rendered *before* the
/// transport starts and a port that changed between renders would be a configuration the
/// appliance had to rewrite on every launch.
public enum RadioAddress {
    /// The loopback port the radio bridge serves.
    ///
    /// It is its own port and not one of Reticulum's: the shared instance takes 37428, its
    /// control channel 37429, and the sensor feed 37430, and a bridge that took one of
    /// those would leave the transport dialling a radio where it expected its own
    /// instance.
    public static let port = 37431

    /// The address the transport is given for a paired radio.
    public static var loopback: String { "tcp://127.0.0.1:\(port)" }

    /// The transport interface a paired radio is rendered as.
    public static func interface() -> RNodeSpec {
        RNodeSpec(port: loopback)
    }
}

/// What a radio is, as the bridge scans for it.
///
/// The identifiers are the radio's own published service: the Nordic UART Service the
/// RNode firmware exposes over Bluetooth, with one characteristic the appliance writes to
/// and one it subscribes to. They are the same three the reference implementation uses,
/// which is what makes an appliance and a desktop able to talk to the same radio.
public enum RadioService {
    /// The service a radio advertises.
    public static let service = "6E400001-B5A3-F393-E0A9-E50E24DCCA9E"
    /// The characteristic bytes are written to.
    public static let toRadio = "6E400002-B5A3-F393-E0A9-E50E24DCCA9E"
    /// The characteristic the radio notifies on.
    public static let fromRadio = "6E400003-B5A3-F393-E0A9-E50E24DCCA9E"
    /// The name a radio's advertisement carries, which is what a scan shows a reader.
    public static let namePrefix = "RNode "
    /// Whether a radio's advertised name is one the appliance will attach to.
    ///
    /// An advertisement with no name is not a radio this appliance will guess at: an
    /// unnamed device that happens to advertise the service is a stranger, and connecting
    /// to it would be the appliance pairing with something nobody chose.
    public static func isRadioName(_ name: String?) -> Bool {
        guard let name else { return false }
        return name.hasPrefix(namePrefix)
    }
}

/// The bridge between a transport's TCP connection and a radio, with no Bluetooth in it.
///
/// It is a byte pump in both directions and it is deliberately **transparent**. The RNode
/// protocol — KISS framing, the detect and firmware-version handshake, the radio
/// configuration commands, and the data frames — is spoken by the transport above the
/// bridge and answered by the firmware below it, so a bridge that framed, split or reordered
/// anything would be a second framer in the middle of a conversation it is not a party to.
/// Bytes survive it exactly: a frame the transport writes is handed to the radio in as many
/// writes as the radio's characteristic accepts, and the radio's notifications are handed back
/// to the transport as the one stream they are.
///
/// It holds no Bluetooth and no socket, so what it does with bytes is settled in a unit test
/// against a fake radio, with no device and no radio present.
public final class RadioPump: @unchecked Sendable {
    /// A radio, as the pump writes to it.
    ///
    /// This is the whole of what the pump needs from CoreBluetooth, which is what lets it be
    /// tested without one: the real implementation writes to a `CBCharacteristic` and reports
    /// the negotiated write size, and a test's implementation collects what it was given.
    public protocol Radio: AnyObject {
        /// The largest write the radio's characteristic accepts without a response.
        var maxWrite: Int { get }
        /// write hands bytes to the radio.
        func write(_ bytes: [UInt8])
    }

    /// The pump's own bound, for a radio that reports no write size at all.
    public static let defaultMaxWrite = 244

    private let radio: Radio
    private let sink: ([UInt8]) -> Void

    /// The lock guards the outbound queue, which the transport's reader fills and the
    /// radio's writer drains; the radio's notifications arrive on CoreBluetooth's own queue
    /// and are handed straight to the sink.
    private let lock = NSLock()
    private var pending: [UInt8] = []
    private var start = 0
    private var stopped = false

    /// Creates a pump over a radio and a sink the transport's bytes are handed to.
    public init(radio: Radio, sink: @escaping ([UInt8]) -> Void) {
        self.radio = radio
        self.sink = sink
    }

    /// receivedFromTransport takes bytes the transport wrote to the loopback address.
    public func receivedFromTransport(_ bytes: [UInt8]) {
        guard !bytes.isEmpty else { return }
        lock.lock()
        if stopped {
            lock.unlock()
            return
        }
        pending.append(contentsOf: bytes)
        lock.unlock()
    }

    /// receivedFromRadio takes bytes the radio notified.
    ///
    /// They go to the transport as they arrived, in one piece: the transport reads a byte
    /// stream, and a notification boundary is not a frame boundary.
    public func receivedFromRadio(_ bytes: [UInt8]) {
        guard !bytes.isEmpty else { return }
        sink(bytes)
    }

    /// pendingBytes is how many bytes are waiting to be written to the radio.
    public var pendingBytes: Int {
        lock.lock()
        defer { lock.unlock() }
        return pending.count - start
    }

    /// flush writes everything queued to the radio, in the largest writes it accepts.
    ///
    /// A write that splits a frame is correct: the transport framed it, the firmware
    /// deframes it, and the split is the characteristic's business rather than the
    /// protocol's.
    public func flush() {
        while let chunk = nextChunk() {
            radio.write(chunk)
        }
    }

    /// nextChunk takes the next write for the radio, or nil when nothing is queued.
    private func nextChunk() -> [UInt8]? {
        let width = radio.maxWrite > 0 ? radio.maxWrite : Self.defaultMaxWrite

        lock.lock()
        defer { lock.unlock() }
        guard start < pending.count else {
            // Nothing left: let the queue go rather than hold a buffer for a radio that may
            // never write again.
            pending.removeAll(keepingCapacity: false)
            start = 0
            return nil
        }
        let end = min(start + width, pending.count)
        let chunk = Array(pending[start..<end])
        start = end
        if start == pending.count {
            pending.removeAll(keepingCapacity: false)
            start = 0
        }
        return chunk
    }

    /// stop refuses further traffic in either direction.
    ///
    /// It is what a radio being unpaired does: the bytes of a radio that has gone are not
    /// the bytes of the radio that replaces it, and a transport still being fed from a
    /// connection nobody owns is the failure this exists to prevent.
    public func stop() {
        lock.lock()
        stopped = true
        pending.removeAll(keepingCapacity: false)
        start = 0
        lock.unlock()
    }
}
