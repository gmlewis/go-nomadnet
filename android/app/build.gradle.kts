// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

import java.util.Properties

plugins {
    id("com.android.application")
}

// The release key lives outside the repository; only the example file is
// committed. Without it the release build is unsigned and the script refuses to
// install it, which is the right failure: an unsigned appliance cannot be
// upgraded in place later.
val keystorePropertiesFile = rootProject.file("keystore.properties")
val keystoreProperties = Properties().apply {
    if (keystorePropertiesFile.exists()) {
        keystorePropertiesFile.inputStream().use { load(it) }
    }
}

// The version is derived from the Go module's own version file rather than
// hand-maintained: a versionCode that goes backwards is a silent install
// failure. scripts/build-android-apk.sh passes both in.
val derivedVersionName: String = (project.findProperty("gonomadnetVersionName") as String?) ?: "0.0.0"
val derivedVersionCode: Int = ((project.findProperty("gonomadnetVersionCode") as String?) ?: "1").toInt()

android {
    namespace = "com.gmlewis.gonomadnet"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.gmlewis.gonomadnet"
        minSdk = 24
        targetSdk = 34
        versionCode = derivedVersionCode
        versionName = derivedVersionName
    }

    signingConfigs {
        if (keystorePropertiesFile.exists()) {
            create("release") {
                storeFile = file(keystoreProperties.getProperty("storeFile"))
                storePassword = keystoreProperties.getProperty("storePassword")
                keyAlias = keystoreProperties.getProperty("keyAlias")
                keyPassword = keystoreProperties.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            if (keystorePropertiesFile.exists()) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }

    // The bundled daemons are Go binaries shipped under a .so name: that is the
    // only extension the packager keeps, and nativeLibraryDir is the only place
    // an app with targetSdk >= 29 is allowed to execute a file from.
    // useLegacyPackaging keeps them on disk instead of leaving them inside the
    // APK, which is what makes nativeLibraryDir non-empty.
    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
    }

    // The Bible text the bot's kjv command reads ships as a stored asset rather than a
    // deflated one. It is 4.4 MB, and a compressed asset comes back through the platform's
    // decompressing stream rather than as the bytes that were packed: storing it keeps the
    // file inside the APK identical to the file on disk and the read an ordinary one. The
    // APK is roughly 2.5 MB larger for it.
    androidResources {
        noCompress.add("txt")
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    testOptions {
        unitTests {
            isReturnDefaultValues = true
        }
    }
}

dependencies {
    // The appliance itself needs nothing: a plain Activity, a foreground Service, and the
    // platform APIs keep the dependency surface, the build, and the APK small. JUnit is
    // for the unit tests only, and is not packaged.
    testImplementation("junit:junit:4.13.2")
}
