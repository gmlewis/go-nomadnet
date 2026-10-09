# Building and releasing the Android app

This page is for building, signing and publishing the **gonomadnet node**
appliance. It is not needed to *use* it — if you want to install the app on a
device, read [Android-APK.md](Android-APK.md) instead, and download the APK from
the [latest release](https://github.com/gmlewis/go-nomadnet/releases).

---

## Building the APK

```sh
./scripts/build-android-apk.sh              # a signed release APK
./scripts/build-android-apk.sh --debug      # an installable debug APK
./scripts/build-android-apk.sh --test-only  # the Kotlin unit tests only
```

The script cross-compiles **six** Go programs for `linux/arm64` and stages them as
`.so` files in `jniLibs` — the only extension the packager keeps, and the only
directory an app that targets the current `targetSdk` may execute a file from:

| Staged as | Built from | What it is |
| --- | --- | --- |
| `libgornsd.so` | `go-reticulum` | the Reticulum transport |
| `libgorrcd.so` | `go-reticulum` | the RRC hub |
| `libgorrcbot.so` | `go-reticulum` | the chat bot |
| `libgonsensor.so` | `go-reticulum` | the sensor converters |
| `libgonomadnetclient.so` | this repository | the client |
| `libgorcons.so` | this repository | the console host that gives that client a PTY |

It then runs the Kotlin unit tests, assembles the APK, verifies the signature,
and leaves it in `dist/android/`. The version comes from
`nomadnet/version/version.go`, and `versionCode` is derived from the same string
(`major * 10000 + minor * 100 + patch`), so an upgrade cannot install backwards.

The six programs total ~96 MB before compression, and a debug APK is ~74 MB.

**The client is built once.** It used to be staged a second time as the asset
`gonomadnet-client`, for another application to run out of shared storage — worth
another ~16 MB in the download. The app runs `libgonomadnetclient.so` itself now,
so that copy, the launchers and the setup script that went with it are gone. What
is still in `assets/` is the console's font and its license, the terminal colors —
which are the console's own palette as well as the theme the client installs for a
Termux user — the tmux configuration, and the bundled Bible text the bot's `kjv`
command reads.

Building needs the Android SDK and a signing key. The release publisher never
builds an APK, so a machine without either can still cut a Go release.

### Signing

The release key lives **outside the repository**, at
`~/.android-keys/gonomadnet-release.jks`, with its passwords in the gitignored
`android/keystore.properties`. Only `android/keystore.properties.example` is
committed.

Certificate SHA-256:
`a52688c9ef9fa2ffc6545384dc81389f48b75402fe73ffe9b963f3218a45a6f6`,
valid until 2056-09-30.

**Keep this key for the life of the appliance.** Changing it between releases
forces every user to uninstall, which for an appliance that holds a node identity
means losing that identity.

### Build settings that matter

```kotlin
android {
    compileSdk = 36
    defaultConfig {
        minSdk = 24
        targetSdk = 34
    }
    packaging {
        jniLibs { useLegacyPackaging = true }   // so the .so really lands on disk
    }
}
```

and in the manifest, `<application android:extractNativeLibs="true">`. Without
both, the `.so` files stay inside the APK and `nativeLibraryDir` does not contain
them.

Two further notes for anyone editing the Gradle build:

- **Do not apply `org.jetbrains.kotlin.android`.** Android Gradle Plugin 9.1.0
  registers the `kotlin` extension itself, and applying the Kotlin plugin on top
  fails with `Cannot add extension with name 'kotlin'`. Sources under
  `src/main/java/**/*.kt` compile with the Kotlin compiler AGP embeds, so no
  plugin and no plugin marker are needed — which also keeps the build offline.
- Stripping a Go binary warns harmlessly (`Unable to strip … libprobe.so`); there
  is no `strip` tool for a Go object, and nothing that matters to strip.

### Publishing with a release

`scripts/publish-github-release-artifacts.sh` attaches a staged APK to the
**go-nomadnet** GitHub release as
`gonomadnet-<version>-android-arm64-v8a.apk`.

- **The publisher never builds it.** The APK has to be staged in `dist/android/`
  first, by `scripts/build-android-apk.sh`. When none is staged, the run says so
  and carries on: a release without an APK is a normal release.
- An APK whose name does not carry the version being released is **refused
  rather than attached**, so a release cannot ship an appliance built from other
  sources.
- The retention prune **never deletes an APK**, however old the release. An
  install artifact is not a build output: the binaries the prune retires are
  rebuilt for every release, while an APK is a signed thing somebody installed
  once and must be able to fetch again to upgrade in place.
- The release notes name the cross-repo pin, because the APK embeds daemons
  compiled from the sibling `go-reticulum` module.

## Tests

The Go repositories' gate, `./run-all-tests.sh`, must keep working on a machine
with no Android SDK, so the Android tests are not part of it. They are their own
gate:

```sh
cd android && ./gradlew testDebugUnitTest
```

`scripts/build-android-apk.sh` runs them before it assembles. Do not "fix" the
split by adding Gradle to `run-all-tests.sh`.
