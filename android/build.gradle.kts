// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Android Gradle Plugin 9.1.0 ships Kotlin support of its own. Applying
// org.jetbrains.kotlin.android on top of it fails with "Cannot add extension
// with name 'kotlin', as there is an extension already registered with that
// name", so this build deliberately applies the Android plugin only: AGP
// compiles the .kt sources itself, and the build needs no Kotlin plugin and no
// Kotlin plugin marker from the network.
plugins {
    id("com.android.application") version "9.1.0" apply false
}
