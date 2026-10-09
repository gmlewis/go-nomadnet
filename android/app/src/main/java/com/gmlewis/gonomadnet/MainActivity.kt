// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.Manifest
import android.app.Activity
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.View
import android.view.WindowInsets
import android.view.WindowManager
import android.widget.Button
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import java.io.File
import java.net.InetSocketAddress
import java.net.Socket

/**
 * The client the appliance runs, and everything it needs to be started.
 *
 * The client is this APK's own binary, running in this APK's own storage: [binary] is the
 * absolute path inside `nativeLibraryDir`, [argv] is what it is told, and [home] is where it
 * runs. All three are plain data, so what is started can be asserted without a device.
 */
data class BuiltInClient(val binary: String, val argv: List<String>, val home: String)

/**
 * The appliance's two screens.
 *
 * The first is the whole appliance: it hands out the one permission it needs, shows what the
 * two halves are doing, and opens the client. The second is the client itself, drawn a cell
 * at a time by [TerminalView] on the screen [TerminalScreen] holds — see the console page in
 * [showConsole] — with the keys a tablet has no way to press along its bottom edge.
 *
 * The client is run out of the APK itself: it carries the client and the console host that
 * gives it a pseudo-terminal, so "Open gonomadnet" is one tap from this screen to a working
 * interface with nothing else installed. The pieces of that are [BuiltInClient],
 * [MainActivity.Companion.builtInClient] and [MainActivity.Companion.consoleHostSpec], which
 * build the command lines as plain data so that what is started can be asserted without a
 * device.
 *
 * The controls are in the order the work happens in: what the tablet is asked for, then the
 * way into the interface, then the appliance's own halves. Someone setting a tablet up reads
 * this list downwards and is done; the halves are two pairs and not one switch because they
 * are independent, and only this screen says which of the four combinations is running.
 */
class MainActivity : Activity() {

    private lateinit var status: TextView
    private lateinit var hubField: EditText
    private lateinit var axisButton: Button

    /**
     * The controls page, kept so that closing the console comes back to it.
     *
     * The terminal is a second page rather than a second activity, because the console's
     * whole lifetime is a socket, a child process and a thread: an activity that can be
     * destroyed and recreated by a rotation would have to rebuild all three, and a terminal
     * that lost its session to a rotation is a terminal nobody can use.
     */
    private var controlsPage: View? = null

    /** The running console, if the terminal page is open. Written and read on the UI thread. */
    private var console: ConsoleSession? = null

    /** The thread reading the console socket, which ends when the client does. */
    private var consolePump: Thread? = null

    /** The grid the console was last told it has, which is what the client is sized for. */
    private var consoleGrid = TerminalGrid(cols = DEFAULT_CONSOLE_COLS, rows = DEFAULT_CONSOLE_ROWS)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        val layout = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        status = TextView(this)
        val controls = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }

        // The appliance's way into the interface, and the only one: the APK carries the client
        // and the console host that gives it a terminal, so this needs nothing else installed.
        controls.addView(button("Open gonomadnet") { openBuiltInClient() })

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
        controlsPage = layout
        applyEdgeToEdgeInsets(layout)
        // The hub field is not a thing to type into the moment the screen appears, and a field
        // that holds the focus asks its scroll view to bring it into view: the list then opens
        // scrolled, with the first button — "Open gonomadnet", the one thing this screen is for
        // — cut off at the top. The focus is given up and the list put back at its top once the
        // page has been laid out, which is the first moment either is possible.
        layout.post {
            hubField.clearFocus()
            controlScroll.scrollTo(0, 0)
        }

        report(prerequisiteChecklist())
        requestPermissionsIfNeeded()
    }

    private fun button(label: String, action: () -> Unit): Button =
        Button(this).apply {
            text = label
            setOnClickListener { action() }
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

    // ---------------------------------------------------------- the built-in client

    /**
     * Opens the appliance's own console, starting the transport first.
     *
     * The client this opens is an *attached* client: it requires the shared instance the
     * appliance's transport owns, and it refuses to become a transport itself. Starting the
     * stack is therefore part of opening the console, and so is waiting for it: the client
     * looks for the instance for about two hundred milliseconds and then exits, so a console
     * opened in the same tap that started the transport would show a client that died during
     * startup and no reason for it.
     *
     * Starting the stack is asked of the supervisor through the service, which is the same
     * path the Start stack button takes and is idempotent: an appliance that is already
     * running is not restarted, and the wait costs one connection.
     */
    private fun openBuiltInClient() {
        if (console != null) {
            report("the console is already open")
            return
        }
        report("asking the appliance's transport to start")
        SensorService.command(this, SensorService.ACTION_START_STACK)
        Thread({
            val up = awaitTransport(
                port = NodeConfigSpec.DEFAULT_SHARED_INSTANCE_PORT,
                attempts = TRANSPORT_WAIT_ATTEMPTS,
                waitMs = TRANSPORT_WAIT_MS,
                probe = ::transportIsUp,
                sleep = { Thread.sleep(it) },
            )
            if (!up) {
                report(
                    "the transport has not answered on " +
                        "${NodeConfigSpec.DEFAULT_SHARED_INSTANCE_PORT} after " +
                        "${TRANSPORT_WAIT_ATTEMPTS * TRANSPORT_WAIT_MS / 1000} seconds, so the client " +
                        "would be started with no shared instance to attach to. Check the hub " +
                        "address and the log.",
                )
                return@Thread
            }
            runOnUiThread { showConsole() }
        }, "console-transport").start()
    }

    /**
     * whether the transport's shared instance is accepting connections.
     *
     * The transport publishes its shared instance on the loopback interface, and the client's
     * own attach is a connection to it, so a connection is exactly the readiness the client
     * needs — and the only signal available without reading another process's files. It runs
     * on a thread of its own, never on the UI thread, where Android forbids it.
     */
    private fun transportIsUp(port: Int): Boolean = runCatching {
        Socket().use { socket ->
            socket.connect(InetSocketAddress(LOOPBACK, port), TRANSPORT_CONNECT_TIMEOUT_MS)
        }
    }.isSuccess

    /**
     * Builds the console page and starts the session.
     *
     * The pieces are each one thing: [TerminalScreen] holds the emulated screen,
     * [TerminalParser] turns the client's bytes into it, [ConsoleSession] owns the socket and
     * the host process, and [TerminalView] draws it and reports keys back. The session is
     * given the view's notifications rather than the other way round, so the class that owns
     * the socket still knows nothing about any `View` — which is what makes it testable.
     */
    private fun showConsole() {
        if (console != null) {
            return
        }
        val paths = StackPaths(filesDir.absolutePath)
        // The client's home is the console host's working directory, and a working directory
        // that does not exist is a host that does not start — with an error that names the
        // host rather than the directory.
        paths.ensureDirectories()
        // The settings that make this a node and give it an editor, written before the client
        // can own the file: a client whose configuration says `enable_node = no` serves no
        // pages, and one whose editor is `nano` has no editor on this device at all.
        seedClientConfig(paths)
        // The channels the appliance comes with, written before the client can own the file.
        // An install that has already run the client has a store of its own, and it is left
        // alone: it is the operator's by then, and it holds whatever they have added.
        seedDefaultHubs(paths)
        val client = builtInClient(applicationInfo.nativeLibraryDir, paths)
        val screen = TerminalScreen(consoleGrid.cols, consoleGrid.rows)
        val view = TerminalView(this).apply { this.screen = screen }
        val session = ConsoleSession(
            // The host needs the size the client should be started with, and the socket name
            // it is to dial, which does not exist until the session binds it.
            host = { name ->
                consoleHostSpec(
                    nativeLibraryDir = applicationInfo.nativeLibraryDir,
                    paths = paths,
                    client = client,
                    socketName = name,
                    grid = consoleGrid,
                )
            },
            listeners = LocalConsole,
            runner = RealProcessRunner(),
            uid = android.os.Process.myUid(),
            pid = android.os.Process.myPid(),
            parser = TerminalParser(screen, onOutput = { view.outputParsed() }),
            onRefused = { peer ->
                report("an application with uid $peer connected to the console socket, which is not this appliance")
            },
            onEnded = { end -> onConsoleEnded(end) },
        )
        view.onKey = { key -> session.sendKey(key) }
        // A tap on the console is a mouse event to the client, which is what its lists and
        // buttons are driven by. The view spells it — the encoding is the terminal's, and
        // the view is what knows whether the client asked to be told about the mouse — and
        // the session carries the bytes, as it carries the bytes of a key.
        view.onMouse = { bytes -> session.sendBytes(bytes) }
        // A rotation, or the keyboard, changes the grid the client is laid out for. The
        // screen is resized with it, so the emulator and the terminal it is emulating agree.
        view.onResize = { grid ->
            consoleGrid = grid
            screen.resize(grid.cols, grid.rows)
            session.resize(grid.cols, grid.rows)
        }
        console = session

        // The session is started before the page is shown, because a session that cannot
        // start has already reported why through [onConsoleEnded] — and a page switched to
        // first would be a black screen with the reason for it behind the page.
        session.start()
        if (session.socketName == null) {
            console = null
            return
        }

        val page = FrameLayout(this)
        page.addView(view, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        // The console is the whole of the screen it is on, and there is no title bar to take
        // out of it: the theme carries no action bar, which is what keeps the client's first
        // rows on the screen. A bar that exists and is hidden at runtime is a bar the window
        // has already measured itself around: its content view is slid up by the bar's
        // height and never grown to take it, so the top rows of the client end up above the
        // screen and a band the bar's own height ends up empty at the bottom of it.
        setContentView(page)
        // The key strip is the soft keyboard's companion: the window insets are what know
        // whether the keyboard is up, so the console is told from here, and the client is
        // resized as the strip appears and goes — those rows are its rows.
        applyEdgeToEdgeInsets(page) { up -> view.keysVisible = up }

        consolePump = Thread({ session.pump() }, "console-pump").also { it.start() }
        report("the console is open; the client is starting on ${consoleGrid.cols}x${consoleGrid.rows}")
    }

    /**
     * Writes the settings the appliance states in the client's configuration.
     *
     * Two of them are about being an appliance rather than a desktop — a node that serves
     * pages and has a name, and an editor that is on the device — and the client's own
     * default says neither. They are written here, before the client's first run, into the
     * file the client reads; see [ClientConfig] for what is written and what is left alone.
     *
     * The file is the Go client's default taken from the APK's assets rather than a
     * configuration invented here, so an operator who opens the editor sees the documented
     * file with three values changed. An asset that is not in the APK is reported rather
     * than silently skipped: the symptom is a console that answers "This instance is not
     * hosting a node", and the reason has to be on the screen that opened it.
     */
    private fun seedClientConfig(paths: StackPaths) {
        val template = runCatching {
            assets.open(ClientConfig.TEMPLATE_ASSET_NAME).use { it.readBytes().decodeToString() }
        }.getOrNull()
        if (template == null) {
            report(
                "the APK carries no ${ClientConfig.TEMPLATE_ASSET_NAME}, so the client keeps the " +
                    "configuration it writes for itself: it will not host a node and its editor " +
                    "will not start",
            )
            return
        }
        val editor = LaunchSpecs.binary(applicationInfo.nativeLibraryDir, ClientConfig.EDITOR_NAME)
        val seeded = runCatching { ClientConfig.ensure(paths.nomadnetworkConfig, template, editor) }
            .getOrElse { failure ->
                report("could not write the client's settings: ${failure.message}")
                return
            }
        if (seeded) {
            report("wrote the client's settings to ${paths.nomadnetworkConfig}")
        }
    }

    /**
     * Writes the channels the appliance comes with, if it has never run its client.
     *
     * The client keeps its channels in a store of its own and has no notion of a default, so
     * an appliance that seeded none arrived with an empty Channels page. The store's own
     * existence is the record that this has been done: once the client has run, the file is
     * the operator's, and an appliance that rewrote it on every start would undo every channel
     * they had added or removed.
     *
     * The local hub is listed only when it has published its destination, which is derived
     * from an identity generated on this device and therefore different on every install. An
     * appliance whose stack has not been started yet has no local hub to list, and it is
     * listed the next time the console is opened.
     */
    private fun seedDefaultHubs(paths: StackPaths) {
        val localHub = File(paths.hubDestinationFile).takeIf { it.isFile }?.readText()?.trim()
        val seeded = runCatching { DefaultHubs.seed(File(paths.clientHubStore), localHub) }
            .getOrElse { failure ->
                report("could not write the appliance's default channels: ${failure.message}")
                return
            }
        if (seeded) {
            report("wrote ${DefaultHubs.hubs(localHub).size} default channels to ${paths.clientHubStore}")
        }
    }

    /**
     * Reports how the session ended, and closes the console page.
     *
     * This arrives on the thread that read the socket, which is usually the pump, so
     * everything it touches is either thread-safe — the status readout posts to the UI thread
     * itself — or posted. The client exiting is the ordinary end of a session: ctrl-q in the
     * interface quits the client, and the console closes behind it.
     */
    private fun onConsoleEnded(end: ConsoleEnd) {
        report(
            when (end) {
                is ConsoleEnd.Exited -> "the client exited with code ${end.code}"
                ConsoleEnd.Closed -> "the console socket closed"
                is ConsoleEnd.Failed -> "the console failed: ${end.reason}"
                ConsoleEnd.Stopped -> "the console was closed"
            },
        )
        runOnUiThread { closeConsole() }
    }

    /**
     * Ends the session, if there is one, and goes back to the controls.
     *
     * Ending it kills the host, and the host ends the client with it: closing the console
     * must not leave a client running with nobody reading its terminal, which would be a
     * process holding a socket nothing is draining and an identity still announcing.
     */
    private fun closeConsole() {
        val session = console ?: return
        console = null
        consoleGrid = TerminalGrid(cols = DEFAULT_CONSOLE_COLS, rows = DEFAULT_CONSOLE_ROWS)
        session.stop()
        consolePump = null
        // The controls come back into the same window, which the console left edge to edge:
        // the page is re-attached by being laid out again, so it is told about the bars
        // rather than being left to draw under them.
        controlsPage?.let {
            setContentView(it)
            applyEdgeToEdgeInsets(it)
        }
    }

    /**
     * Keeps a page inside the system bars, and reports whether the keyboard is up.
     *
     * On Android 15 the appliance is drawn edge to edge, which for a terminal is what is
     * wanted — every pixel of a tablet is a pixel of someone's interface — but a grid drawn
     * under the status bar is a row of the client's window that cannot be read or tapped.
     * The bars are therefore taken out of whichever page is showing rather than being drawn
     * over it. Both pages take them: the console is the one that needs the whole screen, and
     * the controls follow it into the same edge-to-edge window when the console closes.
     *
     * The bottom is the larger of the navigation bar and the keyboard: with the keyboard up
     * the grid must end above it, or the client's last rows — and the keys drawn under them —
     * are behind it. [onKeyboardUp] is told which of the two it is, because the console's
     * key strip is the keyboard's companion and comes and goes with it.
     */
    @Suppress("DEPRECATION")
    private fun applyEdgeToEdgeInsets(page: View, onKeyboardUp: ((Boolean) -> Unit)? = null) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.VANILLA_ICE_CREAM) {
            // Below Android 15 the window already fits the system bars, so taking them out
            // again would be padding the page twice.
            return
        }
        window.setDecorFitsSystemWindows(false)
        page.setOnApplyWindowInsetsListener { view, insets ->
            val bars = insets.getInsets(WindowInsets.Type.systemBars())
            val keyboard = insets.getInsets(WindowInsets.Type.ime()).bottom
            view.setPadding(bars.left, bars.top, bars.right, maxOf(bars.bottom, keyboard))
            onKeyboardUp?.invoke(keyboard > 0)
            insets
        }
        page.requestApplyInsets()
    }

    /** The Android back gesture leaves the console rather than the appliance. */
    @Suppress("DEPRECATION", "OVERRIDE_DEPRECATION")
    override fun onBackPressed() {
        if (console != null) {
            closeConsole()
            return
        }
        super.onBackPressed()
    }

    override fun onDestroy() {
        // The pump thread is blocked on a socket read that only closing the connection ends,
        // so ending the session here is what stops it: the thread then sees the connection
        // closed and returns.
        console?.stop()
        super.onDestroy()
    }

    /**
     * The one permission this appliance is granted, and what the controls below do.
     *
     * The appliance holds one permission and it is silent when it is missing: a refused
     * location has no symptom on this screen at all, so its state is printed where somebody
     * debugging an appliance can read it without a manual. The rest of the readout says which
     * button starts which half, because there are four combinations of running and stopped and
     * only this screen says which one the tablet is in.
     */
    private fun prerequisiteChecklist(): String =
        "Location: " +
            if (checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED) {
                "granted"
            } else {
                "NOT granted"
            } +
            "\n\nThe appliance runs both halves out of this app: \"Start stack\" runs the\n" +
            "transport, and \"Open gonomadnet\" runs the client on it. Nothing else has to be\n" +
            "installed, and nothing here installs anything.\n\n"

    /**
     * Writes to both of the appliance's diagnostic channels: the screen, for the person
     * holding the tablet, and logcat, for anyone diagnosing it from a host. The appliance's
     * failures are silent by nature, so they have to be visible in both places.
     */
    private fun report(line: String) {
        android.util.Log.i(TAG, line)
        runOnUiThread { status.append(line + "\n") }
    }

    companion object {
        private const val TAG = "gonomadnet"

        private const val REQUEST_CODE = 1

        /** The client's name inside `nativeLibraryDir`, where a `.so` suffix is required. */
        const val CLIENT_NAME = "gonomadnetclient"

        /** The console host's name inside `nativeLibraryDir`, which gives the client a PTY. */
        const val CONSOLE_HOST_NAME = "gorcons"

        /**
         * The grid the console starts at, before anything has measured the screen.
         *
         * The client is told its size on the command line, so a session started before the
         * view has been laid out has to name one. Eighty by twenty-four is the terminal every
         * program already behaves well on, and the first layout replaces it with what the
         * screen actually fits — which is a resize the client is told about, not a restart.
         */
        private const val DEFAULT_CONSOLE_COLS = 80
        private const val DEFAULT_CONSOLE_ROWS = 24

        /**
         * How long to wait for the transport's shared instance, as attempts and a gap.
         *
         * Ten seconds is a generous bound for a daemon that is already running and a fair one
         * for one that is being started on a tablet that has just woken up. The wait is not
         * the client's timeout — the client's own attach gives up after two hundred
         * milliseconds — it is the wait that keeps the client from being started at all until
         * there is something for it to attach to.
         */
        private const val TRANSPORT_WAIT_ATTEMPTS = 40
        private const val TRANSPORT_WAIT_MS = 250L

        /**
         * How long a readiness probe may take.
         *
         * The probe connects to the loopback interface, where an open port answers at once
         * and a closed one is refused at once, so this only bounds a pathological case — a
         * transport that has accepted its socket and then stopped answering.
         */
        private const val TRANSPORT_CONNECT_TIMEOUT_MS = 500

        /** The interface the transport publishes its shared instance on. */
        private const val LOOPBACK = "127.0.0.1"

        /**
         * builtInClient is the client as the appliance runs it, out of the APK.
         *
         * The two directories it names are the ones that make it the *attached* client: the
         * Reticulum configuration that requires the shared instance the appliance's own
         * transport owns, rather than one that lists interfaces and would become a second
         * transport. Its Nomad Network configuration is its own, under the appliance's
         * private storage: the messages and the identity the client keeps there are the
         * appliance's, and a second client announcing the same identity is a node that
         * appears twice.
         */
        fun builtInClient(nativeLibraryDir: String, paths: StackPaths): BuiltInClient = BuiltInClient(
            binary = LaunchSpecs.binary(nativeLibraryDir, CLIENT_NAME),
            argv = listOf(
                "-t",
                // The transport's own directory and the client's differ by one word, and by
                // everything that matters: one lists interfaces and owns the shared instance,
                // the other requires it. This one is the client's.
                "--rnsconfig", paths.rnsClientConfigDir,
                "--config", paths.nomadnetworkConfigDir,
            ),
            home = paths.clientHome,
        )

        /**
         * consoleHostSpec is the console host, the program that gives the client a terminal.
         *
         * Android starts an application's processes with no controlling terminal and no way
         * to allocate one, so a tview client started directly has nothing to draw on. The
         * host opens a pseudo-terminal, runs the client on it, and carries the terminal's
         * bytes back to the socket the session bound — see `android/console`.
         *
         * The client's own arguments go through `--arg`, one at a time: the host takes its
         * own flags and nothing else, and an argument left bare is a host that refuses to
         * start. `HOME` here is the host's working directory rather than the child's
         * environment, because the host builds the child's environment from nothing by
         * itself — nothing set here can leak into the client.
         *
         * The client is told one wheel notch moves one row, because the only wheel it will
         * ever see is a finger: a drag arrives as one notch per row it has travelled, and at
         * the client's own default of several rows a notch the page runs away from the
         * finger that is dragging it. A whole-notches-per-row multiplier is not a thing the
         * wire can say, so the client is told what one notch means instead.
         */
        fun consoleHostSpec(
            nativeLibraryDir: String,
            paths: StackPaths,
            client: BuiltInClient,
            socketName: String,
            grid: TerminalGrid,
        ): LaunchSpec = LaunchSpec(
            name = CONSOLE_HOST_NAME,
            binary = LaunchSpecs.binary(nativeLibraryDir, CONSOLE_HOST_NAME),
            argv = listOf(
                "--socket", socketName,
                "--command", client.binary,
                "--home", client.home,
            ) + client.argv.flatMap { listOf("--arg", it) } + listOf(
                "--env", WHEEL_LINES_ENV,
                "--cols", grid.cols.toString(),
                "--rows", grid.rows.toString(),
            ),
            env = mapOf("HOME" to client.home),
            logFile = "${paths.logDir}/$CONSOLE_HOST_NAME.log",
        )

        /**
         * WHEEL_LINES_ENV is how the console tells the client what one wheel notch means.
         *
         * A finger drag is carried to the client as wheel notches, one per row of the
         * screen it travelled, so what the page does is what the client does with a notch.
         * The client's default moves several rows per notch, which makes a drag scroll
         * several times further than the finger went.
         */
        const val WHEEL_LINES_ENV = "GONOMADNET_WHEEL_LINES=1"

        /**
         * awaitTransport waits for the appliance's transport, and reports whether it arrived.
         *
         * The built-in client is an *attached* client: it requires the shared instance the
         * transport owns and refuses to become a transport itself, and it looks for one for
         * about a fifth of a second before giving up. "Open gonomadnet (built in)" starts the
         * stack and opens the console as one tap, and a console opened before the transport
         * has bound its instance is a console showing a client that died during startup.
         *
         * So the wait happens before the console exists, it is bounded, and it gives up with a
         * report rather than hanging. A transport that is already up costs one probe and no
         * sleep, which is the usual case: Start stack was tapped earlier, or the appliance is
         * running and the client is being reopened.
         *
         * The probe and the sleep are parameters because one is a socket connection and the
         * other is time, and neither belongs in a unit test.
         */
        fun awaitTransport(
            port: Int,
            attempts: Int,
            waitMs: Long,
            probe: (Int) -> Boolean,
            sleep: (Long) -> Unit,
        ): Boolean {
            repeat(attempts) { attempt ->
                if (probe(port)) {
                    return true
                }
                // Having used the last attempt there is nothing left to wait for: sleeping
                // after the final probe would only postpone the report.
                if (attempt < attempts - 1) {
                    sleep(waitMs)
                }
            }
            return false
        }
    }
}
