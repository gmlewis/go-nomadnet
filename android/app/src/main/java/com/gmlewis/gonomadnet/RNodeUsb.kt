// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.hardware.usb.UsbConstants
import android.hardware.usb.UsbDevice
import android.hardware.usb.UsbDeviceConnection
import android.hardware.usb.UsbEndpoint
import android.hardware.usb.UsbInterface
import android.hardware.usb.UsbManager
import android.net.LocalSocket
import android.net.LocalSocketAddress
import android.os.Build
import android.util.Log
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/**
 * A duplex byte stream that can be read from one thread while it is written to from another.
 *
 * It is what the radio's endpoints and the bridge's socket have in common, and it exists so
 * that the code carrying bytes between them can be exercised without a USB device and
 * without an Android runtime: neither of those can be arranged in a unit test, and the pump
 * is the part that decides whether a radio works.
 */
interface ByteStream {
    /** Reads up to `into.size` bytes, blocking. Returns the count, or -1 at the end. */
    fun read(into: ByteArray): Int

    /** Writes `length` bytes, in full. Returns false when the stream has ended. */
    fun write(bytes: ByteArray, length: Int): Boolean

    /** Releases the stream. */
    fun close()
}

/** A USB device the appliance may drive as a radio. */
interface RadioDevice {
    /** How the device is named in the appliance's log. */
    val description: String

    /** Claims the device and returns its byte stream, or null when it cannot be claimed. */
    fun open(): ByteStream?
}

/**
 * Finds the radio on the tablet's USB port.
 *
 * It is an interface so that the bridge's lifecycle — bring the radio up, publish what the
 * transport is dialled at, put it down again — can be asserted with a fake, because a unit
 * test has no USB device and may not have one.
 */
interface RadioFinder {
    /** Returns the radio, or null when none is attached or none may be used. */
    fun find(): RadioDevice?
}

/** One USB vendor and product pair, as the platform's own filter spells it. */
data class UsbId(val vendor: Int, val product: Int) {
    /** The `vvvv:pppp` form the log and the tests read. */
    fun hex(): String = "%04x:%04x".format(vendor, product)
}

/** One endpoint as the platform describes it. */
data class UsbEndpointSpec(val address: Int, val direction: Int, val type: Int, val maxPacketSize: Int)

/** One interface as the platform describes it. */
data class UsbInterfaceSpec(
    val id: Int,
    val klass: Int,
    val subclass: Int,
    val endpoints: List<UsbEndpointSpec>,
)

/**
 * The interface and endpoints a radio's bytes move through.
 *
 * [controlInterfaceId] is the CDC-ACM control interface, which is where the line the radio
 * expects is put into the state it expects. It is null for a device that presents bulk
 * endpoints without a CDC control interface at all.
 */
data class RadioPorts(
    val interfaceId: Int,
    val inEndpoint: UsbEndpointSpec,
    val outEndpoint: UsbEndpointSpec,
    val controlInterfaceId: Int?,
)

/** The USB devices the appliance will drive as a radio. */
object RNodeUsb {

    /**
     * The vendor and product pairs a radio presents.
     *
     * It is the same list `res/xml/usb_device_filter.xml` declares, and a test reads both
     * and fails when they drift: the filter is what makes the system offer the appliance for
     * a device, and this is what the appliance then looks for, so a pair in one and not the
     * other is a radio that is offered and never found, or found and never offered.
     *
     * The one in hand is the first: an Espressif USB JTAG/serial debug unit, which is the
     * ESP32-S3's own USB port and what a Heltec v4 RNode presents. It enumerates as a
     * CDC-ACM device — which is why the kernel creates a `ttyACM` node for it — and the
     * three below it are the bridge chips RNode boards ship with.
     */
    val RADIOS: List<UsbId> = listOf(
        UsbId(0x303a, 0x1001),
        UsbId(0x10c4, 0xea60),
        UsbId(0x1a86, 0x7523),
        UsbId(0x0403, 0x6001),
    )

    /** How long the system's permission dialog is waited for before the radio is given up on. */
    const val PERMISSION_TIMEOUT_MS = 30_000L

    /** How long one bulk transfer may block before it is asked for again. */
    const val TRANSFER_TIMEOUT_MS = 1000

    /** The line speed an RNode's own default is, and the one it is told to expect. */
    const val LINE_SPEED = 115200

    /**
     * The request type of a class request for an interface, in the outgoing direction.
     *
     * Direction is the top bit (0x00 = host to device), type is the next two bits (0x20 =
     * class), and recipient is the low five (0x01 = interface). Both requests below are that
     * shape; a request sent as a standard, device-directed one is a request the radio's
     * control interface ignores, and the radio then never opens its data endpoint.
     */
    const val REQUEST_TYPE_CLASS_INTERFACE_OUT = 0x21

    /** The CDC-ACM class request that sets the line's rate, stop bits, parity and width. */
    const val REQUEST_SET_LINE_CODING = 0x20

    /** The CDC-ACM class request that raises and lowers DTR and RTS. */
    const val REQUEST_SET_CONTROL_LINE_STATE = 0x22

    /** DTR and RTS asserted, which is what an RNode's serial bridge waits for. */
    const val CONTROL_LINE_DTR_AND_RTS = 0x3

    /** How long one control transfer may block. */
    const val CONTROL_TIMEOUT_MS = 1000

    /**
     * chooseRadioPorts picks the interface and endpoints a radio is driven through.
     *
     * A CDC-ACM device — which is what every one of these is, including the Espressif —
     * presents two interfaces: a control interface of class 2 and a data interface of class
     * 10 carrying a bulk pair. The data interface is the one the radio's bytes cross, and
     * the control interface is where the line is configured.
     *
     * A device that presents neither, but does present an interface with one bulk endpoint
     * in each direction, is driven through that interface with no control transfer at all.
     * Nothing else is a radio.
     */
    fun chooseRadioPorts(interfaces: List<UsbInterfaceSpec>): RadioPorts? {
        val control = interfaces.firstOrNull { it.klass == UsbConstants.USB_CLASS_COMM }
        for (iface in interfaces) {
            val bulkIn = iface.endpoints.firstOrNull {
                it.type == UsbConstants.USB_ENDPOINT_XFER_BULK && it.direction == UsbConstants.USB_DIR_IN
            } ?: continue
            val bulkOut = iface.endpoints.firstOrNull {
                it.type == UsbConstants.USB_ENDPOINT_XFER_BULK && it.direction == UsbConstants.USB_DIR_OUT
            } ?: continue
            // The data interface of a CDC-ACM pair is the one with the bulk pair; a
            // vendor-specific interface with a bulk pair is driven the same way, without a
            // line to configure.
            val dataIsCdc = iface.klass == UsbConstants.USB_CLASS_CDC_DATA
            return RadioPorts(
                interfaceId = iface.id,
                inEndpoint = bulkIn,
                outEndpoint = bulkOut,
                controlInterfaceId = if (dataIsCdc) control?.id else null,
            )
        }
        return null
    }

    /**
     * lineCoding is the CDC-ACM SET_LINE_CODING payload: 115200 8N1, little-endian.
     *
     * An RNode speaks its own protocol over the serial line and does not care what rate the
     * host believes the line runs at, but the device class does: a line left in whatever
     * state the interface's reset left it in is a device that will not open its data
     * endpoint. 8 data bits, 1 stop bit, no parity is what `RNodeDefaultSpeed` matches.
     */
    fun lineCoding(speed: Int = LINE_SPEED): ByteArray = byteArrayOf(
        (speed and 0xff).toByte(),
        ((speed shr 8) and 0xff).toByte(),
        ((speed shr 16) and 0xff).toByte(),
        ((speed shr 24) and 0xff).toByte(),
        0, // one stop bit
        0, // no parity
        8, // eight data bits
    )
}

/**
 * The radio, found the way Android requires: by asking the system for it.
 *
 * Android has no udev and no `/dev/serial/by-id`. The kernel's node for a radio is
 * `crw------- root root` and no application may open it, however it is configured. The
 * supported way is this one, which hands back a usbfs descriptor that no path can name —
 * which is why the transport is given a pseudo-terminal instead, and this class is the half
 * that owns the device behind it.
 */
class UsbRadioFinder(private val context: Context) : RadioFinder {

    override fun find(): RadioDevice? {
        val manager = context.getSystemService(Context.USB_SERVICE) as? UsbManager ?: return null
        for (device in manager.deviceList.values) {
            val id = UsbId(device.vendorId, device.productId)
            if (id !in RNodeUsb.RADIOS) {
                continue
            }
            if (!manager.hasPermission(device) && !requestPermission(manager, device)) {
                Log.i(TAG, "the operator did not allow the radio ${id.hex()}; no radio this time")
                return null
            }
            return UsbRadioDevice(manager, device)
        }
        return null
    }

    /**
     * requestPermission asks the system for the device, and waits for the answer.
     *
     * The dialog is the system's and it lands on the person using the tablet, so the wait is
     * bounded and a refusal is reported rather than waited on: an appliance that hung here
     * would be an appliance whose Start stack button did nothing and said nothing.
     *
     * The receiver is registered dynamically and not in the manifest, and it is registered
     * without being exported: it takes an answer about a device the appliance asked for, and
     * nothing outside this app has any business sending it one.
     */
    private fun requestPermission(manager: UsbManager, device: UsbDevice): Boolean {
        val answered = CountDownLatch(1)
        var granted = false
        val receiver = object : BroadcastReceiver() {
            override fun onReceive(receiverContext: Context?, intent: Intent?) {
                if (intent?.action != ACTION_USB_PERMISSION) {
                    return
                }
                granted = intent.getBooleanExtra(UsbManager.EXTRA_PERMISSION_GRANTED, false)
                answered.countDown()
            }
        }
        val filter = IntentFilter(ACTION_USB_PERMISSION)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            context.registerReceiver(receiver, filter, Context.RECEIVER_NOT_EXPORTED)
        } else {
            @Suppress("UnspecifiedRegisterReceiverFlag")
            context.registerReceiver(receiver, filter)
        }
        try {
            // The PendingIntent has to be mutable: the system fills in the grant.
            val flags = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                android.app.PendingIntent.FLAG_UPDATE_CURRENT or android.app.PendingIntent.FLAG_MUTABLE
            } else {
                android.app.PendingIntent.FLAG_UPDATE_CURRENT
            }
            val intent = Intent(ACTION_USB_PERMISSION).setPackage(context.packageName)
            val pending = android.app.PendingIntent.getBroadcast(
                context,
                0,
                intent,
                flags,
            )
            manager.requestPermission(device, pending)
            if (!answered.await(RNodeUsb.PERMISSION_TIMEOUT_MS, TimeUnit.MILLISECONDS)) {
                Log.i(TAG, "the system never answered for ${UsbId(device.vendorId, device.productId).hex()}")
                return false
            }
            return granted
        } catch (interrupted: InterruptedException) {
            Thread.currentThread().interrupt()
            return false
        } finally {
            runCatching { context.unregisterReceiver(receiver) }
        }
    }

    companion object {
        /** The action the system answers a USB permission request with. */
        const val ACTION_USB_PERMISSION = "com.gmlewis.gonomadnet.USB_PERMISSION"

        private const val TAG = "gonomadnet"
    }
}

/** A radio the system has handed over, and the byte stream its endpoints carry. */
private class UsbRadioDevice(
    private val manager: UsbManager,
    private val device: UsbDevice,
) : RadioDevice {

    override val description: String
        get() = "RNode on ${UsbId(device.vendorId, device.productId).hex()}"

    override fun open(): ByteStream? {
        val specs = (0 until device.interfaceCount).map { index -> device.getInterface(index).toSpec() }
        val ports = RNodeUsb.chooseRadioPorts(specs)
        if (ports == null) {
            Log.i(TAG, "$description presents no bulk endpoint pair, so it is not a radio")
            return null
        }
        val dataInterface = device.getInterface(ports.interfaceId)
        val connection = manager.openDevice(device)
        if (connection == null) {
            Log.i(TAG, "the system would not open $description")
            return null
        }
        if (!connection.claimInterface(dataInterface, true)) {
            Log.i(TAG, "$description would not hand over its data interface")
            connection.close()
            return null
        }
        // The line is put into the state the radio expects before its data endpoint is used.
        // A device left in whatever state a reset left it in does not open its data endpoint,
        // and the failure looks exactly like a radio that is not there.
        ports.controlInterfaceId?.let { controlId ->
            val control = device.getInterface(controlId)
            if (connection.claimInterface(control, true)) {
                val coding = RNodeUsb.lineCoding()
                connection.controlTransfer(
                    RNodeUsb.REQUEST_TYPE_CLASS_INTERFACE_OUT,
                    RNodeUsb.REQUEST_SET_LINE_CODING,
                    0,
                    controlId,
                    coding,
                    coding.size,
                    RNodeUsb.CONTROL_TIMEOUT_MS,
                )
                // DTR and RTS asserted: an RNode's serial bridge holds the board in reset
                // until the host says it is talking to it.
                connection.controlTransfer(
                    RNodeUsb.REQUEST_TYPE_CLASS_INTERFACE_OUT,
                    RNodeUsb.REQUEST_SET_CONTROL_LINE_STATE,
                    RNodeUsb.CONTROL_LINE_DTR_AND_RTS,
                    controlId,
                    null,
                    0,
                    RNodeUsb.CONTROL_TIMEOUT_MS,
                )
            }
        }

        val out = dataInterface.endpointAt(ports.outEndpoint.address)
        val input = dataInterface.endpointAt(ports.inEndpoint.address)
        if (out == null || input == null) {
            Log.i(TAG, "$description's data interface has no matching endpoint pair")
            connection.close()
            return null
        }
        // Whether the radio is still there is asked of the system rather than inferred from a
        // transfer: a bulk transfer that moved nothing and one whose device was unplugged
        // both report a failure, and a radio that treated every quiet second as the end would
        // disconnect itself whenever nobody was transmitting.
        return UsbRadioLink(connection, dataInterface, input, out) {
            manager.deviceList.containsKey(device.deviceName)
        }
    }

    private fun UsbInterface.toSpec(): UsbInterfaceSpec = UsbInterfaceSpec(
        id = id,
        klass = interfaceClass,
        subclass = interfaceSubclass,
        endpoints = (0 until endpointCount).map { index ->
            val endpoint = getEndpoint(index)
            UsbEndpointSpec(
                address = endpoint.address,
                direction = endpoint.direction,
                type = endpoint.type,
                maxPacketSize = endpoint.maxPacketSize,
            )
        },
    )

    private fun UsbInterface.endpointAt(address: Int): UsbEndpoint? {
        for (index in 0 until endpointCount) {
            val endpoint = getEndpoint(index)
            if (endpoint.address == address) {
                return endpoint
            }
        }
        return null
    }

    companion object {
        private const val TAG = "gonomadnet"
    }
}

/**
 * The radio's byte stream, over the endpoints the system gave us.
 *
 * A bulk read that times out returns nothing and is not an end: the radio has nothing to say
 * until it has something to say, and treating a quiet second as the end of the stream is a
 * radio that disconnects itself whenever nobody is transmitting.
 */
private class UsbRadioLink(
    private val connection: UsbDeviceConnection,
    private val dataInterface: UsbInterface,
    private val input: UsbEndpoint,
    private val output: UsbEndpoint,
    private val present: () -> Boolean,
) : ByteStream {

    override fun read(into: ByteArray): Int {
        val read = connection.bulkTransfer(input, into, into.size, RNodeUsb.TRANSFER_TIMEOUT_MS)
        if (read > 0) {
            return read
        }
        // Nothing transferred is a radio with nothing to say, which is what a radio that is
        // not being talked to does. It is the end of the stream only when the device is gone
        // as well; see the `present` predicate.
        return if (present()) 0 else -1
    }

    override fun write(bytes: ByteArray, length: Int): Boolean {
        var written = 0
        while (written < length) {
            val chunk = connection.bulkTransfer(output, bytes, written, length - written, RNodeUsb.TRANSFER_TIMEOUT_MS)
            if (chunk <= 0) {
                return present()
            }
            written += chunk
        }
        return true
    }

    override fun close() {
        runCatching { connection.releaseInterface(dataInterface) }
        runCatching { connection.close() }
    }
}

/**
 * The bridge's socket, as the app dials it.
 *
 * The bridge binds an abstract Unix socket — the same name `RNodeSocket.name` derives — and
 * carries the radio's bytes across it. An abstract socket has no filesystem entry, so the
 * address is a bare name and the namespace flag is what says so.
 */
class LocalBridgeChannel(private val name: String) : ByteStream {

    private val socket = LocalSocket()

    /** connect dials the bridge. Returns false when nothing is listening. */
    fun connect(): Boolean = try {
        socket.connect(LocalSocketAddress(name, LocalSocketAddress.Namespace.ABSTRACT))
        true
    } catch (failure: java.io.IOException) {
        Log.i(TAG, "could not dial the radio bridge on $name: ${failure.message}")
        false
    }

    private val input get() = socket.inputStream
    private val output get() = socket.outputStream

    override fun read(into: ByteArray): Int = try {
        input.read(into)
    } catch (failure: java.io.IOException) {
        -1
    }

    override fun write(bytes: ByteArray, length: Int): Boolean = try {
        output.write(bytes, 0, length)
        output.flush()
        true
    } catch (failure: java.io.IOException) {
        false
    }

    /**
     * close ends the channel, and ends a read that is in flight.
     *
     * Closing the socket on its own does not release a thread already blocked in a read:
     * the descriptor goes away underneath it and the thread stays blocked in the kernel.
     * Shutting the input down first is what delivers the end of the stream to that reader,
     * which is here, and the pump is one thread per direction.
     */
    override fun close() {
        runCatching { socket.shutdownInput() }
        runCatching { socket.shutdownOutput() }
        runCatching { socket.close() }
    }

    companion object {
        private const val TAG = "gonomadnet"
    }
}
