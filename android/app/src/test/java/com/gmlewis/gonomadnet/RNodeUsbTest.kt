// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.hardware.usb.UsbConstants
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/**
 * Choosing the interface a radio's bytes cross is the part of the USB half that can be
 * decided without a USB device, and it is the part most likely to be wrong: a CDC-ACM radio
 * presents two interfaces and drives only one of them, and picking the control interface is
 * a radio that opens and carries nothing.
 */
class RNodeUsbTest {

    private fun endpoint(address: Int, direction: Int, type: Int = UsbConstants.USB_ENDPOINT_XFER_BULK) =
        UsbEndpointSpec(address = address, direction = direction, type = type, maxPacketSize = 64)

    @Test
    fun `a cdc-acm radio is driven through its data interface and configured through its control interface`() {
        // The Espressif USB JTAG/serial unit an RNode presents is a CDC-ACM device — which is
        // why the kernel creates a ttyACM node for it — and so is every one of the bridge
        // chips below it in the filter. Its two interfaces are class 2 (communications) and
        // class 10 (data), and only the second carries the bulk pair the radio's bytes cross.
        val interfaces = listOf(
            UsbInterfaceSpec(
                id = 0,
                klass = UsbConstants.USB_CLASS_COMM,
                subclass = 2,
                endpoints = listOf(endpoint(0x82, UsbConstants.USB_DIR_IN, UsbConstants.USB_ENDPOINT_XFER_INT)),
            ),
            UsbInterfaceSpec(
                id = 1,
                klass = UsbConstants.USB_CLASS_CDC_DATA,
                subclass = 0,
                endpoints = listOf(
                    endpoint(0x81, UsbConstants.USB_DIR_IN),
                    endpoint(0x01, UsbConstants.USB_DIR_OUT),
                ),
            ),
        )
        val ports = RNodeUsb.chooseRadioPorts(interfaces)
        assertNotNull("a CDC-ACM radio's data interface was not found", ports)
        assertEquals("the bytes must cross the data interface", 1, ports!!.interfaceId)
        assertEquals(0x81, ports.inEndpoint.address)
        assertEquals(0x01, ports.outEndpoint.address)
        assertEquals("the line is configured through the control interface", 0, ports.controlInterfaceId)
    }

    @Test
    fun `a radio with no control interface is still driven`() {
        // A board whose bridge chip presents a vendor-specific interface with a bulk pair has
        // no CDC control interface, and there is no line to configure. It is still a radio.
        val interfaces = listOf(
            UsbInterfaceSpec(
                id = 3,
                klass = 0xff,
                subclass = 0,
                endpoints = listOf(
                    endpoint(0x81, UsbConstants.USB_DIR_IN),
                    endpoint(0x02, UsbConstants.USB_DIR_OUT),
                ),
            ),
        )
        val ports = RNodeUsb.chooseRadioPorts(interfaces)
        assertNotNull(ports)
        assertEquals(3, ports!!.interfaceId)
        assertNull("there is no CDC control interface to configure", ports.controlInterfaceId)
    }

    @Test
    fun `nothing else is a radio`() {
        // An interface with only one direction, or only interrupt endpoints, cannot carry the
        // radio's stream. Picking one would claim the device and then move no bytes.
        for (interfaces in listOf(
            emptyList(),
            listOf(UsbInterfaceSpec(0, 0xff, 0, listOf(endpoint(0x81, UsbConstants.USB_DIR_IN)))),
            listOf(UsbInterfaceSpec(0, 0xff, 0, listOf(endpoint(0x01, UsbConstants.USB_DIR_OUT)))),
            listOf(
                UsbInterfaceSpec(
                    0,
                    0xff,
                    0,
                    listOf(
                        endpoint(0x81, UsbConstants.USB_DIR_IN, UsbConstants.USB_ENDPOINT_XFER_INT),
                        endpoint(0x01, UsbConstants.USB_DIR_OUT, UsbConstants.USB_ENDPOINT_XFER_INT),
                    ),
                ),
            ),
        )) {
            assertNull("$interfaces was taken for a radio", RNodeUsb.chooseRadioPorts(interfaces))
        }
    }

    @Test
    fun `the line is put into the state an rnode expects`() {
        // 115200 8N1, little-endian: four bytes of rate, then one stop bit, no parity, eight
        // data bits. A line left in whatever state a reset left it in is a device that does
        // not open its data endpoint, and the failure looks exactly like a radio that is not
        // there.
        assertEquals(
            listOf(0x00, 0xc2, 0x01, 0x00, 0x00, 0x00, 0x08),
            RNodeUsb.lineCoding().map { it.toInt() and 0xff },
        )
        assertEquals(115200, RNodeUsb.LINE_SPEED)
    }

    @Test
    fun `the control requests are the class requests a radio's control interface answers`() {
        // Direction is the top bit of the request type (0x00 = host to device), type is the
        // next two (0x20 = class), and recipient is the low five (0x01 = interface); so a class
        // request for an interface is 0x21. SET_LINE_CODING is request 0x20 and
        // SET_CONTROL_LINE_STATE is 0x22, with DTR and RTS in the low two bits of its value.
        //
        // These four numbers are the whole of how the line is put into the state the radio
        // opens its data endpoint in, and getting them wrong produces a request the control
        // interface silently ignores — which looks exactly like a radio that is not there.
        assertEquals(0x21, RNodeUsb.REQUEST_TYPE_CLASS_INTERFACE_OUT)
        assertEquals(0x20, RNodeUsb.REQUEST_SET_LINE_CODING)
        assertEquals(0x22, RNodeUsb.REQUEST_SET_CONTROL_LINE_STATE)
        assertEquals(0x03, RNodeUsb.CONTROL_LINE_DTR_AND_RTS)
    }

    @Test
    fun `the devices the appliance drives are the devices the manifest offers it for`() {
        // res/xml/usb_device_filter.xml is what makes the system offer the appliance when a
        // radio is plugged in; RNodeUsb.RADIOS is what the appliance then looks for. A pair in
        // one and not the other is a radio that is offered and never found, or found and never
        // offered — and neither failure says anything anywhere.
        val declared = File(RES_DIR, "xml/usb_device_filter.xml").let { file ->
            val document = DocumentBuilderFactory.newInstance()
                .apply { isNamespaceAware = true }
                .newDocumentBuilder()
                .parse(file)
            val devices = document.getElementsByTagName("usb-device")
            (0 until devices.length).map { index ->
                val element = devices.item(index)
                UsbId(
                    vendor = element.attributes.getNamedItem("vendor-id").nodeValue.toInt(),
                    product = element.attributes.getNamedItem("product-id").nodeValue.toInt(),
                )
            }
        }
        assertEquals(
            "the filter the system reads and the list the appliance scans have drifted apart",
            declared,
            RNodeUsb.RADIOS,
        )
    }

    companion object {
        /**
         * The module directory the test runs in, which is where the manifest and the resource
         * files are read from — Gradle runs the JVM tests with the module as the working
         * directory. See MainActivityTest, which reads the manifest the same way.
         */
        private val RES_DIR: File = File("src/main/res")
    }
}
