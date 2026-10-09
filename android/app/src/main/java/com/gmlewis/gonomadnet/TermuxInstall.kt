// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.content.Context
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.os.Build
import java.io.File
import java.io.IOException
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.URL
import java.util.Locale

/**
 * The Termux build this appliance can work with.
 *
 * There is more than one application on the stores called Termux, and only one of them can run
 * programs it installs: Android 10 forbids an application whose `targetSdkVersion` is 29 or
 * higher from calling `exec()` on a file inside its own data directory, and Termux keeps its
 * whole `$PREFIX` there. The Play Store build has a modern `targetSdk` and therefore cannot
 * run the client at all, which is why the appliance installs Termux from F-Droid itself rather
 * than sending a person to look for it.
 *
 * The build is identified by its signing key rather than by its name. The name on the icon is
 * not the build, and a name is exactly what a hostile copy would get right; the certificate is
 * not, and the key below is the one F-Droid's repository is signed with.
 */
object TermuxPackage {

    /** The package name every build of Termux uses. */
    const val PACKAGE = "com.termux"

    /**
     * The `targetSdkVersion` from which an application can no longer `exec()` its own files.
     *
     * This is the whole distinction between a Termux that works and one that does not, so it is
     * the test the appliance applies rather than anything Termux reports about itself.
     */
    const val UNUSABLE_TARGET_SDK = 29

    /** The F-Droid build this appliance installs, and the version its repository serves. */
    const val FDROID_VERSION_NAME = "0.118.3"
    const val FDROID_VERSION_CODE = 1002

    /**
     * Where that build is downloaded from.
     *
     * A version-pinned path in F-Droid's own repository, which is the canonical source for it
     * and the one its own client installs from. The URL is pinned rather than discovered
     * because discovering it means fetching an index whose size is measured in tens of
     * megabytes, and because a downloaded APK is verified against [FDROID_SIGNER_SHA256]
     * before anything is offered to the installer — so a repository that served something else
     * under this name would be refused rather than installed.
     */
    const val FDROID_APK_URL = "https://f-droid.org/repo/com.termux_$FDROID_VERSION_CODE.apk"

    /** The repository that URL belongs to, named for the readout. */
    const val FDROID_HOST = "f-droid.org"

    /**
     * The SHA-256 of the certificate F-Droid signs its Termux with, as lowercase hexadecimal
     * with no separators.
     *
     * `CN=FDroid, OU=FDroid, O=fdroid.org, C=UK`. F-Droid's builds and the Termux project's own
     * releases are signed by different keys and are not interchangeable — switching between
     * them requires uninstalling, which deletes Termux's home directory — so which one a
     * device has is a matter of record rather than of trust.
     */
    const val FDROID_SIGNER_SHA256 = "228fb2cfe90831c1499ec3ccaf61e96e8e1ce70766b9474672ce427334d41c42"

    /** A certificate fingerprint in the one form comparisons are made in. */
    fun normaliseFingerprint(sha256: String): String =
        sha256.replace(":", "").replace(" ", "").lowercase(Locale.ROOT)
}

/** What is installed on this device, as far as the appliance can tell. */
enum class TermuxBuild {
    /** Nothing with Termux's package name is installed. */
    ABSENT,

    /** A modern build, which cannot execute what it installs. */
    MODERN,

    /** A build with a `targetSdk` old enough to execute its own `$PREFIX`. */
    USABLE,
}

/** What the device reports about the Termux that is installed, if any. */
data class TermuxStatus(
    val build: TermuxBuild,
    val versionName: String? = null,
    val versionCode: Long = 0,
    val targetSdk: Int = 0,
) {
    /** Whether this build can run the client. */
    val usable: Boolean get() = build == TermuxBuild.USABLE
}

/** Turns a package name, a version and its `targetSdk` into what the appliance should say. */
object TermuxReport {

    /**
     * Describes what the appliance found, in terms of what to do about it.
     *
     * The refusal is the interesting half: a Play Store Termux looks installed, reports a
     * version, and fails at the first attempt to run anything, with a message about a
     * permission that names nothing a person can act on.
     */
    fun describe(status: TermuxStatus): String = when (status.build) {
        TermuxBuild.ABSENT ->
            "Termux is not installed. Tap 'Install Termux' and the appliance will fetch the " +
                "F-Droid build and check its signature before offering it to Android."
        TermuxBuild.MODERN ->
            "the Termux on this device has targetSdk ${status.targetSdk} (version " +
                "${status.versionName ?: "unknown"}), which cannot run programs it installs; " +
                "uninstall it and install the F-Droid build instead, or the client cannot run"
        TermuxBuild.USABLE ->
            "Termux ${status.versionName ?: "unknown"} (targetSdk ${status.targetSdk}) is " +
                "installed, which is the build the client needs"
    }

    /** Describes a build that was found, with the two numbers the check is made of. */
    fun probe(packageName: String, versionName: String?, versionCode: Long, targetSdk: Int): TermuxStatus = TermuxStatus(
        build = when {
            versionName == null -> TermuxBuild.ABSENT
            targetSdk >= TermuxPackage.UNUSABLE_TARGET_SDK -> TermuxBuild.MODERN
            else -> TermuxBuild.USABLE
        },
        versionName = versionName,
        versionCode = versionCode,
        targetSdk = targetSdk,
    )
}

/**
 * What an APK on disk says about itself.
 *
 * Written as data rather than read from the platform at the point of decision, because the
 * decision — is this the F-Droid Termux and nothing else — is the security-relevant half and
 * has to be assertable without a device or an APK.
 */
data class ArchiveIdentity(
    val packageName: String?,
    val versionName: String?,
    val versionCode: Long,
    /** The SHA-256 of each signing certificate, in whatever form the platform gave it. */
    val signers: List<String>,
)

/** Whether an APK may be offered to Android's installer, and why not when it may not. */
data class InstallVerdict(val installable: Boolean, val reason: String)

/**
 * The rule that decides whether a downloaded APK is installed.
 *
 * Two things are checked and nothing else: that it is Termux, and that it carries the F-Droid
 * signing key. Everything else about it is F-Droid's business. A download that fails either
 * check is never offered to the installer, so a repository that was compromised, a redirect
 * that landed somewhere else, or a file that was not an APK at all ends in a report rather
 * than in an installation.
 */
object TermuxArchive {

    fun judge(
        identity: ArchiveIdentity?,
        expected: String = TermuxPackage.FDROID_SIGNER_SHA256,
    ): InstallVerdict {
        if (identity == null) {
            return InstallVerdict(false, "the download is not an Android package the system can read")
        }
        if (identity.packageName != TermuxPackage.PACKAGE) {
            return InstallVerdict(
                false,
                "the download is ${identity.packageName ?: "a package with no name"}, not ${TermuxPackage.PACKAGE}",
            )
        }
        if (identity.signers.isEmpty()) {
            return InstallVerdict(false, "the download carries no signature")
        }
        val wanted = TermuxPackage.normaliseFingerprint(expected)
        if (identity.signers.none { TermuxPackage.normaliseFingerprint(it) == wanted }) {
            return InstallVerdict(
                false,
                "the download is signed by " +
                    identity.signers.joinToString(", ") { TermuxPackage.normaliseFingerprint(it) } +
                    ", which is not F-Droid's key; it was not installed",
            )
        }
        return InstallVerdict(
            true,
            "${TermuxPackage.PACKAGE} ${identity.versionName ?: TermuxPackage.FDROID_VERSION_NAME}'s " +
                "signature is F-Droid's",
        )
    }
}

/** What the appliance did about Termux, in the terms its screen shows it. */
data class ProvisionOutcome(val installed: Boolean, val notes: List<String>)

/**
 * Fetches the F-Droid Termux, checks it, and offers it to Android's installer.
 *
 * Every step is injected, because every one of them needs either a network, a package manager
 * or an APK: what is asserted in the tests is the order and the refusal, not the platform.
 *
 * @param fetch downloads a URL to a file, returning whether it did.
 * @param read reads an APK's identity, or null when the system will not parse it.
 * @param install hands a verified APK to the installer, which asks the person to confirm.
 * @param scratch a directory this application owns, for the download.
 */
class TermuxInstaller(
    private val fetch: (String, File) -> Boolean,
    private val read: (File) -> ArchiveIdentity?,
    private val install: (File) -> Result<Unit>,
    private val scratch: File,
) {

    /** Runs the whole provision, reporting each step. */
    fun provision(url: String = TermuxPackage.FDROID_APK_URL): ProvisionOutcome {
        val notes = mutableListOf<String>()
        val apk = File(scratch, "termux.apk")
        apk.delete()

        notes += "downloading $url"
        if (!fetch(url, apk) || !apk.isFile) {
            apk.delete()
            notes += "the download failed; install Termux from $TermuxPackage.FDROID_HOST yourself, " +
                "or from F-Droid's own client"
            return ProvisionOutcome(installed = false, notes = notes)
        }

        val verdict = TermuxArchive.judge(read(apk))
        notes += verdict.reason
        if (!verdict.installable) {
            // The download is deleted rather than kept: there is nothing here worth retrying,
            // and a rejected package left in the app's own storage is one somebody could hand
            // to the installer by hand later.
            apk.delete()
            return ProvisionOutcome(installed = false, notes = notes)
        }

        notes += "offering it to Android's installer, which will ask you to confirm"
        return install(apk).fold(
            onSuccess = { ProvisionOutcome(installed = true, notes = notes) },
            onFailure = { failure ->
                notes += "the system refused to start the install: ${failure.message}"
                ProvisionOutcome(installed = false, notes = notes)
            },
        )
    }
}

/**
 * Downloads a file over HTTP, reporting whether it did.
 *
 * The response code and the length are both checked, because a captive portal or a 404 body is
 * a file of the right shape and the wrong contents, and the size is capped because the URL is
 * a name in a document and not a promise: a download that answers with something enormous
 * would fill the device's cache before anything looked at it.
 *
 * @param limit the most bytes that will be accepted.
 */
class UrlFetcher(
    private val connectTimeoutMs: Int = 15_000,
    private val readTimeoutMs: Int = 60_000,
    private val limit: Long = 128L * 1024 * 1024,
) {
    operator fun invoke(url: String, destination: File): Boolean {
        var connection: HttpURLConnection? = null
        return try {
            connection = URL(url).openConnection() as HttpURLConnection
            connection.instanceFollowRedirects = true
            connection.connectTimeout = connectTimeoutMs
            connection.readTimeout = readTimeoutMs
            connection.setRequestProperty("User-Agent", USER_AGENT)
            if (connection.responseCode != HttpURLConnection.HTTP_OK) {
                return false
            }
            if (connection.contentLengthLong > limit) {
                return false
            }
            destination.parentFile?.mkdirs()
            val staging = File(destination.absolutePath + ".part")
            connection.inputStream.use { input -> write(input, staging, limit) }
            staging.renameTo(destination)
        } catch (failed: IOException) {
            false
        } finally {
            connection?.disconnect()
        }
    }

    /** Writes [input] to [staging], refusing to write more than [limit] bytes. */
    private fun write(input: InputStream, staging: File, limit: Long) {
        var written = 0L
        val buffer = ByteArray(64 * 1024)
        staging.outputStream().use { output ->
            while (true) {
                val read = input.read(buffer)
                if (read < 0) {
                    break
                }
                written += read
                if (written > limit) {
                    throw IOException("the download is larger than the $limit-byte limit")
                }
                output.write(buffer, 0, read)
            }
        }
    }

    companion object {
        /** Named so that a repository can tell an appliance from a browser. */
        const val USER_AGENT = "gonomadnet-android-appliance"
    }
}

/**
 * Hands a verified APK to Android's package installer.
 *
 * It is the installer's own screen that asks the person to confirm and that does the install;
 * this only opens a session and gives it the bytes. The result comes back on a broadcast, which
 * [InstallResultReceiver] handles — including the one case that has to be acted on rather than
 * logged: an install that needs the person's confirmation arrives as an `Intent` to start.
 */
class PackageInstallerHandoff(private val context: Context) {

    /** Opens a session for [apk] and commits it. */
    fun install(apk: File): Result<Unit> = runCatching {
        val installer = context.packageManager.packageInstaller
        val parameters = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
        val sessionId = installer.createSession(parameters)
        installer.openSession(sessionId).use { session ->
            session.openWrite(apk.name, 0, apk.length()).use { output ->
                apk.inputStream().use { it.copyTo(output) }
                session.fsync(output)
            }
            session.commit(InstallResultReceiver.confirmation(context, sessionId).intentSender)
        }
    }

    companion object {
        /**
         * Whether the appliance may install a package at all.
         *
         * It is a setting of its own on every Android version since 8 — "install unknown apps"
         * — and it is off until somebody turns it on, so a provision that skipped this check
         * would fail at the last step with a message about a permission.
         */
        fun mayInstall(context: Context): Boolean =
            Build.VERSION.SDK_INT < Build.VERSION_CODES.O ||
                context.packageManager.canRequestPackageInstalls()
    }
}

/** Reads an APK's identity through the platform's own archive parser. */
object AndroidArchiveIdentity {

    /**
     * Reads the package name, the version and the signing certificates of an APK on disk.
     *
     * `GET_SIGNING_CERTIFICATES` is asked for rather than the deprecated signer queries: it is
     * the only one that reports the certificates an APK in *this* format was signed with, and
     * its answer is what the F-Droid key is compared against.
     */
    fun read(context: Context, apk: File): ArchiveIdentity? {
        val flags = PackageManager.GET_SIGNING_CERTIFICATES
        val info = context.packageManager.getPackageArchiveInfo(apk.absolutePath, flags) ?: return null
        val signers = info.signingInfo?.apkContentsSigners?.map { fingerprint(it.toByteArray()) } ?: emptyList()
        return ArchiveIdentity(
            packageName = info.packageName,
            versionName = info.versionName,
            versionCode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) info.longVersionCode else @Suppress("DEPRECATION") info.versionCode.toLong(),
            signers = signers,
        )
    }

    /** The SHA-256 of a certificate, as lowercase hexadecimal with no separators. */
    fun fingerprint(certificate: ByteArray): String =
        java.security.MessageDigest.getInstance("SHA-256")
            .digest(certificate)
            .joinToString("") { "%02x".format(it) }
}

/** Probes the Termux on this device through the package manager. */
object AndroidTermuxProbe {

    /** What the device reports about Termux, or that it is absent. */
    fun probe(context: Context): TermuxStatus {
        val info = runCatching { context.packageManager.getPackageInfo(TermuxPackage.PACKAGE, 0) }.getOrNull()
            ?: return TermuxStatus(TermuxBuild.ABSENT)
        val application = info.applicationInfo
        val targetSdk = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N && application != null) {
            application.targetSdkVersion
        } else {
            0
        }
        val versionCode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            info.longVersionCode
        } else {
            @Suppress("DEPRECATION") info.versionCode.toLong()
        }
        return TermuxReport.probe(TermuxPackage.PACKAGE, info.versionName, versionCode, targetSdk)
    }
}
