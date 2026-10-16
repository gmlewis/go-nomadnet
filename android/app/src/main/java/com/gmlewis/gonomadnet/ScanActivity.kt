// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.hardware.Camera
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.Gravity
import android.view.Surface
import android.view.SurfaceHolder
import android.view.SurfaceView
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

/**
 * The screen the camera is read on.
 *
 * A dialog in the client would rather read an address from a QR code than have it typed, and
 * the client has no camera, so it asks the appliance — see [ScanServer]. This is what the
 * appliance answers with: a full-screen preview, and the first QR code that decodes is
 * handed back as this activity's result.
 *
 * It is a plain [Activity] on the platform's own theme, and it is built out of platform views
 * only: the appliance carries no AndroidX, and a scanner is not a reason to start. The camera
 * itself is the platform's deprecated [Camera] interface, which is still the only camera API
 * that needs nothing else installed — CameraX is an AndroidX library, and the modern camera2
 * interface is considerably more code for a preview that is read and thrown away.
 *
 * Everything that can be decided without hardware is decided elsewhere and tested there: the
 * wire protocol is [ScanProtocol], the decode is [ScanDecoder], and the preview size is
 * [choosePreviewSize]. What is left here is the camera's lifecycle, which is the part that
 * can only be checked on a device.
 */
@Suppress("DEPRECATION")
class ScanActivity : Activity(), SurfaceHolder.Callback {

    /** The camera, while this screen holds it. Null when the preview is not running. */
    private var camera: Camera? = null

    /** The surface the preview is drawn on. */
    private var preview: SurfaceView? = null

    /** The row that says what to do, replaced by the reason when there is nothing to do. */
    private var hint: TextView? = null

    /** The preview size actually in use, cached because the callback runs per frame. */
    private var frameWidth = 0
    private var frameHeight = 0

    /** The handler the focus ticker runs on, made only by a camera that needs one. */
    private var focusHandler: Handler? = null

    /** The tick that asks a camera without continuous focus to focus again, while it is up. */
    private var focusTick: Runnable? = null

    /** decoding is set while a frame is being read, so frames are dropped rather than queued. */
    private val decoding = AtomicBoolean(false)

    /** The one thread frames are decoded on, so the preview is never blocked by a decode. */
    private val decoder = Executors.newSingleThreadExecutor()

    /** finished is set once a result has been handed back, so a late frame cannot repeat it. */
    private val finished = AtomicBoolean(false)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // The preview is the whole screen, and a screen that dims or sleeps while the reader
        // is aiming it is a screen that has stopped working.
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        setContentView(buildUi())

        if (cameraPermissionGranted()) {
            // The surface is not ready yet; surfaceCreated starts the camera.
            return
        }
        requestPermissions(arrayOf(Manifest.permission.CAMERA), REQUEST_CAMERA)
    }

    /** buildUi lays out the preview, the hint and the way out of this screen. */
    private fun buildUi(): FrameLayout {
        val root = FrameLayout(this)

        val surface = SurfaceView(this)
        surface.holder.addCallback(this)
        preview = surface
        root.addView(
            surface,
            FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT,
            ),
        )

        val message = TextView(this).apply {
            text = getString(R.string.scan_hint)
            setTextColor(Color.WHITE)
            setBackgroundColor(Color.argb(160, 0, 0, 0))
            gravity = Gravity.CENTER
            setPadding(24, 24, 24, 24)
        }
        hint = message

        val cancel = Button(this).apply {
            text = getString(R.string.scan_cancel)
            setOnClickListener { finishWith(null) }
        }

        val bottom = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            addView(
                message,
                LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    ViewGroup.LayoutParams.WRAP_CONTENT,
                ),
            )
            addView(
                cancel,
                LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    ViewGroup.LayoutParams.WRAP_CONTENT,
                ),
            )
        }
        root.addView(
            bottom,
            FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
                Gravity.BOTTOM,
            ),
        )
        return root
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_CAMERA) {
            return
        }
        if (cameraPermissionGranted()) {
            // The surface was created before the answer arrived, so the preview is started
            // from here rather than from surfaceCreated, which has already run.
            startPreview()
            return
        }
        // A refused permission has no other symptom anywhere: the reader pressed a button,
        // a system dialog appeared, and then nothing happened. So the reason is said here,
        // on the screen they are already looking at, and the screen stays until they dismiss
        // it — including for the "don't ask again" answer, which no later attempt can repair.
        hint?.text = getString(R.string.scan_permission_refused)
        preview?.visibility = SurfaceView.INVISIBLE
    }

    /** surfaceCreated starts the preview once there is a surface to draw it on. */
    override fun surfaceCreated(holder: SurfaceHolder) {
        if (cameraPermissionGranted()) {
            startPreview()
        }
    }

    /**
     * surfaceChanged starts the preview once the surface has a size.
     *
     * It is the second chance at what [surfaceCreated] attempted, and it is what makes that
     * attempt's guard safe: a surface that is created but not yet valid is refused there and
     * accepted here, so a preview is never lost to the order the callbacks happen to arrive in.
     */
    override fun surfaceChanged(holder: SurfaceHolder, format: Int, width: Int, height: Int) {
        if (cameraPermissionGranted()) {
            startPreview()
        }
    }

    /** surfaceDestroyed stops the preview: the surface it was drawn on is gone. */
    override fun surfaceDestroyed(holder: SurfaceHolder) {
        stopPreview()
    }

    /** onPause stops the preview, because another application may want the camera. */
    override fun onPause() {
        stopPreview()
        super.onPause()
    }

    /** onDestroy stops the preview and the decode thread. */
    override fun onDestroy() {
        stopPreview()
        decoder.shutdown()
        super.onDestroy()
    }

    /** cameraPermissionGranted reports whether this app may use the camera. */
    private fun cameraPermissionGranted(): Boolean =
        checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED

    /**
     * startPreview opens the camera and begins feeding frames to the decoder.
     *
     * It is safe to call more than once: a camera is opened only when one is not already
     * running, so the surface callback and the permission answer cannot between them open two.
     */
    private fun startPreview() {
        if (camera != null || isFinishing) {
            return
        }
        // The permission answer and the surface arriving are two events in an order that is
        // not fixed, and a preview cannot be attached to a surface that does not exist yet.
        // Rather than treating that as a failure, the attempt is left to whichever of the two
        // arrives second — which is the one that can see both.
        if (preview?.holder?.surface?.isValid != true) {
            return
        }
        val cameraId = backCameraId()
        val opened = try {
            if (cameraId >= 0) Camera.open(cameraId) else Camera.open()
        } catch (e: Exception) {
            // Camera.open throws when the camera is already in use or the device has none.
            // A scan that cannot happen is not a scan that may leave the reader with a black
            // screen and no explanation.
            hint?.text = getString(R.string.scan_unavailable)
            Log.i(TAG, "could not open the camera: ${e.message}")
            return
        }

        try {
            val parameters = opened.parameters
            val chosen = choosePreviewSize(
                parameters.supportedPreviewSizes.map { PreviewSize(it.width, it.height) },
            )
            if (chosen != null) {
                parameters.setPreviewSize(chosen.width, chosen.height)
                frameWidth = chosen.width
                frameHeight = chosen.height
            } else {
                frameWidth = parameters.previewSize.width
                frameHeight = parameters.previewSize.height
            }
            if (frameWidth <= 0 || frameHeight <= 0) {
                // A camera that reports no preview size at all cannot be given a preview: the
                // frame buffer to read into would be empty, and asking for one is a crash
                // rather than a scan that does not work.
                hint?.text = getString(R.string.scan_unavailable)
                opened.release()
                return
            }
            // The focus mode is named before the parameters are handed over, because the
            // platform's own default does not focus: a camera left in FOCUS_MODE_AUTO never
            // moves its lens until something asks it to, and a code held at arm's length is
            // then photographed from whatever distance the lens happened to be parked at.
            val focusMode = chooseFocusMode(parameters.supportedFocusModes)
            if (focusMode != null) {
                parameters.focusMode = focusMode
            }
            opened.parameters = parameters
            opened.setDisplayOrientation(displayOrientation(cameraId))
            opened.setPreviewDisplay(preview?.holder)
            armPreview(opened)
            opened.startPreview()
            camera = opened
            if (focusMode == Camera.Parameters.FOCUS_MODE_AUTO) {
                // The one mode that has to be asked, so it is asked for as long as this screen
                // is up — see startFocusTicker.
                startFocusTicker()
            }
        } catch (e: Exception) {
            // Nothing about a preview that failed to start is recoverable here, and leaving
            // the camera open would keep it from the next attempt.
            Log.i(TAG, "could not start the preview: ${e.message}")
            hint?.text = getString(R.string.scan_unavailable)
            opened.release()
            camera = null
        }
    }

    /**
     * armPreview installs the frame callback and the buffer it is handed frames in.
     *
     * The buffered form is what keeps this from allocating a frame's worth of memory per
     * frame — on a 640x480 preview that is 300 KB several times a second, which is enough
     * garbage to bring a tablet to a stutter. The callback re-arms the buffer it was given
     * before doing anything else, because a callback that does not is a preview that stops
     * after one frame.
     */
    private fun armPreview(opened: Camera) {
        val bytes = frameWidth * frameHeight * 3 / 2 // the Y plane and the half-resolution chroma
        opened.setPreviewCallbackWithBuffer { data, source ->
            source.addCallbackBuffer(data)
            onFrame(data)
        }
        opened.addCallbackBuffer(ByteArray(bytes))
        opened.addCallbackBuffer(ByteArray(bytes))
    }

    /**
     * startFocusTicker keeps asking a camera to focus for as long as the preview is up.
     *
     * It is for the cameras that offer no continuous mode at all — see [chooseFocusMode]. On
     * those, FOCUS_MODE_AUTO is inert: the lens sits where it is until `autoFocus()` is called,
     * one call focuses once, and a reader who moves the phone has moved out of focus again. So
     * the call is repeated, at the pace a person moves rather than the pace the sensor could
     * answer, until the preview stops.
     *
     * The ticks are posted to the main looper because that is the thread the camera was opened
     * on, and the camera's own callbacks arrive there.
     */
    private fun startFocusTicker() {
        val handler = focusHandler ?: Handler(Looper.getMainLooper()).also { focusHandler = it }
        val tick = object : Runnable {
            override fun run() {
                val running = camera ?: return
                try {
                    running.autoFocus(null)
                } catch (e: Exception) {
                    // autoFocus throws when the camera has gone away or is already focusing.
                    // Both are answered by the next tick, and neither is worth a word on a
                    // screen the reader is aiming.
                }
                handler.postDelayed(this, FOCUS_INTERVAL_MS)
            }
        }
        focusTick = tick
        handler.postDelayed(tick, FOCUS_INTERVAL_MS)
    }

    /**
     * onFrame hands one preview frame to the decoder.
     *
     * The frame is copied before it is handed over, because the buffer it arrived in is the
     * camera's and is already being filled again. A frame that arrives while another is being
     * read is dropped rather than queued: a reader aiming a camera produces frames far faster
     * than a decoder reads them, and a queue would only decode older and older pictures of
     * where the camera used to be pointing.
     */
    private fun onFrame(data: ByteArray?) {
        if (data == null || frameWidth <= 0 || frameHeight <= 0) {
            return
        }
        if (!decoding.compareAndSet(false, true)) {
            return
        }
        val luminance = data.copyOf(frameWidth * frameHeight)
        val width = frameWidth
        val height = frameHeight
        decoder.execute {
            val payload = ScanDecoder.decode(luminance, width, height)
            decoding.set(false)
            if (payload != null && !finished.get()) {
                runOnUiThread { finishWith(payload) }
            }
        }
    }

    /** stopPreview releases the camera and its callback, and stops asking it to focus. */
    private fun stopPreview() {
        // The ticker goes first: one that outlived the camera would be asking a released one,
        // and one that outlived this screen would hold it alive behind the one that replaced it.
        focusTick?.let { tick -> focusHandler?.removeCallbacks(tick) }
        focusTick = null
        val running = camera ?: return
        camera = null
        try {
            running.setPreviewCallbackWithBuffer(null)
            running.stopPreview()
        } catch (e: Exception) {
            // A preview that has already stopped is the state this was trying to reach.
        }
        running.release()
    }

    /**
     * finishWith hands the reader's answer back and closes this screen.
     *
     * A null payload is a reader who backed out, which the client is told apart from a
     * payload it could not use — the first costs them nothing, and the second leaves what
     * they had typed alone.
     */
    private fun finishWith(payload: String?) {
        if (!finished.compareAndSet(false, true)) {
            return
        }
        if (payload == null) {
            setResult(RESULT_CANCELED)
        } else {
            setResult(RESULT_OK, Intent().putExtra(EXTRA_PAYLOAD, payload))
        }
        finish()
    }

    /** backCameraId returns the id of a rear-facing camera, or -1 when there is none. */
    private fun backCameraId(): Int {
        val count = Camera.getNumberOfCameras()
        for (id in 0 until count) {
            val info = Camera.CameraInfo()
            Camera.getCameraInfo(id, info)
            if (info.facing == Camera.CameraInfo.CAMERA_FACING_BACK) {
                return id
            }
        }
        // A tablet with only a front camera still reads a code held up to it, so the first
        // camera is better than a refusal.
        return if (count > 0) 0 else -1
    }

    /**
     * displayOrientation returns the rotation that makes the preview upright.
     *
     * The sensor is mounted the same way round in every device and the screen is not, so the
     * preview has to be turned by the difference. The decoder does not care — a QR code is
     * read from its corner patterns, whichever way up the image is — but the reader does: a
     * preview that is on its side is a preview they cannot aim.
     */
    private fun displayOrientation(cameraId: Int): Int {
        val info = Camera.CameraInfo()
        Camera.getCameraInfo(cameraId, info)
        val rotation = when (windowManager.defaultDisplay.rotation) {
            Surface.ROTATION_90 -> 90
            Surface.ROTATION_180 -> 180
            Surface.ROTATION_270 -> 270
            else -> 0
        }
        return if (info.facing == Camera.CameraInfo.CAMERA_FACING_FRONT) {
            (360 - (info.orientation + rotation) % 360) % 360
        } else {
            (info.orientation - rotation + 360) % 360
        }
    }

    companion object {
        /** The log tag this screen reports under. */
        private const val TAG = "gonomadnet"

        /** The request code for the camera permission. */
        private const val REQUEST_CAMERA = 1

        /** EXTRA_PAYLOAD is the QR code's text, as the camera read it. */
        const val EXTRA_PAYLOAD: String = "com.gmlewis.gonomadnet.SCAN_PAYLOAD"
    }
}
