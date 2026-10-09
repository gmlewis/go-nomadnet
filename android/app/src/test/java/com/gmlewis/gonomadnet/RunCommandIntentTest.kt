// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The Termux launch contract, asserted against the documented constants rather than against
 * the implementation.
 *
 * This is the piece of the appliance whose failure is silent: a wrong extra name, a missing
 * `<queries>` element, or an ungranted permission all produce a tap that does nothing at all,
 * with nothing on the screen to say why. Asserting every literal here is the only cheap
 * defence against a typo that costs an afternoon of the operator's time.
 */
class RunCommandIntentTest {

    @Test
    fun theActionAndComponentAreTheDocumentedOnes() {
        assertEquals("com.termux.RUN_COMMAND", RunCommandRequest.ACTION)
        assertEquals("com.termux", RunCommandRequest.PACKAGE)
        assertEquals("com.termux.app.RunCommandService", RunCommandRequest.SERVICE)
        assertEquals("com.termux.permission.RUN_COMMAND", RunCommandRequest.PERMISSION)
    }

    @Test
    fun everyExtraNameIsTheDocumentedConstant() {
        assertEquals("com.termux.RUN_COMMAND_PATH", RunCommandRequest.EXTRA_COMMAND_PATH)
        assertEquals("com.termux.RUN_COMMAND_ARGUMENTS", RunCommandRequest.EXTRA_ARGUMENTS)
        assertEquals("com.termux.RUN_COMMAND_WORKDIR", RunCommandRequest.EXTRA_WORKDIR)
        assertEquals("com.termux.RUN_COMMAND_BACKGROUND", RunCommandRequest.EXTRA_BACKGROUND)
        assertEquals("com.termux.RUN_COMMAND_SESSION_ACTION", RunCommandRequest.EXTRA_SESSION_ACTION)
    }

    @Test
    fun aRequestCarriesEveryExtraTermuxNeeds() {
        val request = TermuxLauncher.attachedRequest()
        val extras = request.extras()

        assertEquals(RunCommandRequest.SERVICE.let { "com.termux.app.RunCommandService" }, request.let { RunCommandRequest.SERVICE })
        assertTrue("the command path must be absolute", (extras[RunCommandRequest.EXTRA_COMMAND_PATH] as String).startsWith("/"))
        assertTrue("the arguments must be an array", extras[RunCommandRequest.EXTRA_ARGUMENTS] is Array<*>)
        assertTrue("the working directory must be absolute", (extras[RunCommandRequest.EXTRA_WORKDIR] as String).startsWith("/"))
        assertEquals(false, extras[RunCommandRequest.EXTRA_BACKGROUND])
        assertEquals(
            "the session action is passed as a string, which is what Termux parses",
            RunCommandRequest.SESSION_ACTION_NEW.toString(),
            extras[RunCommandRequest.EXTRA_SESSION_ACTION],
        )
    }

    @Test
    fun theStandaloneRequestTouchesNoStackConfiguration() {
        // Mode A is the configuration that works today, and it must not learn anything about
        // the appliance. The moment its command line mentions the appliance's transport, the
        // fallback has stopped being a fallback.
        val standalone = TermuxLauncher.standaloneRequest()
        assertEquals(listOf("-t"), standalone.arguments)
        assertEquals(TermuxLauncher.TERMUX_HOME + "/gonomadnet", standalone.commandPath)
        assertTrue(
            "the standalone mode must not point at the attached mode's Reticulum config",
            standalone.arguments.none { it.contains(".reticulum-stack") },
        )
    }

    @Test
    fun theAttachedRequestNamesItsOwnReticulumConfiguration() {
        val attached = TermuxLauncher.attachedRequest()
        val configIndex = attached.arguments.indexOf("--rnsconfig")
        assertTrue("the attached mode must name its own RNS config", configIndex >= 0)
        assertEquals(
            TermuxLauncher.TERMUX_HOME + "/.reticulum-stack",
            attached.arguments[configIndex + 1],
        )
        assertTrue(
            "the attached mode's config must not be the standalone mode's",
            attached.arguments[configIndex + 1] != TermuxLauncher.TERMUX_HOME + "/.reticulum",
        )
    }

    @Test
    fun bothModesOpenTheirOwnSessionSoTheyCannotShareAPane() {
        // Two modes sharing a terminal pane would look exactly like one mode, which is the
        // confusing half-broken state both launchers exist to prevent.
        assertEquals(RunCommandRequest.SESSION_ACTION_NEW, TermuxLauncher.standaloneRequest().sessionAction)
        assertEquals(RunCommandRequest.SESSION_ACTION_NEW, TermuxLauncher.attachedRequest().sessionAction)
        assertEquals(1, RunCommandRequest.SESSION_ACTION_NEW)
    }

    @Test
    fun theWorkingDirectoryIsTermuxsHomeAndNotTheAppliances() {
        for (request in listOf(TermuxLauncher.standaloneRequest(), TermuxLauncher.attachedRequest())) {
            assertEquals(TermuxLauncher.TERMUX_HOME, request.workdir)
            assertTrue(
                "the appliance cannot reach into Termux's private home, so the launch must " +
                    "be expressed as a request to Termux rather than a path it writes",
                request.workdir.startsWith("/data/data/com.termux"),
            )
        }
    }

    @Test
    fun theSetupRequestRunsThePublishedScriptThroughTermuxsOwnBash() {
        // Not `/usr/bin/bash`: Android has no such file, and Termux's shell lives under its own
        // prefix. A bare `bash` would not work either — resolving a program name through the
        // dynamic linker is what Android's seccomp policy kills.
        val request = TermuxLauncher.setupRequest("/sdcard/Download/gonomadnet-setup.sh")

        assertEquals("/data/data/com.termux/files/usr/bin/bash", request.commandPath)
        assertEquals(listOf("/sdcard/Download/gonomadnet-setup.sh"), request.arguments)
        assertEquals(TermuxLauncher.TERMUX_HOME, request.workdir)
        // No arguments after the script: it finds what it needs where both applications can see
        // it, and a shared directory passed as an argument would be a second place to get wrong.
        assertEquals(1, request.arguments.size)
    }

    @Test
    fun theSetupSessionIsAlwaysNew() {
        // A setup that wrote into a pane somebody was already reading would be a setup whose
        // output went somewhere else.
        assertEquals(RunCommandRequest.SESSION_ACTION_NEW, TermuxLauncher.setupRequest("/x/s.sh").sessionAction)
    }

    @Test
    fun thePasteLineAndTheSetUpButtonDoTheSameThing() {
        // The two ways in — the button, and the one line a person pastes when the button cannot
        // reach — must describe the same work, or the terminal a person sets up by hand differs
        // from the one the appliance sets up for them.
        val script = "/storage/emulated/0/Download/gonomadnet-setup.sh"

        assertEquals("bash $script", TermuxLauncher.setupLine(script))
        assertEquals(
            TermuxLauncher.setupLine(script).split(' ').last(),
            TermuxLauncher.setupRequest(script).arguments.single(),
        )
    }
}
