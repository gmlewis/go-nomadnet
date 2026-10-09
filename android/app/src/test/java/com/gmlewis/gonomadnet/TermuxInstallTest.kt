// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import com.sun.net.httpserver.HttpServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.net.InetSocketAddress
import java.nio.file.Files

/**
 * Installing Termux, which is the step everything else waits on.
 *
 * Two builds of Termux look identical on a home screen and only one of them can run a program
 * it installs, so the appliance does not describe the difference to a person — it downloads the
 * F-Droid build and checks the signing key itself. That check is the security-relevant half of
 * this file, and it is asserted as data: an APK's identity is a record here, not a package
 * manager call, so the refusals can be asserted without a device, a download or an APK.
 *
 * The download is exercised for real against a server in this process, because the parts of it
 * that fail are the parts nobody writes a test for otherwise: a 404 body written to disk as
 * though it were an APK, and a response with no declared length that runs until the tablet is
 * full.
 */
class TermuxInstallTest {

    private val scratch = mutableListOf<File>()
    private var server: HttpServer? = null

    @After
    fun cleanUp() {
        server?.stop(0)
        server = null
        scratch.forEach { it.deleteRecursively() }
        scratch.clear()
    }

    /**
     * A temporary directory.
     *
     * `/tmp` is named explicitly rather than left to `java.io.tmpdir`, because on macOS that is
     * a path under `/var/folders` long enough to break anything that opens a Unix socket in it.
     */
    private fun tempDir(prefix: String): File {
        val base = if (System.getProperty("os.name").orEmpty().startsWith("Mac")) File("/tmp") else File(checkNotNull(System.getProperty("java.io.tmpdir")))
        return Files.createTempDirectory(base.toPath(), prefix).toFile().also { scratch += it }
    }

    /** A provisioner whose three steps are recorded rather than performed. */
    private class Recorder(
        val downloads: Boolean = true,
        val archive: ArchiveIdentity? = fdroidIdentity(),
        val install: Result<Unit> = Result.success(Unit),
    ) {
        val calls = mutableListOf<String>()
        val installed = mutableListOf<File>()

        fun installer(scratch: File) = TermuxInstaller(
            fetch = { url, destination ->
                calls += "fetch $url"
                if (downloads) destination.writeText("the downloaded package")
                downloads
            },
            read = { apk ->
                calls += "read ${apk.name}"
                archive
            },
            install = { apk ->
                calls += "install ${apk.name}"
                installed += apk
                install
            },
            scratch = scratch,
        )
    }

    // ------------------------------------------------------------ the pinned build

    @Test
    fun theBuildIsPinnedToTheOneTheDocumentationRecords() {
        // The fingerprint is a claim about somebody else's key, and it is the only thing
        // standing between a tablet and whatever a repository chose to serve. It is asserted
        // here against the number written in the installation guide, so the two cannot drift:
        // a pin that was updated in one place and not the other is a build that refuses the
        // right Termux, or accepts the wrong one.
        val guide = File("../../docs/Android.md").readText()

        assertTrue("the guide does not record the pinned version", guide.contains(TermuxPackage.FDROID_VERSION_NAME))
        assertTrue("the guide does not record the pinned version code", guide.contains("${TermuxPackage.FDROID_VERSION_CODE}"))
        assertTrue("the guide does not record the pinned signing key", guide.contains(TermuxPackage.FDROID_SIGNER_SHA256))
    }

    @Test
    fun theDownloadComesFromFdroidsOwnRepositoryAtThePinnedVersion() {
        assertEquals("https://f-droid.org/repo/com.termux_1002.apk", TermuxPackage.FDROID_APK_URL)
        assertTrue(TermuxPackage.FDROID_APK_URL.startsWith("https://"))
        assertTrue(TermuxPackage.FDROID_APK_URL.contains(TermuxPackage.FDROID_HOST))
        assertTrue(
            "the URL must name the version code the pin is about",
            TermuxPackage.FDROID_APK_URL.contains("${TermuxPackage.FDROID_VERSION_CODE}"),
        )
    }

    @Test
    fun theTargetSdkBoundaryIsTheOneAndroidEnforces() {
        // 29 is the first version that forbids an application from executing a file it owns,
        // and Termux keeps its whole prefix in its own data directory. Getting this number
        // wrong in either direction is a tablet that either refuses a working Termux or accepts
        // one that cannot run anything.
        assertEquals(29, TermuxPackage.UNUSABLE_TARGET_SDK)
        assertEquals(TermuxBuild.USABLE, TermuxReport.probe(TermuxPackage.PACKAGE, "0.118.3", 1002, 28).build)
        assertEquals(TermuxBuild.MODERN, TermuxReport.probe(TermuxPackage.PACKAGE, "0.119.0", 1003, 29).build)
        assertEquals(TermuxBuild.MODERN, TermuxReport.probe(TermuxPackage.PACKAGE, "0.119.0", 1003, 34).build)
        assertEquals(TermuxBuild.ABSENT, TermuxReport.probe(TermuxPackage.PACKAGE, null, 0, 0).build)
    }

    @Test
    fun theRefusalOfAModernBuildSaysWhichBuildItIs() {
        // The failure this replaces is a message about a permission that names nothing a person
        // can act on. So the refusal has to name the build, and has to say what to do about it.
        val refusal = TermuxReport.describe(TermuxReport.probe(TermuxPackage.PACKAGE, "0.119.0", 1003, 34))

        assertTrue("the targetSdk is what makes it the wrong build", refusal.contains("34"))
        assertTrue(refusal.contains("0.119.0"))
        assertTrue("a refusal that does not say what to do is not a report", refusal.contains("uninstall"))
    }

    @Test
    fun aBuildThatWorksIsReportedAsSuch() {
        val report = TermuxReport.describe(TermuxReport.probe(TermuxPackage.PACKAGE, "0.118.3", 1002, 28))

        assertTrue(report.contains("0.118.3"))
        assertTrue(report.contains("28"))
        assertFalse("a working Termux is not a request to do anything", report.contains("uninstall"))
    }

    // ------------------------------------------------------------ what gets installed

    @Test
    fun theFdroidBuildIsAcceptedAndItsReasonSaysSo() {
        val verdict = TermuxArchive.judge(identity())

        assertTrue(verdict.installable)
        assertTrue(verdict.reason.contains("0.118.3"))
        assertTrue(verdict.reason.contains("F-Droid"))
    }

    @Test
    fun aFingerprintInAnySpellingIsRecognised() {
        // Android reports a certificate fingerprint in whatever form the caller asked for, and
        // the pinned constant is written in the form a person can compare by eye. A comparison
        // that depended on the spelling would refuse the right build on some devices.
        val uppercase = TermuxPackage.FDROID_SIGNER_SHA256.uppercase()
        val withColons = TermuxPackage.FDROID_SIGNER_SHA256.chunked(2).joinToString(":").uppercase()

        assertTrue(TermuxArchive.judge(identity(signers = listOf(uppercase))).installable)
        assertTrue(TermuxArchive.judge(identity(signers = listOf(withColons))).installable)
        assertEquals(TermuxPackage.FDROID_SIGNER_SHA256, TermuxPackage.normaliseFingerprint(withColons))
    }

    @Test
    fun anotherSigningKeyIsRefusedAndTheKeyIsNamed() {
        // The whole point of the pin: a Termux-shaped APK signed by somebody else is not
        // installed, and what was offered instead is written out so the refusal can be checked.
        val verdict = TermuxArchive.judge(identity(signers = listOf("aabbccdd")))

        assertFalse(verdict.installable)
        assertTrue("the offending key must be reported", verdict.reason.contains("aabbccdd"))
        assertTrue(verdict.reason.contains("not installed"))
    }

    @Test
    fun oneGoodSignerAmongSeveralIsEnough() {
        // An APK signed by more than one key is one any of them may have signed, and refusing
        // it because the first fingerprint in the list was not F-Droid's would refuse F-Droid's.
        val verdict = TermuxArchive.judge(
            identity(signers = listOf("00112233", TermuxPackage.FDROID_SIGNER_SHA256)),
        )

        assertTrue(verdict.installable)
    }

    @Test
    fun somethingThatIsNotTermuxIsRefusedWhateverItIsSignedWith() {
        // F-Droid signs other applications with the same key, so the key alone is not the test.
        val verdict = TermuxArchive.judge(identity(packageName = "org.fdroid.fdroid"))

        assertFalse(verdict.installable)
        assertTrue(verdict.reason.contains("org.fdroid.fdroid"))
    }

    @Test
    fun anUnsignedOrUnreadableDownloadIsRefused() {
        assertFalse(TermuxArchive.judge(identity(signers = emptyList())).installable)
        assertFalse(TermuxArchive.judge(null).installable)
        assertFalse(TermuxArchive.judge(identity(packageName = null)).installable)
    }

    // ------------------------------------------------------------ the provision

    @Test
    fun aDownloadIsCheckedBeforeAnythingIsOfferedToTheInstaller() {
        val recorder = Recorder()
        val scratch = tempDir("termux-")

        val outcome = recorder.installer(scratch).provision()

        assertTrue(outcome.installed)
        assertEquals(
            listOf("fetch ${TermuxPackage.FDROID_APK_URL}", "read termux.apk", "install termux.apk"),
            recorder.calls,
        )
        assertEquals(1, recorder.installed.size)
        assertTrue("the installer is handed the downloaded file", recorder.installed[0].isFile)
    }

    @Test
    fun aDownloadThatFailedIsReportedAndNothingElseIsTried() {
        val recorder = Recorder(downloads = false)
        val scratch = tempDir("termux-")

        val outcome = recorder.installer(scratch).provision()

        assertFalse(outcome.installed)
        assertEquals(listOf("fetch ${TermuxPackage.FDROID_APK_URL}"), recorder.calls)
        assertTrue("the report has to say where Termux can be got instead", outcome.notes.any { it.contains(TermuxPackage.FDROID_HOST) })
        assertFalse("nothing may be left behind for someone to install by hand", File(scratch, "termux.apk").exists())
    }

    @Test
    fun aDownloadSignedBySomebodyElseIsNeverOfferedToTheInstaller() {
        // The one case where a half-finished install would be worse than none: the file is a
        // real APK, it installed cleanly, and it is not Termux.
        val recorder = Recorder(archive = identity(signers = listOf("aabbccdd")))
        val scratch = tempDir("termux-")

        val outcome = recorder.installer(scratch).provision()

        assertFalse(outcome.installed)
        assertTrue("the download was not read at all", recorder.calls.contains("read termux.apk"))
        assertTrue("the installer must never see it", recorder.calls.none { it.startsWith("install") })
        assertFalse("a refused package is not left on the device", File(scratch, "termux.apk").exists())
    }

    @Test
    fun aRefusedInstallIsReportedWithTheSystemsOwnWords() {
        val recorder = Recorder(install = Result.failure(IllegalStateException("no session for you")))
        val scratch = tempDir("termux-")

        val outcome = recorder.installer(scratch).provision()

        assertFalse(outcome.installed)
        assertTrue(outcome.notes.any { it.contains("no session for you") })
    }

    // ------------------------------------------------------------ the download itself

    /** Serves [body] at `/apk` on a socket in this process, and returns the port. */
    private fun serve(status: Int, body: ByteArray): Int {
        val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
        server.createContext("/apk") { exchange ->
            exchange.sendResponseHeaders(status, body.size.toLong())
            exchange.responseBody.use { it.write(body) }
        }
        server.start()
        this.server = server
        return server.address.port
    }

    private fun bytes(count: Int): ByteArray = ByteArray(count) { (it % 251).toByte() }

    @Test
    fun aDownloadedFileArrivesWhole() {
        // The bytes are checked and not just the file's existence: a download that stopped short
        // and left a partial file is an APK the installer would reject with a message about a
        // corrupt package.
        val body = bytes(300_000)
        val port = serve(200, body)
        val destination = File(tempDir("download-"), "termux.apk")

        val fetched = UrlFetcher()("http://127.0.0.1:$port/apk", destination)

        assertTrue(fetched)
        assertTrue(destination.readBytes().contentEquals(body))
        assertFalse("the staging file must not be left beside it", File(destination.absolutePath + ".part").exists())
    }

    @Test
    fun aResponseThatIsNotAnApkIsNotWrittenToDisk() {
        // A captive portal and a 404 page are both a body of the right shape and the wrong
        // contents, and the file that would be handed to the installer is the error page.
        val port = serve(404, "not here".toByteArray())
        val destination = File(tempDir("download-"), "termux.apk")

        val fetched = UrlFetcher()("http://127.0.0.1:$port/apk", destination)

        assertFalse(fetched)
        assertFalse(destination.exists())
    }

    @Test
    fun aResponseTooLargeToBeTermuxStopsBeingWritten() {
        // The URL is a name in a document and not a promise. Without the cap, a repository that
        // answered with something enormous would fill the device's cache before anything looked
        // at it, on a device that has little to spare.
        val port = serve(200, bytes(200_000))
        val destination = File(tempDir("download-"), "termux.apk")

        val fetched = UrlFetcher(limit = 1024)("http://127.0.0.1:$port/apk", destination)

        assertFalse(fetched)
        assertFalse(destination.exists())
    }

    @Test
    fun aDownloadFromNowhereIsReportedRatherThanThrown() {
        // Reported, because this is a button on a screen and not a fatal error: a tablet with no
        // network is a tablet whose owner taps the button again.
        val destination = File(tempDir("download-"), "termux.apk")

        val fetched = UrlFetcher(connectTimeoutMs = 250, readTimeoutMs = 250)("http://127.0.0.1:1/apk", destination)

        assertFalse(fetched)
        assertFalse(destination.exists())
    }
}

/**
 * An APK's identity as the platform would report it, which is the record the decision is made
 * from. The default is the build this appliance pins, so a test that does not care about the
 * identity gets the accepted one and has to say so explicitly to get anything else.
 */
private fun identity(
    packageName: String? = TermuxPackage.PACKAGE,
    versionName: String? = TermuxPackage.FDROID_VERSION_NAME,
    signers: List<String> = listOf(TermuxPackage.FDROID_SIGNER_SHA256),
): ArchiveIdentity = ArchiveIdentity(packageName, versionName, TermuxPackage.FDROID_VERSION_CODE.toLong(), signers)

/** The pinned F-Droid build's identity. */
private fun fdroidIdentity(): ArchiveIdentity = identity()
