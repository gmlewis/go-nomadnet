// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * The settings the appliance states in the client's own configuration.
 *
 * The client's configuration ships describing a desktop: no node, no name, and an editor
 * that has to be installed. On an appliance all three are wrong, and two of them fail
 * silently — a console that answers "This instance is not hosting a node", and a button
 * that starts nothing. What the appliance writes is the client's own default with three
 * values changed, which makes the template part of the contract: these tests read the real
 * `default.conf` out of the Go module rather than a fixture, so a template that stops
 * naming the keys fails here instead of on a tablet.
 */
class ClientConfigTest {

    @Test
    fun `the clients own default is made to host a named node with an editor`() {
        val patched = ClientConfig.settings(template(), EDITOR)

        assertTrue(
            "the client would still serve nothing:\n$patched",
            patched.contains("enable_node = yes"),
        )
        assertTrue(
            "the node has no name to announce:\n$patched",
            patched.contains("node_name = ${ClientConfig.NODE_NAME}"),
        )
        assertTrue(
            "the editor is not the one this APK carries:\n$patched",
            patched.contains("editor = $EDITOR"),
        )
        for (unset in listOf("enable_node = no", "node_name = None", "editor = nano")) {
            assertFalse("the default still says \"$unset\", so the rewrite missed it", patched.contains(unset))
        }
    }

    @Test
    fun `the rest of the configuration is the clients own, comments included`() {
        // The point of starting from the client's default rather than from a file written
        // here: an operator who opens the editor gets the documented configuration, not
        // three lines and a surprise. Every line but the three is the client's.
        val template = template()
        val patched = ClientConfig.settings(template, EDITOR)

        val changed = template.lines().zip(patched.lines()).filter { (was, now) -> was != now }
        assertEquals(
            "the rewrite changed more than the three settings: $changed",
            // In the file's own order: the editor is in [textui], which is written above
            // the node's section.
            listOf("editor = nano", "enable_node = no", "node_name = None"),
            changed.map { (was, _) -> was },
        )
        assertEquals(
            "lines were added or dropped, so an operator's file is not the client's",
            template.lines().size,
            patched.lines().size,
        )
        assertTrue("the comments were rewritten:\n$patched", patched.contains("# Whether to enable node hosting"))
        assertTrue(patched.contains("colormode = 24bit"))
    }

    @Test
    fun `a key in another section is not the key this sets`() {
        // The keys are read out of their own sections and nowhere else, so a rewrite that
        // scanned the whole document would be changing values nobody asked it to change the
        // first time another section grew a key of the same name.
        val elsewhere = listOf(
            "[client]",
            "enable_node = no",
            "node_name = None",
            "",
            "[textui]",
            "editor = nano",
            "",
            "[node]",
            "enable_node = no",
            "node_name = None",
        ).joinToString("\n") + "\n"

        assertEquals(
            "a key of another section was rewritten",
            listOf(
                "[client]",
                "enable_node = no",
                "node_name = None",
                "",
                "[textui]",
                "editor = $EDITOR",
                "",
                "[node]",
                "enable_node = yes",
                "node_name = ${ClientConfig.NODE_NAME}",
            ).joinToString("\n") + "\n",
            ClientConfig.settings(elsewhere, EDITOR),
        )
    }

    @Test
    fun `the shape of the template is the templates`() {
        // A key's indentation is kept. The file is the client's, and a rewrite that
        // reformatted it would make every future diff against the client's own default
        // meaningless — including the diff of an operator's own edits.
        val indented = listOf(
            "[textui]",
            "\teditor = nano",
            "",
            "[node]",
            "  enable_node = no",
            "  node_name = None",
        ).joinToString("\n") + "\n"

        assertEquals(
            listOf(
                "[textui]",
                "\teditor = $EDITOR",
                "",
                "[node]",
                "  enable_node = yes",
                "  node_name = ${ClientConfig.NODE_NAME}",
            ).joinToString("\n") + "\n",
            ClientConfig.settings(indented, EDITOR),
        )
    }

    @Test
    fun `a template that no longer names the keys is refused`() {
        // The failure this prevents is the quiet one: a configuration that looks right, is
        // written, and hosts nothing. A template that has been renamed or re-sectioned has to
        // stop the write and say so.
        val renamed = "[node]\nenable_node = no\nnode_naming = None\n"
        val failure = assertThrows(IllegalArgumentException::class.java) {
            ClientConfig.settings(renamed, EDITOR)
        }
        assertTrue(
            "the refusal must name the key it could not find: ${failure.message}",
            failure.message!!.contains("node_name"),
        )

        assertThrows(IllegalArgumentException::class.java) {
            ClientConfig.settings("[node]\nnode_name = None\n", EDITOR)
        }
        assertThrows(IllegalArgumentException::class.java) {
            ClientConfig.settings("[node]\nenable_node = no\nnode_name = None\n", EDITOR)
        }
    }

    @Test
    fun `a fresh install is written the settings before the client ever runs`() {
        val path = File(tempDir(), "etc/nomadnetwork/config").absolutePath
        assertTrue("the file was not written on a fresh install", ClientConfig.ensure(path, template(), EDITOR))
        assertTrue(File(path).readText().contains("node_name = ${ClientConfig.NODE_NAME}"))
        assertTrue(File(path).readText().contains("editor = $EDITOR"))
    }

    @Test
    fun `a configuration nobody has edited is upgraded, and one that has is left alone`() {
        // An install that ran the client before the appliance wrote anything has the
        // client's own default on disk — byte-identical to the template — and that is a
        // configuration nobody has made a decision in. A file somebody has edited keeps
        // their edits, including a file in which they switched node hosting back off.
        val dir = tempDir()
        val untouched = File(dir, "untouched").apply { writeText(template()) }
        val edited = File(dir, "edited")
            .apply { writeText(template().replace("enable_node = no", "enable_node = no\nnode_extra = mine")) }
        val off = File(dir, "off")
            .apply { writeText(ClientConfig.settings(template(), EDITOR).replace("enable_node = yes", "enable_node = no")) }

        assertTrue("an untouched default was not upgraded", ClientConfig.ensure(untouched.absolutePath, template(), EDITOR))
        assertTrue(untouched.readText().contains("enable_node = yes"))

        assertTrue("an edited file's default editor was not re-pointed", ClientConfig.ensure(edited.absolutePath, template(), EDITOR))
        assertTrue("the operator's own line was lost", edited.readText().contains("node_extra = mine"))
        assertFalse("an operator's file was given a node it had switched off", edited.readText().contains("enable_node = yes"))

        assertFalse(
            "a file whose settings are already the appliance's was rewritten",
            ClientConfig.ensure(off.absolutePath, template(), EDITOR),
        )
        assertTrue(off.readText().contains("enable_node = no"))
    }

    @Test
    fun `the editor is re-pointed when this install moved it`() {
        // The APK's native library directory changes with every install, so a path written
        // by an earlier one names a file that is not there — and "Open Editor" then starts
        // nothing at all, which is the failure the editor setting exists to remove.
        val moved = "/data/app/~~old==/com.gmlewis.gonomadnet-old==/lib/arm64/lib${ClientConfig.EDITOR_NAME}.so"
        val stale = ClientConfig.settings(template(), moved)

        assertTrue(ClientConfig.retargetEditor(stale, EDITOR).contains("editor = $EDITOR"))
        assertFalse(ClientConfig.retargetEditor(stale, EDITOR).contains(moved))
    }

    @Test
    fun `an editor an operator chose is never overwritten`() {
        // The one value in the file that is a person's choice about how they work. Whatever
        // they set it to — including a name this appliance has never heard of — is left.
        for (chosen in listOf("vim", "/usr/bin/emacs", "/data/data/com.termux/files/usr/bin/nano", "my-editor")) {
            val text = ClientConfig.settings(template(), EDITOR).replace("editor = $EDITOR", "editor = $chosen")
            assertEquals(
                "the operator's own editor was overwritten",
                text,
                ClientConfig.retargetEditor(text, EDITOR),
            )
        }
    }

    @Test
    fun `the editors the appliance may replace are the ones nobody chose`() {
        // The client's own default, and the alias Python resolves on the machine it runs on.
        // Both name an editor this device does not have.
        for (unset in listOf("nano", "editor")) {
            val text = ClientConfig.settings(template(), EDITOR).replace("editor = $EDITOR", "editor = $unset")
            assertTrue(
                "a configuration still saying \"$unset\" keeps an editor this device has not got",
                ClientConfig.retargetEditor(text, EDITOR).contains("editor = $EDITOR"),
            )
        }
    }

    @Test
    fun `nothing is left beside the configuration`() {
        // The write goes through a staging file and a rename, because a process killed while
        // it writes must not leave a truncated configuration the client will read. What must
        // not be left is the staging file itself: the client's directory holds its identity
        // and its store, and a stray file there is a file nobody can explain.
        val dir = tempDir()
        val path = File(dir, "config").absolutePath
        assertTrue(ClientConfig.ensure(path, template(), EDITOR))
        assertEquals(
            "a staging file was left behind: ${dir.list()?.joinToString()}",
            listOf("config"),
            dir.list()?.sorted(),
        )
    }

    @Test
    fun `the template the appliance reads is the clients own default`() {
        // The asset is the Go module's file, added to the APK's assets by the build, so
        // these tests reading the same path is what makes them a check on the packaged file
        // rather than on a fixture. The directory holding it is packaged whole, which is why
        // it must hold nothing else.
        assertTrue("the client's default configuration is not there: $TEMPLATE_FILE", TEMPLATE_FILE.isFile)
        assertEquals(
            "the asset the appliance opens by name is not the file the build packages",
            TEMPLATE_FILE.name,
            ClientConfig.TEMPLATE_ASSET_NAME,
        )
        assertEquals(
            "the packaged directory holds more than the client's default, so the APK now " +
                "carries files nothing reads",
            listOf(TEMPLATE_FILE.name),
            TEMPLATE_FILE.parentFile?.list()?.sorted(),
        )
    }

    @Test
    fun `the appliance's editor is the program the APK carries`() {
        // The name is spelled in two places — the client's configuration and the build that
        // puts the binary in the APK — and a client pointed at a name the package does not
        // carry is a button that starts nothing.
        assertEquals(
            "the editor is not looked for where the package puts it",
            "/data/app/~~7Yq==/com.gmlewis.gonomadnet-2Qw==/lib/arm64/lib${ClientConfig.EDITOR_NAME}.so",
            LaunchSpecs.binary("/data/app/~~7Yq==/com.gmlewis.gonomadnet-2Qw==/lib/arm64", ClientConfig.EDITOR_NAME),
        )
    }

    /** template is the client's default configuration, as the APK will carry it. */
    private fun template(): String = TEMPLATE_FILE.readText()

    /** tempDir is a directory for one test, removed with it. */
    private fun tempDir(): File = File("/tmp", "client-config-${System.nanoTime()}").apply {
        mkdirs()
        deleteOnExit()
    }

    private companion object {
        /** The client's default configuration, in the Go module that owns it. */
        val TEMPLATE_FILE = File("../../nomadnet/config/testdata/default.conf")

        /** Where the editor lands in `nativeLibraryDir`, as the appliance names it. */
        const val EDITOR = "/data/app/~~7Yq==/com.gmlewis.gonomadnet-2Qw==/lib/arm64/libgonomadnetedit.so"
    }
}
