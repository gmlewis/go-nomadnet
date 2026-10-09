// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.File
import java.io.IOException

/**
 * The settings the appliance states in the client's own configuration.
 *
 * The client is a program with a configuration file of its own, written from the Go
 * project's default on its first run, and that default describes a desktop: a client that
 * serves no pages, a node with no name, and an editor that has to be installed. An
 * appliance is none of those things, so the appliance states the three values before the
 * client has ever run:
 *
 *   - `enable_node = yes`, because the appliance is a node and its console otherwise
 *     answers "This instance is not hosting a node";
 *   - `node_name = gonomadnet on Android`, so that its node can be told apart from every
 *     desktop node it appears beside;
 *   - `editor = <the editor this APK carries>`, because Android gives an application no
 *     editor to run and no way to look one up.
 *
 * The file is written from the client's own default rather than from a second copy of it:
 * the template is the `default.conf` the Go client embeds and writes on its first run,
 * taken from the module that owns it when the APK is built. A configuration written here
 * is therefore complete — an operator who opens the editor sees the documented file they
 * would have got from the client — with three values changed.
 *
 * The client owns its configuration from the moment it has run. Two rules keep that true:
 * the values above are written only into a file nobody has edited, and the editor is
 * re-pointed only at a value the appliance itself wrote (or the client's own default). A
 * value an operator chose is never overwritten.
 */
object ClientConfig {

    /**
     * The client's default configuration, as a name inside the APK's assets.
     *
     * It is the Go client's own `nomadnet/config/testdata/default.conf`, which the build
     * takes from that directory rather than duplicating here: the embedded default and the
     * one written out on a first run are the same bytes, and a copy kept beside this file
     * would be a second default that drifts from the first.
     */
    const val TEMPLATE_ASSET_NAME = "default.conf"

    /**
     * The name the appliance's node announces and shows to peers.
     *
     * It says which appliance this is rather than what the node is for: the tablet's node
     * appears in other people's node lists beside desktop nodes, and "gonomadnet on
     * Android" is the one name that tells them apart without a lookup.
     */
    const val NODE_NAME = "gonomadnet on Android"

    /**
     * The editor the APK carries, as a name inside `nativeLibraryDir`.
     *
     * The client runs it on its configuration file when the Config page's "Open Editor" is
     * used — the same program Python's EditorTerminal starts, except that this one travels
     * with the appliance.
     */
    const val EDITOR_NAME = "gonomadnetedit"

    /** The section the node's settings live in. */
    private const val NODE_SECTION = "node"

    /** The key that turns page serving on. */
    private const val ENABLE_NODE = "enable_node"

    /** The key that names the node in announces. */
    private const val NODE_NAME_KEY = "node_name"

    /** The section the editor setting lives in. */
    private const val TEXTUI_SECTION = "textui"

    /** The key naming the editor the Config page opens. */
    private const val EDITOR_KEY = "editor"

    /**
     * The values of `editor` that name no editor this machine will ever have: the client's
     * own default, and the alias Python resolves to a program on the machine.
     *
     * A configuration that still says one of these is one nobody has chosen an editor in,
     * which is what makes it safe to point at the appliance's own.
     */
    private val EDITOR_DEFAULTS = setOf("nano", "editor")

    /** The suffix that marks an editor path as one this appliance wrote earlier. */
    private val EDITOR_SUFFIX = "/lib$EDITOR_NAME.so"

    /**
     * settings is [template] with the appliance's node settings and its editor.
     *
     * Every other line is the client's own, comments included, so the file an operator
     * reads is the file the client would have written with three values different. Only the
     * two sections the keys live in are looked at: `enable_node` is not a key in any other
     * section, and a rewrite that scanned the whole document would change a value it had
     * not understood the first time another section grew a key of the same name.
     *
     * A template that no longer carries the keys is refused rather than patched: the
     * alternative is a file that looks correct and hosts nothing, which is the failure this
     * whole mechanism exists to remove.
     */
    fun settings(template: String, editor: String): String {
        val served = rewrite(template, NODE_SECTION, setOf(ENABLE_NODE, NODE_NAME_KEY)) { key, _ ->
            when (key) {
                ENABLE_NODE -> "yes"
                NODE_NAME_KEY -> NODE_NAME
                else -> null
            }
        }
        return rewrite(served, TEXTUI_SECTION, setOf(EDITOR_KEY)) { key, _ ->
            if (key == EDITOR_KEY) editor else null
        }
    }

    /**
     * ensure writes the appliance's settings to [path], and reports whether it wrote.
     *
     * A file that is absent, or that is still byte-identical to the template, is the
     * client's untouched default and is replaced: that is a first run, and a configuration
     * nobody has made a decision in. Any other file is the operator's, and only the editor
     * path is looked at in it — a path this appliance wrote earlier has to be re-pointed
     * when the APK is updated, because the directory the binary lives in changes with the
     * install, and a stale path is an "Open Editor" button that does nothing.
     *
     * The write goes through a staging file and a rename, because a process killed while it
     * writes must not leave a truncated configuration where the client will read it.
     */
    fun ensure(path: String, template: String, editor: String): Boolean {
        val file = File(path)
        val existing = if (file.isFile) file.readText() else null
        val wanted = if (existing == null || existing == template) {
            settings(template, editor)
        } else {
            retargetEditor(existing, editor)
        }
        if (existing == wanted) {
            return false
        }
        val staging = File("$path${RealBundledAssets.STAGING_SUFFIX}")
        return try {
            file.parentFile?.mkdirs()
            staging.writeText(wanted)
            if (staging.renameTo(file)) {
                true
            } else {
                staging.delete()
                false
            }
        } catch (failed: IOException) {
            staging.delete()
            false
        }
    }

    /**
     * retargetEditor points the `editor` setting at [editor], and leaves everything else.
     *
     * It rewrites only the two values that are nobody's choice: the client's default, and a
     * path this appliance wrote on an earlier install. An operator who set their own editor
     * keeps it, whatever it is.
     */
    fun retargetEditor(text: String, editor: String): String = rewrite(text, TEXTUI_SECTION, emptySet()) { key, current ->
        if (key == EDITOR_KEY && (current in EDITOR_DEFAULTS || current.endsWith(EDITOR_SUFFIX))) {
            editor
        } else {
            null
        }
    }

    /**
     * rewrite replaces the values of the keys of one section, and refuses the result if a
     * key of [required] was not in it.
     *
     * [value] is given each key of the section and its current value, and returns the value
     * to write or null to leave the line alone, which is how a rule that applies to some of
     * a section's keys is expressed. The key and its spacing are the file's: only the value
     * is written, so a template that indents or spaces its lines is not reformatted.
     */
    private fun rewrite(
        text: String,
        section: String,
        required: Set<String>,
        value: (String, String) -> String?,
    ): String {
        val lines = text.lines().toMutableList()
        var inSection = false
        val found = mutableSetOf<String>()
        for ((index, line) in lines.withIndex()) {
            val trimmed = line.trim()
            if (trimmed.startsWith("[") && trimmed.endsWith("]")) {
                inSection = trimmed.substring(1, trimmed.length - 1).trim() == section
                continue
            }
            if (!inSection || trimmed.startsWith("#") || '=' !in trimmed) {
                continue
            }
            val key = trimmed.substringBefore('=').trim()
            val replacement = value(key, trimmed.substringAfter('=').trim()) ?: continue
            lines[index] = line.substringBefore('=') + "= " + replacement
            found += key
        }
        val missing = required - found
        require(missing.isEmpty()) {
            "the client's default configuration states no ${missing.joinToString()} in " +
                "[$section], so the appliance cannot set it"
        }
        return lines.joinToString("\n")
    }
}
