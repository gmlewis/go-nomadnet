// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import android.util.Log
import android.view.Surface
import java.io.File

/**
 * The sensor service: the appliance's one job that is not optional.
 *
 * It holds the tablet's GNSS receiver and magnetometer open, turns their readings into
 * the JSON sample stream `gonsensor` understands, and publishes the resulting NMEA both
 * to the pipes the bot reads and to the loopback socket that the Nomad Network client in
 * Termux subscribes to.
 *
 * It is deliberately independent of the stack supervisor. It runs with the daemons or
 * without them, which is what gives the standalone client a real position on an appliance
 * that is running no local bot at all — and it is also the cheapest configuration for the
 * battery, since a receiver and a magnetometer cost far less than three more Go processes.
 *
 * It is a foreground service holding a partial wake lock because every sensor it uses is a
 * non-wakeup sensor: Android stops delivering from them the moment the process stops
 * holding one, and a position that stops updating when the screen goes off is not a
 * position.
 */
class SensorService : Service() {

    private lateinit var paths: StackPaths
    private lateinit var wakeLock: PowerManager.WakeLock
    private var pipeline: SensorPipeline? = null
    private var converter: GonsensorFactory? = null
    private var supervisor: StackSupervisor? = null
    private var monitor: Thread? = null
    private var stopping = false

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        paths = StackPaths(filesDir.absolutePath)
        File(paths.logDir).mkdirs()
        File(paths.runDir).mkdirs()
        wakeLock = (getSystemService(Context.POWER_SERVICE) as PowerManager)
            .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, WAKE_LOCK_TAG)
        startForeground(NOTIFICATION_ID, buildNotification("starting"))
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP_SENSORS -> {
                stopStack()
                stopSensors()
                stopSelf()
                return START_NOT_STICKY
            }
            // Starting the stack resolves a name, probes a port, creates pipes, spawns three
            // processes and waits up to fifteen seconds for the hub to publish its
            // destination. None of that may happen on the main thread: the network parts
            // throw NetworkOnMainThreadException, and the wait would be an ANR.
            ACTION_RESTART_SENSORS -> {
                stopSensors()
                startSensors()
            }
            ACTION_START_STACK -> Thread({ startStack() }, "stack-start").start()
            ACTION_STOP_STACK -> Thread({ stopStack() }, "stack-stop").start()
        }
        startSensors()
        return START_STICKY
    }

    override fun onDestroy() {
        stopStack()
        stopSensors()
        super.onDestroy()
    }

    // ---- the sensor half, which never depends on the stack --------------------

    private fun startSensors() {
        if (pipeline != null) {
            return
        }
        // A restart of the sensor half goes through stopSensors first, and the poll below
        // has to be told to run again: the flag is a statement about the service, not about
        // one run of it, and a monitor that exits at once leaves the daemons unpolled for the
        // rest of the service's life.
        stopping = false
        // The converters write the bot's two pipes, so this half makes and holds them: a
        // converter's output is redirected to its path the moment it is spawned, and a path
        // that is not a pipe yet becomes a plain file that the pipe made afterwards replaces.
        val converters = GonsensorFactory(
            runner = RealProcessRunner(),
            binary = LaunchSpecs.binary(applicationInfo.nativeLibraryDir, "gonsensor"),
            paths = paths,
            pipes = RealSensorFifos(paths),
            log = ::report,
        )

        // The location source needs the heading source's position, and the heading source
        // needs the location source's, so both are wired after construction.
        lateinit var location: LocationSource
        lateinit var heading: HeadingSource
        location = LocationSource(this, ::publish, ::report)
        heading = HeadingSource(this, ::publish, ::report).also { source ->
            // Which axis the heading is measured along is the reader's decision, because the
            // right answer depends on how they are holding the tablet.
            source.axis = ApplianceSettings(this).headingAxis
            report("heading reference axis: ${source.axis}")
        }
        heading.setDisplayRotation(::displayRotation)
        heading.setPositionSource {
            location.lastKnown()?.let { Triple(it.latitude, it.longitude, it.altitude) }
        }

        val running = SensorPipeline(
            positionSink = converters.positionOnly(),
            headingSink = converters.headingOnly(),
            broadcastSink = converters.piped(),
            feed = SensorFeedServer(::report),
            location = location,
            heading = heading,
            wakeLock = PowerWakeLock { wakeLock },
            log = ::report,
        )
        running.start()
        pipeline = running
        converter = converters

        // A daemon that dies is restarted, and one that dies in a loop is eventually given
        // up on rather than restarted forever. The check is a poll rather than a thread per
        // daemon so that nothing here can outlive the service.
        monitor = Thread({
            while (!stopping) {
                try {
                    Thread.sleep(MONITOR_INTERVAL_MS)
                } catch (interrupted: InterruptedException) {
                    Thread.currentThread().interrupt()
                    return@Thread
                }
                supervisor?.poll(System.currentTimeMillis())
            }
        }, "stack-monitor").apply {
            isDaemon = true
            start()
        }
        updateNotification("live position and heading")
    }

    private fun stopSensors() {
        stopping = true
        monitor = null
        pipeline?.stop()
        pipeline = null
        // After the converters are stopped, so nothing is spawned against a pipe that is
        // about to be released from underneath it.
        converter?.close()
        converter = null
        if (wakeLock.isHeld) {
            runCatching { wakeLock.release() }
        }
    }

    /** Sends one already-serialized sample to every converter and to the loopback feed. */
    private fun publish(line: String) {
        pipeline?.offer(line)
    }

    // ---- the optional half ----------------------------------------------------

    private fun startStack() {
        if (supervisor != null) {
            return
        }
        // The transport owns every interface on this tablet, so its one interface is built
        // here, from the hub the operator named, resolved to a literal in Java because the
        // bundled Go binary cannot resolve a name on Android at all.
        val hubSpec = ApplianceSettings(this).hubSpec
        ApplianceConfig.describeFailure(hubSpec)?.let { report(it) }
        // The RPC key is stated rather than derived, because the transport and its clients run
        // from different configuration directories and would derive different keys from the
        // identities in them — which leaves every client's RPC refused as "unauthorized".
        val config = ApplianceConfig.nodeConfig(hubSpec, rpcKey = ApplianceSettings(this).rpcKey)
        if (config.interfaces.isEmpty()) {
            report("the transport has no interface, so the appliance is isolated from the network")
            report("hub address: \"$hubSpec\"")
            Resolver.lastFailure?.let { report("the lookup failed: $it") }
        } else {
            val iface = config.interfaces.first()
            report(ApplianceConfig.describeChoice(hubSpec, DialableAddress(iface.host, Resolver.isIPv6Literal(iface.host), true), iface.port))
        }
        supervisor = StackSupervisor(
            nativeDir = applicationInfo.nativeLibraryDir,
            paths = paths,
            config = config,
            runner = RealProcessRunner(),
            fifos = RealSensorFifos(paths),
            // Assets are read through the service's own resources, which is the only handle
            // to the APK there is; a name that is not in it yields null rather than throwing.
            bundled = RealBundledAssets { name -> runCatching { assets.open(name) }.getOrNull() },
            readHubDestination = {
                File(paths.hubDestinationFile).takeIf { it.isFile }?.readText()?.trim()
            },
            writeConfig = { path, contents ->
                File(path).also { it.parentFile?.mkdirs() }.writeText(contents)
            },
            log = ::report,
        ).also { it.start() }
        updateNotification("sensors and stack running")
    }

    private fun stopStack() {
        supervisor?.stop()
        supervisor = null
    }

    @Suppress("DEPRECATION")
    private fun displayRotation(): Int = when (
        (getSystemService(Context.WINDOW_SERVICE) as android.view.WindowManager).defaultDisplay.rotation
    ) {
        Surface.ROTATION_90 -> 1
        Surface.ROTATION_180 -> 2
        Surface.ROTATION_270 -> 3
        else -> 0
    }

    private fun report(line: String) {
        Log.i(TAG, line)
    }

    // ---- notification ---------------------------------------------------------

    private fun buildNotification(detail: String): Notification {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            manager.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "sensors", NotificationManager.IMPORTANCE_LOW),
            )
        }
        val stop = PendingIntent.getService(
            this,
            0,
            Intent(this, SensorService::class.java).setAction(ACTION_STOP_SENSORS),
            PendingIntent.FLAG_IMMUTABLE,
        )
        @Suppress("DEPRECATION")
        val builder = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            Notification.Builder(this, CHANNEL_ID)
        } else {
            Notification.Builder(this)
        }
        return builder
            .setContentTitle("gonomadnet sensors")
            .setContentText(detail)
            .setSmallIcon(android.R.drawable.ic_menu_mylocation)
            .setOngoing(true)
            .addAction(Notification.Action.Builder(null, "Stop", stop).build())
            .build()
    }

    private fun updateNotification(detail: String) {
        (getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager)
            .notify(NOTIFICATION_ID, buildNotification(detail))
    }

    companion object {
        private const val TAG = "gonomadnet-sensors"
        private const val CHANNEL_ID = "gonomadnet-sensors"
        private const val NOTIFICATION_ID = 1
        private const val WAKE_LOCK_TAG = "gonomadnet:sensors"

        /** How often the daemons are checked on, in milliseconds. */
        private const val MONITOR_INTERVAL_MS = 2_000L

        const val ACTION_START_STACK = "com.gmlewis.gonomadnet.START_STACK"
        const val ACTION_STOP_STACK = "com.gmlewis.gonomadnet.STOP_STACK"
        const val ACTION_STOP_SENSORS = "com.gmlewis.gonomadnet.STOP_SENSORS"

        /** Restarts the sensor half, which is how a changed heading axis takes effect. */
        const val ACTION_RESTART_SENSORS = "com.gmlewis.gonomadnet.RESTART_SENSORS"

        /** Starts the sensor service. */
        fun start(context: Context) {
            context.startForegroundService(Intent(context, SensorService::class.java))
        }

        /** Asks the running service to start or stop the daemon stack. */
        fun command(context: Context, action: String) {
            context.startService(Intent(context, SensorService::class.java).setAction(action))
        }
    }
}
