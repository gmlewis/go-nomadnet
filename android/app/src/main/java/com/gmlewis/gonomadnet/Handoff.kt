// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.annotation.TargetApi
import android.content.ContentResolver
import android.content.ContentValues
import android.graphics.Paint
import android.graphics.Typeface
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import java.io.File
import java.io.IOException
import java.io.InputStream

/**
 * The client binary the appliance carries for Termux.
 *
 * `gonomadnet` is a terminal application and Termux is the terminal that runs it, so the
 * appliance's half of the installation is handing over a binary — and the binary it hands over
 * is the one it was built beside, cross-compiled for the same device ABI as its own daemons.
 *
 * Carrying it is the difference between an installation that is a button and one that is a
 * treasure hunt: the release page holds a dozen assets for a dozen machines, and telling an
 * `arm64-v8a` tablet from an `armeabi-v7a` one is not a question to put to somebody who just
 * wants the program to start.
 *
 * It is an asset rather than a bundled `.so` because nothing executes it from here: it runs in
 * Termux, under Termux's uid, from Termux's home directory, and all this appliance can do is
 * put a copy where Termux is able to read it.
 */
const val CLIENT_ASSET_NAME = "gonomadnet-client"

/** The MIME type the client binary is published as. */
const val CLIENT_MIME_TYPE = "application/octet-stream"

/**
 * The script that installs everything published here into Termux.
 *
 * Termux reads its terminal font, its colors and its properties from inside its own private
 * data directory, which Android gives this appliance no way to write. So the files are
 * published where both applications can read them, and this script — running as Termux, the
 * only process that can — puts them where Termux looks.
 *
 * It is a file rather than a command built here for two reasons: it can be read before it is
 * run by anybody who wants to know what it does to their terminal, and its length is not
 * bounded by what fits in an intent.
 */
const val SETUP_SCRIPT_ASSET_NAME = "gonomadnet-setup.sh"

/** The MIME type the setup script is published as. */
const val SETUP_SCRIPT_MIME_TYPE = "text/plain"

/**
 * The launcher the home-screen icon runs in standalone mode.
 *
 * It is installed into Termux's `~/.shortcuts`, which is what `Termux:Widget` runs a target
 * script from. The appliance has its own buttons that start the same mode, so the icons are a
 * convenience rather than the way in — but the launcher is what refreshes the hub's address
 * before the client starts, and that is worth having whichever way the client is started.
 */
const val STANDALONE_LAUNCHER_ASSET_NAME = "gonomadnet-standalone"

/** Where a Termux:Widget target script lands inside Termux's home directory. */
const val STANDALONE_SHORTCUT_NAME = "gonomadnet"

/**
 * The launcher for the attached mode, where the client uses this appliance's transport.
 *
 * Its configuration declares no interfaces of its own: the appliance's bundled transport owns
 * every interface on the tablet, and a second one would build a second, invisible Reticulum
 * network on the same device.
 */
const val STACK_LAUNCHER_ASSET_NAME = "gonomadnet-stack"

/** The Reticulum configuration the attached launcher reads. */
const val STACK_CONFIG_ASSET_NAME = "reticulum-stack-config"

/** The shared-instance setting the attached configuration has to agree with this appliance on. */
const val STACK_CONFIG_SHARED_INSTANCE_PORT = 37428

/** Where the attached launcher's Reticulum configuration is installed. */
const val STACK_CONFIG_INSTALLED_NAME = ".reticulum-stack/config"

/** The MIME type the launchers and the attached configuration are published as. */
const val TERMUX_TEXT_MIME_TYPE = "text/plain"

/**
 * What the setup script wrote about its own run.
 *
 * Termux's home directory can be read by neither this appliance nor `adb`, so shared storage is
 * the only channel on which a setup that half worked can be told from one that did nothing —
 * and the script writes to it for exactly that reason.
 */
const val SETUP_STATUS_FILE_NAME = "gonomadnet-setup-status.txt"

/**
 * The Nerd Font the terminal needs for the user interface's glyphs.
 *
 * The text user interface draws its icons from the Nerd Font private-use area by default —
 * `[textui] glyphs = nerdfont` is the shipped setting — and a terminal whose font has no such
 * glyphs draws an empty box for every one of them, which is what the appliance's own menu bar
 * does under Termux's default font.
 *
 * This is **Atkynson Mono Nerd Font Mono**, which is Nerd Fonts' patched Atkinson Hyperlegible
 * Mono — a monospaced face designed for low vision, and the name Nerd Fonts publishes it under
 * because the upstream reserves its own name against modified versions. The "Mono" variant is
 * the right one here and not a preference: the plain Nerd Font variant draws its icons
 * double-width, and this interface lays its columns out in cells, so a double-width icon would
 * push every box to the right of it out of alignment.
 *
 * The upstream is the SIL Open Font License 1.1, which is why [TERMINAL_FONT_LICENSE_FILE_NAME]
 * travels with it: the license requires its notice and its text to accompany every copy.
 * The name table still carries the wording of Atkinson Hyperlegible's older, pre-OFL
 * Braille Institute license, which the 2024 mono release replaced with OFL 1.1.
 */
const val TERMINAL_FONT_FILE_NAME = "AtkynsonMonoNerdFontMono-Regular.otf"

/** The license the font is redistributed under, which is published beside it. */
const val TERMINAL_FONT_LICENSE_FILE_NAME = "AtkinsonHyperlegibleMono-OFL.txt"

/**
 * The tmux configuration the appliance hands to Termux.
 *
 * The client runs inside Termux, where tmux is available as a package, and it reads its
 * configuration from `~/.tmux.conf`. This is the operator's own settings — status bar on top
 * with blue accents, windows numbered from one, true color passed through to the programs in a
 * pane — so that a pane on the tablet looks like a pane on the desktop.
 *
 * tmux itself is not bundled: it is a Termux package, installed once with `pkg install tmux`.
 */
const val TMUX_CONFIG_FILE_NAME = "tmux.conf"

/** The MIME type the tmux configuration is published as. */
const val TMUX_CONFIG_MIME_TYPE = "text/plain"

/**
 * The terminal colors the appliance hands to Termux.
 *
 * The client draws most of its chrome — frame borders, pane titles, the key hints along the
 * bottom — in the terminal's *default* foreground and background, and its accents from the
 * terminal's own 16-color palette. Those are resolved by whichever terminal is drawing, so the
 * same client under Termux's stock white-on-black palette and under the desktop's themed
 * terminal renders the same interface in visibly different colors. This file is the desktop's
 * theme in the form Termux reads.
 *
 * Termux reads it from `~/.termux/colors.properties` and applies it on the next
 * `termux-reload-settings`, which is a program Termux ships with.
 */
const val TERMINAL_COLORS_FILE_NAME = "colors.properties"

/** The MIME type the terminal colors are published as. */
const val TERMINAL_COLORS_MIME_TYPE = "text/plain"

/**
 * The MIME type an OpenType font is published as.
 *
 * It is the type of the file and not the name it ends up with: MediaStore stores a file under
 * the extension Android's own type table gives the type it was handed, and that table gives
 * `font/otf` the extension `.ttf`. The name the font really has is read back from the stored
 * row rather than predicted.
 */
const val TERMINAL_FONT_MIME_TYPE = "font/otf"

/** The MIME type the license is published as. */
const val TERMINAL_FONT_LICENSE_MIME_TYPE = "text/plain"

/**
 * Where a published file appears as a path.
 *
 * Termux is a second application, so it is told a path and not a `content://` URI. This is the
 * public Downloads directory every Android device has, and the one Termux's own
 * `termux-setup-storage` exposes as `~/storage/downloads`.
 */
const val SHARED_DOWNLOADS_DIR = "/sdcard/Download"

/**
 * Writes a file where another application can read it.
 *
 * The appliance cannot install these files itself. Termux reads its terminal font from
 * `~/.termux/font.ttf` inside its own private data directory, and Android gives one
 * application no way at all to write into another's. Publishing into the shared Downloads
 * collection is the platform's intended hand-off from one application to another, and from
 * Android 10 on it needs no permission from either side.
 */
fun interface SharedDownloads {
    /**
     * Publishes [source] as a file named [name], returning its path as another application
     * sees it.
     *
     * Returns null when nothing was published. That is a report and never a throw: a font the
     * tablet does not have is a terminal that draws boxes, not a reason to end the process.
     */
    fun publish(name: String, mimeType: String, source: InputStream): String?
}

/**
 * What Android's own font engine makes of a font.
 *
 * Termux draws its terminal through `Typeface`, the same engine every other Android
 * application draws text with, so asking that engine here answers the one question worth
 * asking about a font this appliance hands over: whether the device will draw the
 * interface's glyphs or boxes for them.
 *
 * It is an interface for the same reason [SharedDownloads] is: `Typeface` exists only on a
 * device, and the rules about what is reported to the reader are asserted on the JVM.
 */
fun interface FontProbe {
    /**
     * Reads [source] as a font and describes what the engine made of it, or returns null when
     * the engine would not load it at all. Consumes and closes [source].
     */
    fun inspect(source: InputStream): String?
}

/**
 * The glyphs to ask a font engine about.
 *
 * They are read from the interface's own glyph table (`tui/glyphs.go`) and are the ones a
 * reader actually sees: the menu bar's decoration, the node and information icons in the
 * message list, and the envelope that marks an unread conversation. The first is the
 * leftmost character of the menu bar, which is the empty box an unpatched terminal draws.
 * The last is a control — a font that does not have an ordinary letter in it is not a font,
 * whatever else it answers.
 *
 * They are written as code points and not as `\u` escapes because three of them are above the
 * basic plane, and a `\u` escape is exactly four hexadecimal digits: `"B"` is not the
 * menu bar's decoration but the unused `U+F043` followed by a letter `B`.
 */
val PROBED_GLYPHS: List<String> = listOf(
    glyph(0xF043B), // the menu bar's decoration
    glyph(0xF0002), // a node in the message list
    glyph(0xF064E), // the information icon
    glyph(0xF003), // the unread envelope, the one the interface selects off Darwin
    "A", // the control
)

/** The single character [codePoint] names, as a string a font engine can be asked about. */
private fun glyph(codePoint: Int): String = String(Character.toChars(codePoint))

/**
 * Loads a font through `Typeface` and reports how much of the interface's glyph set it covers.
 *
 * @param scratchDir a directory this application owns. The font is written there because the
 *   engine loads fonts from files, and a path in the application's own storage is readable by
 *   it on every Android version, whatever the rules for shared storage happen to be.
 */
class AndroidFontProbe(
    private val scratchDir: File,
    private val glyphs: List<String> = PROBED_GLYPHS,
) : FontProbe {

    override fun inspect(source: InputStream): String? {
        val file = File(scratchDir, TERMINAL_FONT_FILE_NAME)
        val report = runCatching {
            file.outputStream().use { out -> source.use { it.copyTo(out) } }
            val paint = Paint().apply { typeface = Typeface.createFromFile(file) }
            val missing = glyphs.filterNot { paint.hasGlyph(it) }
            if (missing.isEmpty()) {
                "$TERMINAL_FONT_FILE_NAME loads, and it covers all ${glyphs.size} glyphs the interface draws"
            } else {
                "$TERMINAL_FONT_FILE_NAME loads, but it is missing ${missing.size} of ${glyphs.size} " +
                    "glyphs the interface draws (${missing.joinToString(" ") { "U+%04X".format(it.codePointAt(0)) }})"
            }
        }.getOrNull()
        file.delete()
        return report
    }
}

/**
 * What the hand-off did, in the terms the appliance's screen has to show it.
 */
data class HandoffOutcome(
    /** Every file that other applications can now read, as a path. */
    val published: List<String>,

    /** Why the files that are not there are not there, one line each. */
    val failures: List<String>,

    /**
     * What Android's font engine made of the font, or null when there was no published font
     * to ask it about. Reported whether or not it is good news: a font the device will not
     * load is a terminal that keeps drawing boxes, and the reader is standing in front of it.
     */
    val engine: String?,

    /**
     * The lines to paste into Termux, which is the whole of what a person has to type: one
     * line that runs the setup script, or — on a build that carries no script — the font's
     * copy commands. Empty when nothing that a person could use was published.
     */
    val pasteLines: List<String>,

    /**
     * Where the setup script landed, or null when there is none.
     *
     * It is the path the appliance hands to Termux itself, and it is not recoverable from
     * [pasteLines] by searching for a file name: MediaStore stores a text file under the name
     * Android's type table gives `text/plain`, which appends an extension of its own. The path
     * is recorded where it is known rather than guessed at where it is used.
     */
    val setupScript: String?,

    /**
     * Whether the client itself is now in shared storage.
     *
     * It is the one file that matters: the script that installs everything else is useless
     * without it, so the screen says "tap Set up Termux" only when this is true.
     */
    val clientPublished: Boolean,
) {
    /** Whether Termux can be set up from what is now in shared storage. */
    val usable: Boolean get() = pasteLines.isNotEmpty()
}

/**
 * Publishes everything Termux needs, into shared storage.
 *
 * [open] is a function rather than an `AssetManager` so that every rule below can be asserted
 * on the JVM, where there is no APK to read — the same seam [RealBundledAssets] uses, for the
 * same reason.
 *
 * A file that is not in the APK is reported and does not stop the others. The two exceptions
 * are the client and the setup script, which are reported differently because without them the
 * hand-off has not happened at all: a build that carries neither has nothing to offer a person
 * beyond the font, and saying so is better than printing a command that will fail.
 */
class HandoffInstaller(
    private val open: (String) -> InputStream?,
    private val downloads: SharedDownloads,
    private val probe: FontProbe,
) {
    /** Publishes every file and reports what the screen should say. */
    fun install(): HandoffOutcome {
        val published = mutableListOf<String>()
        val failures = mutableListOf<String>()

        fun publish(fileName: String, mimeType: String, what: String): String? {
            val source = open(fileName)
            if (source == null) {
                failures += "the APK does not carry $fileName, so $what was not published"
                return null
            }
            val path = downloads.publish(fileName, mimeType, source)
            if (path == null) {
                failures += "$fileName could not be written to $SHARED_DOWNLOADS_DIR"
                return null
            }
            published += path
            return path
        }

        // The two files the hand-off is for, first, because they are the ones whose absence
        // changes what the screen has to say.
        val client = publish(CLIENT_ASSET_NAME, CLIENT_MIME_TYPE, "the client")
        val script = publish(SETUP_SCRIPT_ASSET_NAME, SETUP_SCRIPT_MIME_TYPE, "the setup script")
        publish(STANDALONE_LAUNCHER_ASSET_NAME, TERMUX_TEXT_MIME_TYPE, "the standalone launcher")
        publish(STACK_LAUNCHER_ASSET_NAME, TERMUX_TEXT_MIME_TYPE, "the attached launcher")
        publish(STACK_CONFIG_ASSET_NAME, TERMUX_TEXT_MIME_TYPE, "the attached configuration")
        val font = publish(TERMINAL_FONT_FILE_NAME, TERMINAL_FONT_MIME_TYPE, "the terminal font")
        publish(TERMINAL_FONT_LICENSE_FILE_NAME, TERMINAL_FONT_LICENSE_MIME_TYPE, "its license")
        publish(TMUX_CONFIG_FILE_NAME, TMUX_CONFIG_MIME_TYPE, "the tmux configuration")
        publish(TERMINAL_COLORS_FILE_NAME, TERMINAL_COLORS_MIME_TYPE, "the terminal colors")

        return HandoffOutcome(
            published = published.toList(),
            failures = failures.toList(),
            engine = font?.let { askTheFontEngine() },
            pasteLines = pasteLines(script, font),
            setupScript = script,
            clientPublished = client != null,
        )
    }

    /**
     * What a person has to paste, which is one line whenever the script is there.
     *
     * The script does the whole Termux side — the client, both launchers, the attached
     * configuration, the font and the two settings — so on a complete build there is exactly
     * one line to type. On a build that carries no script the font is still worth installing,
     * and its copy commands are what remains; a font that is not there either leaves nothing
     * worth printing.
     */
    private fun pasteLines(script: String?, font: String?): List<String> = when {
        script != null -> listOf(TermuxLauncher.setupLine(script))
        font != null -> fontInstallLines(font)
        else -> emptyList()
    }

    /**
     * Asks the device whether it will draw with this font at all.
     *
     * The font is read back from the APK rather than from where it was published, so that the
     * answer is about what this appliance carries and not about a copy that may since have been
     * moved, renamed, or deleted.
     */
    private fun askTheFontEngine(): String {
        val source = open(TERMINAL_FONT_FILE_NAME)
            ?: return "the font could not be read back to ask Android's font engine about it"
        return probe.inspect(source)
            ?: "Android's font engine would not load $TERMINAL_FONT_FILE_NAME: the terminal will keep drawing boxes"
    }
}

/**
 * The lines that put a published font in front of Termux's terminal.
 *
 * Used only when the setup script is not in the APK. The file is on the shared filesystem where
 * both applications can reach it, and only Termux can move it into its own home and adopt it.
 * `termux-reload-settings` ships with termux-tools, which is part of every Termux install.
 */
fun fontInstallLines(fontPath: String): List<String> = listOf(
    "mkdir -p ~/.termux",
    "cp $fontPath ~/.termux/font.ttf",
    "termux-reload-settings",
)

/**
 * The real publisher: MediaStore's Downloads collection, from Android 10 on.
 *
 * It is a thin adapter over [ContentResolver] and holds no decisions, because a
 * [ContentResolver] cannot exist on the JVM where the rules are asserted.
 *
 * @param packageName the application's own package, which is how a copy this appliance left
 *   behind is told apart from a file the operator put there by hand.
 */
@TargetApi(Build.VERSION_CODES.Q)
class MediaStoreDownloads(
    private val resolver: ContentResolver,
    private val packageName: String,
) : SharedDownloads {

    override fun publish(name: String, mimeType: String, source: InputStream): String? {
        forgetEarlierCopies(name)
        val uri = insert(name, mimeType) ?: return null
        return try {
            copy(source, uri)
            reveal(uri)
            "$SHARED_DOWNLOADS_DIR/${publishedName(uri, name)}"
        } catch (failed: IOException) {
            resolver.delete(uri, null, null)
            null
        }
    }

    /**
     * The name the file really has, which is the one thing this cannot predict.
     *
     * MediaStore normalises a file name to the type it was given, and Android's own type table
     * gives `font/otf` the extension `.ttf`, so a font offered as
     * `AtkynsonMonoNerdFontMono-Regular.otf` is stored as `AtkynsonMonoNerdFontMono-Regular.otf.ttf`.
     * A path this code assumed would name a file that is not there, and the copy command printed
     * for Termux would copy nothing. The name is read back from the row instead.
     */
    private fun publishedName(uri: Uri, asked: String): String = runCatching {
        resolver.query(uri, arrayOf(MediaStore.MediaColumns.DISPLAY_NAME), null, null, null)
            ?.use { cursor -> if (cursor.moveToFirst()) cursor.getString(0) else null }
    }.getOrNull() ?: asked

    /**
     * Removes the copies this appliance published earlier under the same name.
     *
     * MediaStore never overwrites an existing file: a second write of the same name becomes
     * "name (1).ttf", so an installation run twice would leave two fonts on the tablet and
     * report the wrong one. The comparison is on the name without its extension and not on the
     * whole of it, because the stored name is the normalised one and never equal to the name
     * this asked for — matching exactly would delete nothing at all, which is how the second
     * run became "name (1)" in the first place.
     *
     * Only this application's own contributions are removed.
     */
    private fun forgetEarlierCopies(name: String) {
        val selection = "${MediaStore.MediaColumns.OWNER_PACKAGE_NAME} = ? AND " +
            "(${MediaStore.MediaColumns.DISPLAY_NAME} = ? OR ${MediaStore.MediaColumns.DISPLAY_NAME} LIKE ?)"
        val stem = name.substringBeforeLast('.')
        runCatching {
            resolver.delete(
                MediaStore.Downloads.EXTERNAL_CONTENT_URI,
                selection,
                arrayOf(packageName, name, "$stem%"),
            )
        }
    }

    /**
     * Claims a row in the collection, marked pending so that no other application can read a
     * half-written font. A file that is invisible until it is whole is the same rule
     * [RealBundledAssets] follows with its staging file.
     */
    private fun insert(name: String, mimeType: String): Uri? {
        val values = ContentValues().apply {
            put(MediaStore.MediaColumns.DISPLAY_NAME, name)
            put(MediaStore.MediaColumns.MIME_TYPE, mimeType)
            put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
            put(MediaStore.MediaColumns.IS_PENDING, 1)
        }
        return runCatching { resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values) }.getOrNull()
    }

    /** Writes the whole file, or throws. */
    private fun copy(source: InputStream, uri: Uri) {
        val output = resolver.openOutputStream(uri) ?: throw IOException("no output stream for $uri")
        output.use { source.copyTo(it) }
    }

    /** Publishes the row, which is what makes the file readable by other applications. */
    private fun reveal(uri: Uri) {
        val values = ContentValues().apply { put(MediaStore.MediaColumns.IS_PENDING, 0) }
        resolver.update(uri, values, null, null)
    }
}
