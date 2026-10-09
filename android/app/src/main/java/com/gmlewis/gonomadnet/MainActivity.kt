// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.Manifest
import android.app.Activity
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.view.WindowManager
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView

/**
 * The appliance's one screen.
 *
 * There is no terminal emulator here and there is not meant to be: a tview/tcell user
 * interface needs a real PTY, and Termux already provides one. This screen does everything
 * around that terminal — it installs Termux, hands the client and its configuration over to
 * it, and starts and stops the two independent halves of the appliance, the sensors and the
 * daemon stack.
 *
 * The controls are in the order the work happens in: what a tablet needs before it can run
 * anything, then the two ways into the interface, then the appliance's own halves. Someone
 * setting a tablet up reads this list downwards and is done; the halves are two pairs and not
 * one switch because they are independent, and only this screen says which of the four
 * combinations is running.
 */
class MainActivity : Activity() {

    private lateinit var status: TextView
    private lateinit var hubField: EditText
    private lateinit var axisButton: Button
    private lateinit var termuxButton: Button

    /**
     * Where the setup script landed in this session, once it has been published.
     *
     * Held because three controls want it and because publishing is a 2.5 MB copy through a
     * content provider: "Set up Termux" and "Copy the setup line" both use what "Publish files
     * for Termux" or the first of them to be tapped put there, rather than copying the font a
     * second time. Written on the publishing thread and read on the UI thread.
     */
    @Volatile
    private var publishedScript: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        val layout = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        status = TextView(this)
        val controls = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }

        // The first thing a new tablet needs, and the one step the appliance can take most of
        // itself: the Play Store's Termux cannot run programs it installs, so the F-Droid build
        // is fetched, checked against its signing key and handed to Android's installer. The
        // label says what was found, because "Termux is installed" and "a Termux that works is
        // installed" are different states and only one of them is worth trusting.
        termuxButton = button("Install Termux") { checkOrInstallTermux() }
        controls.addView(termuxButton)

        // The client, the launchers and the terminal's font and colors, put where Termux can
        // read them. This is the one step that cannot happen inside Termux, because the files
        // come out of this APK.
        controls.addView(button("Publish files for Termux") { publishForTermux() })

        // And the step that turns those files into a working terminal: one script, run inside
        // Termux, which is the only process permitted to write into Termux's own home.
        controls.addView(button("Set up Termux") { setUpTermux() })

        // The same line, for the one case the button cannot cover: Termux only accepts commands
        // from other applications once a setting inside Termux's own private files says so, and
        // that setting is what the line turns on. So the line is the bootstrap, and the button
        // is everything after it.
        controls.addView(button("Copy the setup line") { copySetupLine() })

        controls.addView(button("Open gonomadnet (standalone)") { launch(TermuxLauncher.standaloneRequest()) })
        controls.addView(button("Open gonomadnet (attached)") { launch(TermuxLauncher.attachedRequest()) })

        controls.addView(button("Start sensors") { startSensors() })
        controls.addView(button("Stop sensors") { command(SensorService.ACTION_STOP_SENSORS) })
        controls.addView(button("Start stack") { command(SensorService.ACTION_START_STACK) })
        controls.addView(button("Stop stack") { command(SensorService.ACTION_STOP_STACK) })

        // The heading reference axis. The two choices degenerate in opposite mountings, so this
        // is not a preference: on a stand the screen-back normal is right and the screen-top
        // axis is a quarter turn wrong; lying flat it is the other way round. Changing it
        // restarts the sensor service so the new axis takes effect.
        axisButton = button(axisLabel()) { toggleAxis() }
        controls.addView(axisButton)

        // The one interface the appliance's transport needs. It is a setting of its own
        // rather than a copy of the client's configuration, because the appliance owns the
        // transport and the client owns none.
        hubField = EditText(this).apply {
            hint = "hub host:port"
            setText(ApplianceSettings(this@MainActivity).hubSpec)
        }
        controls.addView(hubField)
        controls.addView(button("Save hub address") { saveHub() })

        // Both halves of the screen scroll and share the height evenly. The controls have grown
        // past what a phone shows at once, and a readout pushed off the bottom of the screen is
        // worse than no readout at all: the appliance's failures are silent, and this is where
        // they are supposed to become visible.
        val controlScroll = ScrollView(this).apply { addView(controls) }
        val statusScroll = ScrollView(this).apply { addView(status) }
        layout.addView(controlScroll, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 3f))
        layout.addView(statusScroll, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 2f))
        setContentView(layout)

        report(prerequisiteChecklist())
        probeTermux()
        requestPermissionsIfNeeded()
    }

    private fun button(label: String, action: () -> Unit): Button =
        Button(this).apply {
            text = label
            setOnClickListener { action() }
        }

    // ---------------------------------------------------------------- Termux itself

    /** Asks what Termux is on this device, and labels the button with the answer. */
    private fun probeTermux() {
        Thread({
            val status = AndroidTermuxProbe.probe(this@MainActivity)
            runOnUiThread { termuxButton.text = termuxLabel(status) }
        }, "termux-probe").start()
    }

    /** The button's label, which is the state of the first step of the setup. */
    private fun termuxLabel(status: TermuxStatus): String = when (status.build) {
        TermuxBuild.ABSENT -> "Install Termux (from F-Droid)"
        TermuxBuild.MODERN -> "Replace the Play Store Termux"
        TermuxBuild.USABLE -> "Termux ${status.versionName ?: ""} is installed".trim()
    }

    /**
     * Reports what Termux is here, and installs the F-Droid build when there is none that works.
     *
     * The two checks before the download are the two ways this can be refused, and both are
     * settings only the person holding the tablet can change: this appliance has to be allowed
     * to install applications at all, and Termux has to be absent or the wrong build. Neither
     * refusal is an error — the download would simply fail at the last step with a message about
     * a permission — so both are stated and the screen that changes the first is opened.
     *
     * The download goes out over the network and is several megabytes, so it runs off the UI
     * thread, like every other network operation here.
     */
    private fun checkOrInstallTermux() {
        report("looking for Termux")
        Thread({
            val status = AndroidTermuxProbe.probe(this@MainActivity)
            report(TermuxReport.describe(status))
            runOnUiThread { termuxButton.text = termuxLabel(status) }
            if (status.usable) {
                return@Thread
            }
            if (!PackageInstallerHandoff.mayInstall(this@MainActivity)) {
                report(
                    "this appliance is not allowed to install applications yet, and Android has " +
                        "no way for it to turn that on itself. Android calls the setting " +
                        "'install unknown apps'; it is open now, and this appliance is the entry " +
                        "to allow.",
                )
                runOnUiThread { openUnknownSourcesSettings() }
                return@Thread
            }
            val provision = TermuxInstaller(
                fetch = UrlFetcher()::invoke,
                read = { apk -> AndroidArchiveIdentity.read(this@MainActivity, apk) },
                install = { apk -> PackageInstallerHandoff(this@MainActivity).install(apk) },
                scratch = cacheDir,
            ).provision()
            for (note in provision.notes) {
                report(note)
            }
            report(
                if (provision.installed) {
                    "confirm the install on Android's own screen; when that is done, tap " +
                        "'Publish files for Termux'"
                } else {
                    "Termux was not installed; the appliance cannot run a user interface without it"
                },
            )
        }, "termux-install").start()
    }

    /** Opens the one setting that decides whether this appliance may install anything. */
    private fun openUnknownSourcesSettings() {
        val intent = Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES)
            .setData(Uri.parse("package:$packageName"))
        runCatching { startActivity(intent) }
            .onFailure { report("the setting could not be opened: ${it.message}") }
    }

    // ---------------------------------------------------------------- the hand-off

    /** Publishes everything Termux needs, and says what to do with it. */
    private fun publishForTermux() = publish { }

    /**
     * Runs the setup script inside Termux, from the published copy.
     *
     * This is the whole of the Termux side of the installation: one command, run by Termux on
     * Termux's own files. What the appliance gets back is nothing — Termux is another
     * application — so the script writes what it did where both applications can read it, and
     * the readout names that file.
     */
    private fun setUpTermux() {
        withSetupScript { script ->
            val request = TermuxLauncher.setupRequest(script)
            TermuxLauncher.launch(this, request).fold(
                onSuccess = {
                    report("Termux is running the setup script in a new session")
                    report("it reports what it did into $SHARED_DOWNLOADS_DIR/$SETUP_STATUS_FILE_NAME")
                },
                onFailure = { report("Termux refused the launch: ${it.message}\n$PREREQUISITES") },
            )
        }
    }

    /**
     * Puts the one line a person has to type on the clipboard.
     *
     * It exists for the case the button cannot reach: Termux ignores commands from other
     * applications until a setting inside its own private files allows them, and that setting is
     * exactly what this line turns on. So on a tablet that has never been set up, this is the
     * step that has to happen by hand — once, in a terminal somebody opens themselves.
     */
    private fun copySetupLine() {
        withSetupScript { script ->
            val line = TermuxLauncher.setupLine(script)
            val clipboard = getSystemService(CLIPBOARD_SERVICE) as ClipboardManager
            clipboard.setPrimaryClip(ClipData.newPlainText("gonomadnet setup", line))
            report("copied to the clipboard: $line")
            report("paste it into Termux, in a session you opened yourself")
        }
    }

    /** Hands the published script's path to [action], publishing it first if need be. */
    private fun withSetupScript(action: (String) -> Unit) {
        publishedScript?.let {
            action(it)
            return
        }
        publish { outcome ->
            val script = outcome.setupScript
            if (script == null || !outcome.clientPublished) {
                report("nothing was published for Termux to install, so this step was not taken")
                return@publish
            }
            publishedScript = script
            action(script)
        }
    }

    /**
     * Publishes the client, the launchers and the terminal configuration into shared storage,
     * then calls [after] on the UI thread with what happened.
     *
     * The last step of the Termux side cannot be taken from here: Termux keeps its terminal font
     * and its properties in its own private data directory, and Android gives one application no
     * way to write into another's. What this can do is put the files where both applications can
     * read them — and the script that does the rest is one of them, so a complete build leaves
     * exactly one line to run.
     *
     * The tmux configuration is published for the person debugging with tmux and is deliberately
     * absent from what the readout says: the client is meant to be run directly in Termux, where
     * tmux's status bar would cost a line of an already small screen, and nothing here installs
     * or starts tmux. It is a configuration waiting for anyone who chooses to run it.
     *
     * The readout also says what Android's own font engine made of the font, because that engine
     * is the one Termux draws through and its answer is the difference between glyphs and boxes.
     *
     * The copy is megabytes through a content provider, so it runs off the UI thread for the same
     * reason the hub lookup does.
     */
    private fun publish(after: (HandoffOutcome) -> Unit) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
            report(
                "publishing to $SHARED_DOWNLOADS_DIR needs Android 10 or later; on this device " +
                    "the client has to be taken out of the APK by hand",
            )
            return
        }
        report("publishing the files Termux needs to $SHARED_DOWNLOADS_DIR")
        Thread({
            val outcome = HandoffInstaller(
                open = { name -> runCatching { assets.open(name) }.getOrNull() },
                downloads = MediaStoreDownloads(contentResolver, packageName),
                probe = AndroidFontProbe(cacheDir),
            ).install()
            for (path in outcome.published) {
                report("published $path")
            }
            for (failure in outcome.failures) {
                report(failure)
            }
            outcome.engine?.let { report(it) }
            publishedScript = outcome.setupScript
            if (outcome.usable) {
                report("in Termux, run:\n" + outcome.pasteLines.joinToString("\n") { "  $it" })
            }
            if (!outcome.clientPublished) {
                report("the client was not published, so Termux cannot be set up from here")
            }
            runOnUiThread { after(outcome) }
        }, "handoff").start()
    }

    // ---------------------------------------------------------------- the appliance

    private fun requestPermissionsIfNeeded() {
        if (checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(
                arrayOf(
                    Manifest.permission.ACCESS_FINE_LOCATION,
                    Manifest.permission.ACCESS_COARSE_LOCATION,
                    Manifest.permission.POST_NOTIFICATIONS,
                ),
                REQUEST_CODE,
            )
            return
        }
        startSensors()
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == REQUEST_CODE && grantResults.isNotEmpty() && grantResults.all { it == PackageManager.PERMISSION_GRANTED }) {
            startSensors()
        } else {
            report("location permission is refused; the appliance cannot know where it is")
        }
    }

    /** The current axis, phrased as what the reader gets. */
    private fun axisLabel(): String = when (ApplianceSettings(this).headingAxis) {
        HeadingAxis.SCREEN_BACK -> "Heading axis: screen back (on a stand)"
        HeadingAxis.SCREEN_TOP -> "Heading axis: screen top (lying flat)"
    }

    private fun toggleAxis() {
        val settings = ApplianceSettings(this)
        settings.headingAxis = when (settings.headingAxis) {
            HeadingAxis.SCREEN_BACK -> HeadingAxis.SCREEN_TOP
            HeadingAxis.SCREEN_TOP -> HeadingAxis.SCREEN_BACK
        }
        axisButton.text = axisLabel()
        report(axisButton.text.toString())
        // The axis is read when the sensor service starts, so it has to be restarted for the
        // change to take effect.
        SensorService.command(this, SensorService.ACTION_RESTART_SENSORS)
    }

    private fun saveHub() {
        val spec = hubField.text.toString().trim()
        val problem = ApplianceConfig.describeFailure(spec)
        if (problem != null) {
            report(problem)
            return
        }
        ApplianceSettings(this).hubSpec = spec
        report("saved the hub as \"$spec\"")
        // Resolution and the reachability probe are network operations. On Android they
        // throw NetworkOnMainThreadException from the UI thread, which shows up as every
        // name failing to resolve at once, so they are run off it.
        Thread({
            val parsed = Resolver.parseHostPort(spec) ?: return@Thread
            val resolved = Resolver.resolveDialable(parsed.first, parsed.second)
            report(ApplianceConfig.describeChoice(spec, resolved, parsed.second))
            Resolver.lastFailure?.let { report("the lookup failed: $it") }
        }, "hub-resolve").start()
    }

    private fun startSensors() {
        if (checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            report("location permission is refused; the appliance cannot know where it is")
            return
        }
        SensorService.start(this)
        report("the sensor service is starting")
    }

    private fun command(action: String) {
        SensorService.command(this, action)
        report("sent $action")
    }

    private fun launch(request: RunCommandRequest) {
        val outcome = TermuxLauncher.launch(this, request)
        outcome.fold(
            onSuccess = { report("asked Termux to run ${request.commandPath}") },
            onFailure = { report("Termux refused the launch: ${it.message}\n$PREREQUISITES") },
        )
    }

    /**
     * The three permissions this appliance needs and the two settings it cannot set itself.
     *
     * Every one of them is silent when it is missing: a refused RUN_COMMAND has no symptom on
     * this screen at all, and a Termux that is the wrong build fails at the first attempt to run
     * anything with a message about a permission nobody can act on. So the state of each is
     * printed where somebody debugging an appliance can read it without a manual.
     */
    private fun prerequisiteChecklist(): String =
        "Permissions:\n" +
            "  RUN_COMMAND:  " + if (checkSelfPermission(RunCommandRequest.PERMISSION) == PackageManager.PERMISSION_GRANTED) "granted" else "NOT granted" +
            "\n  location:     " + if (checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED) "granted" else "NOT granted" +
            "\n  install apps: " + if (PackageInstallerHandoff.mayInstall(this)) "allowed" else "NOT allowed" +
            "\n\nSetting a new tablet up: \"Install Termux\" -> \"Publish files for Termux\" ->\n" +
            "\"Set up Termux\" -> \"Open gonomadnet\". Only the second half of that has to be\ndone by hand, " +
            "and only once: Termux ignores commands from other applications until\n" +
            "allow-external-apps = true is in its own ~/.termux/termux.properties, and this\n" +
            "appliance cannot write there. \"Copy the setup line\" gives you the one line that\n" +
            "does it, for a terminal you open yourself.\n\n"

    /**
     * Writes to both of the appliance's diagnostic channels: the screen, for the person
     * holding the tablet, and logcat, for anyone diagnosing it from a host. A launch failure
     * here is silent by nature — that is the whole reason the prerequisites are printed — so
     * it has to be visible in both places.
     */
    private fun report(line: String) {
        android.util.Log.i(TAG, line)
        runOnUiThread { status.append(line + "\n") }
    }

    companion object {
        private const val TAG = "gonomadnet"

        private const val REQUEST_CODE = 1
        private const val PREREQUISITES =
            "Termux has to be the F-Droid build, and it has to have allow-external-apps = true " +
                "in its ~/.termux/termux.properties, and this appliance has to hold " +
                "com.termux.permission.RUN_COMMAND (granted in Settings). 'Copy the setup line' " +
                "does the first two."
    }
}
