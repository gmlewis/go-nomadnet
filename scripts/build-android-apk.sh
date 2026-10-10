#!/bin/bash

# build-android-apk.sh builds, tests and signs the gonomadnet Android appliance.
#
# It exists because the appliance is not a Go program: it is eight Go programs, a Kotlin
# application that supervises them, and a signing key. Getting all of that into one
# installable artifact has to be one command, or it is a checklist that goes stale.
#
# The Go toolchain builds the four linux/arm64 daemons, the client itself, the console host
# that gives the client a terminal under Android, its editor, and the radio bridge that
# gives the transport a serial path to the RNode, and the Android Gradle Plugin packages all
# eight under a .so name in nativeLibraryDir — the only extension the packager keeps, and
# the only directory an app with targetSdk >= 29 is allowed to execute a file from. They are
# all built for the same Linux/arm64, because Android is Linux. The app runs the client
# itself, so one APK is the whole appliance: nothing else has to be installed on the
# device, and there is nothing in it for another application to run.
#
#   ./scripts/build-android-apk.sh              build a signed release APK
#   ./scripts/build-android-apk.sh --debug      build an installable debug APK
#   ./scripts/build-android-apk.sh --test-only  run the Kotlin unit tests and stop
#   ./scripts/build-android-apk.sh --help       show this text
#
# The Android unit tests are deliberately NOT part of run-all-tests.sh: that gate is
# the Go repositories' gate and has to keep working on a machine with no Android SDK.
# Gradle's own unit-test task is invoked here, before the assemble, so the split is
# one command rather than two habits.
#
# Every Gradle invocation passes --no-daemon. A Gradle daemon is meant to outlive the
# build that started it and idle out on its own, which is exactly right for a person
# at a keyboard and exactly wrong for a script: it survives the shell that started it,
# is reparented to init, and is then an unexplained JVM holding 3 GB for three hours.
# A single-use daemon exits with the build.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ANDROID_DIR="$REPO_ROOT/android"
GRADLE="$ANDROID_DIR/gradlew"
JNI_DIR="$ANDROID_DIR/app/src/main/jniLibs/arm64-v8a"
ASSETS_DIR="$ANDROID_DIR/app/src/main/assets"
RETICULUM_DIR="$(cd "$REPO_ROOT/../go-reticulum" 2>/dev/null && pwd || echo "")"

MODE="release"
for arg in "$@"; do
  case "$arg" in
    --help|-h)
      sed -n '2,32p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    --debug) MODE="debug" ;;
    --test-only) MODE="test" ;;
    *)
      echo "unrecognized argument: $arg" >&2
      echo "run with --help" >&2
      exit 2
      ;;
  esac
done

if [ -z "$RETICULUM_DIR" ]; then
  echo "go-reticulum is expected beside this repository at ../go-reticulum" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# The version is derived, never hand-maintained
# ---------------------------------------------------------------------------

# versionFile is the one place a release version is written, and the release
# publisher already reads it. A versionCode that goes backwards is a silent
# install failure, so it is computed from the same string rather than typed.
VERSION_FILE="$REPO_ROOT/nomadnet/version/version.go"
VERSION_NAME="$(sed -n 's/^const VERSION = "\(.*\)"/\1/p' "$VERSION_FILE" | head -1)"
if [ -z "$VERSION_NAME" ]; then
  echo "could not read VERSION from $VERSION_FILE" >&2
  exit 1
fi
IFS=. read -r v_major v_minor v_patch <<< "$VERSION_NAME"
VERSION_CODE=$((v_major * 10000 + v_minor * 100 + v_patch))
echo "building gonomadnet $VERSION_NAME (versionCode $VERSION_CODE)"

# ---------------------------------------------------------------------------
# The Go programs the appliance runs
# ---------------------------------------------------------------------------

# wagoSupportedTarget mirrors what the release publisher does: the in-process wasm
# runtime compiles for linux/arm64 with CGO_ENABLED=0, and the tag is additive.
#
# The directory is a parameter because the appliance's programs come from two
# repositories: the four daemons from go-reticulum, and the client and its console host
# from this one.
build_binary() {
  local dir="$1" package="$2" output="$3" tags="$4"
  echo "  $output"
  ( cd "$dir" && \
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
    go build -trimpath $tags -o "$JNI_DIR/$output" "./$package" )
}

# The launcher icon is rendered in assets/ and staged into the Gradle project's res/ here.
#
# assets/ is the single source of truth: it carries the whole icon family the
# renderer emits, including the iOS sizes and the mascot, and regenerating an icon
# rewrites it. Nothing derives the Android subset from it automatically, so without
# this step the application simply has no android:icon to point at and the launcher
# draws the platform's default green robot in every launcher, the task switcher and
# the settings list.
#
# The file names are the renderer's own, one shape per kind and density, so the
# mapping is read from them rather than listed: a new density is picked up by adding
# the file to assets/ and nothing else. Android resolves the density buckets at
# install time, and mipmap-anydpi-v26 replaces the flat PNG for API 26 and later with
# the adaptive icon, whose two layers are the foreground and background sets below.
stage_launcher_icons() {
  local src="$REPO_ROOT/assets" dst="$ANDROID_DIR/app/src/main/res"
  local f base kind density dir resname staged=0

  for f in "$src"/gonomadnet-android-*.png "$src"/gonomadnet-android-*.xml; do
    [ -f "$f" ] || continue
    base="${f##*/}"
    base="${base#gonomadnet-android-}"

    case "$base" in
      # The round launcher is matched before the plain one, because the plain
      # pattern's '*' would otherwise swallow "round-<density>" as the density.
      launcher-round-*)
        density="${base#launcher-round-}"; density="${density%%-*}"
        dir="mipmap-$density"; resname="ic_launcher_round.png" ;;
      launcher-*)
        density="${base#launcher-}"; density="${density%%-*}"
        dir="mipmap-$density"; resname="ic_launcher.png" ;;
      adaptive-foreground-*)
        density="${base#adaptive-foreground-}"; density="${density%%-*}"
        dir="mipmap-$density"; resname="ic_launcher_foreground.png" ;;
      adaptive-background-*)
        density="${base#adaptive-background-}"; density="${density%%-*}"
        dir="mipmap-$density"; resname="ic_launcher_background.png" ;;
      adaptive-ic_launcher_round-anydpi-v26.xml)
        dir="mipmap-anydpi-v26"; resname="ic_launcher_round.xml" ;;
      adaptive-ic_launcher-anydpi-v26.xml)
        dir="mipmap-anydpi-v26"; resname="ic_launcher.xml" ;;
      *) continue ;;
    esac

    mkdir -p "$dst/$dir"
    cp "$f" "$dst/$dir/$resname"
    staged=$((staged + 1))
  done

  if [ "$staged" -eq 0 ]; then
    echo "no launcher icons found in $src: the APK would install with the platform's default icon" >&2
    return 0
  fi
  echo "  staged $staged launcher icon files"
}

if [ "$MODE" != "test" ]; then
  mkdir -p "$JNI_DIR" "$ASSETS_DIR"
  echo "building the bundled daemons for linux/arm64:"
  # The .so extension is required, not cosmetic: without it the packager does not
  # include the file, and nativeLibraryDir stays empty.
  build_binary "$RETICULUM_DIR" "cmd/gornsd"   "libgornsd.so"   "-tags=wago"
  build_binary "$RETICULUM_DIR" "cmd/gorrcd"   "libgorrcd.so"   "-tags=wago"
  build_binary "$RETICULUM_DIR" "cmd/gorrcbot" "libgorrcbot.so" ""
  build_binary "$RETICULUM_DIR" "cmd/gonsensor" "libgonsensor.so" ""

  echo "building the client, the console host that gives it a terminal, and its editor:"
  # The radio bridge is the eighth program and it belongs to this repository: it is what
  # allocates the pseudo-terminal the transport is dialled at, because Android gives an
  # application no serial device to open at all. It carries no wasm runtime and so needs no
  # build tag, and it is named under android/ because nothing but the appliance ever runs it.
  # The client is executed by the app, not read by it, and the console host is the
  # program the app spawns to put that client on a pseudo-terminal, so both belong in
  # nativeLibraryDir under a .so name. Neither exists anywhere else on the device.
  #
  # The editor is there for the same reason and by the same rule: the client opens one
  # on its configuration file, and Android gives an application no editor to open — no
  # nano, no vi an app may execute, and no way to look a bare name up. The client's
  # editor setting names this binary, by the absolute path it lands at.
  build_binary "$REPO_ROOT" "cmd/gonomadnet"     "libgonomadnetclient.so" "-tags=wago"
  build_binary "$REPO_ROOT" "android/console"   "libgorcons.so"          "-tags=wago"
  build_binary "$REPO_ROOT" "android/editor"    "libgonomadnetedit.so"   "-tags=wago"
  build_binary "$REPO_ROOT" "android/rnode"     "libgornnode.so"         ""
  du -ch "$JNI_DIR"/*.so | tail -1

  echo "staging the launcher icon:"
  stage_launcher_icons
fi

# ---------------------------------------------------------------------------
# The pinned contract between the appliance's transport and its client
# ---------------------------------------------------------------------------

# shared_instance_type, shared_instance_port and instance_name have to agree between
# the configuration the appliance renders for its own transport and the one its
# attached client reads. A mismatch produces "no shared instance is running" and
# nothing else, which is the least debuggable failure this project can produce.
#
# Both configurations are rendered from the constants read here, and the rendering is
# asserted against those constants by the Android unit tests, which run below. So what
# is printed here is the pin itself, read from the one place it is written.
pinned_assertions() {
  local kotlin="$ANDROID_DIR/app/src/main/java/com/gmlewis/gonomadnet/NodeConfig.kt"
  local port name
  port="$(sed -n 's/.*DEFAULT_SHARED_INSTANCE_PORT = \([0-9]*\).*/\1/p' "$kotlin" | head -1)"
  name="$(sed -n 's/.*DEFAULT_INSTANCE_NAME = "\([^"]*\)".*/\1/p' "$kotlin" | head -1)"
  if [ -z "$port" ] || [ -z "$name" ]; then
    echo "could not read the pinned shared-instance values from $kotlin" >&2
    exit 1
  fi
  echo "pinned shared instance: tcp $name on 127.0.0.1:$port"

  # The sensor feed port is pinned on both sides for the same reason.
  local feed
  feed="$(sed -n 's/.*DEFAULT_FEED_PORT = \([0-9]*\).*/\1/p' \
    "$ANDROID_DIR/app/src/main/java/com/gmlewis/gonomadnet/SensorFeedServer.kt" | head -1)"
  echo "pinned sensor feed:      tcp 127.0.0.1:$feed"

  # The programs this repository contributes are what put the client on a terminal inside the
  # app and give the transport a radio to dial, and a build that quietly omitted one of them
  # produces an APK whose console spawns nothing at all, or whose radio is an interface
  # enabled against a path that names nothing. Only a build can answer for them: in
  # --test-only mode nothing was cross-compiled, so their absence means nothing.
  if [ "$MODE" != "test" ]; then
    for so in libgonomadnetclient.so libgorcons.so libgonomadnetedit.so libgornnode.so; do
      if [ ! -f "$JNI_DIR/$so" ]; then
        echo "$so was not built into $JNI_DIR" >&2
        exit 1
      fi
    done
  fi
}

pinned_assertions

# ---------------------------------------------------------------------------
# The Android tests: this repository's second gate
# ---------------------------------------------------------------------------

echo "running the Android unit tests"
( cd "$ANDROID_DIR" && "$GRADLE" --no-daemon --quiet testDebugUnitTest \
    -PgonomadnetVersionName="$VERSION_NAME" -PgonomadnetVersionCode="$VERSION_CODE" )
echo "the Android unit tests passed"

if [ "$MODE" = "test" ]; then
  echo "test-only: stopping before the assemble"
  exit 0
fi

# ---------------------------------------------------------------------------
# The APK
# ---------------------------------------------------------------------------

if [ "$MODE" = "release" ] && [ ! -f "$ANDROID_DIR/keystore.properties" ]; then
  cat >&2 <<'MESSAGE'
a release build needs a signing key, and cannot be upgraded in place without one.

Create one outside the repository:

  keytool -genkeypair -keystore ~/.android-keys/gonomadnet-release.jks \
    -alias gonomadnet -keyalg RSA -keysize 4096 -validity 10950 \
    -storetype PKCS12 -dname "CN=gonomadnet, O=you, C=US"

then copy android/keystore.properties.example to android/keystore.properties and
fill in the path and the passwords. Both files are gitignored. Use --debug for a
throwaway build that needs no key.
MESSAGE
  exit 1
fi

TASK="assembleRelease"
APK="$ANDROID_DIR/app/build/outputs/apk/release/app-release.apk"
if [ "$MODE" = "debug" ]; then
  TASK="assembleDebug"
  APK="$ANDROID_DIR/app/build/outputs/apk/debug/app-debug.apk"
fi

echo "assembling the $MODE APK"
( cd "$ANDROID_DIR" && "$GRADLE" --no-daemon --quiet "$TASK" \
    -PgonomadnetVersionName="$VERSION_NAME" -PgonomadnetVersionCode="$VERSION_CODE" )

if [ ! -f "$APK" ]; then
  echo "the build reported success but $APK is not there" >&2
  exit 1
fi

# The signer is verified rather than assumed: an unsigned or differently-signed APK
# installs fine over nothing and fails on the next release, which is a bad way to
# find out.
APKSIGNER="$(ls "$ANDROID_HOME"/build-tools/*/apksigner 2>/dev/null | sort -V | tail -1)"
if [ -n "$APKSIGNER" ]; then
  echo "verifying the signature"
  "$APKSIGNER" verify --print-certs "$APK" | sed -n '1,6p'
else
  echo "note: apksigner was not found under \$ANDROID_HOME, so the signature was not checked"
fi

# Stage the APK where the release publisher looks for it, under the name it will be
# published as. The publisher never builds an appliance — see androidStagingDir in
# cmd/publish-github-release-artifacts/main.go — so this hand-off is the whole reason the two
# scripts know about each other.
STAGE="$REPO_ROOT/dist/android"
STAGED="$STAGE/gonomadnet-$VERSION_NAME-android-arm64-v8a.apk"
mkdir -p "$STAGE"

# The directory the publisher enumerates is a staging area, and it only ever added: every
# local appliance build left another APK behind for good, and the publisher re-read and
# re-skipped all of them on every run. The skipping is deliberate — an appliance built from
# another release's daemons must not be published as this one's — but the accumulation that
# makes it necessary is not, so the directory is left holding exactly the appliance this
# build produced.
for stale in "$STAGE"/gonomadnet-*-android-*.apk; do
  if [ -e "$stale" ] && [ "$stale" != "$STAGED" ]; then
    rm -f "$stale"
    echo "removed the appliance staged for another release: $(basename "$stale")"
  fi
done

cp "$APK" "$STAGED"

echo
echo "APK:    $APK"
echo "staged: $STAGED"
echo "sha256: $(shasum -a 256 "$APK" | cut -d' ' -f1)"
echo "size:   $(du -h "$APK" | cut -f1)"
