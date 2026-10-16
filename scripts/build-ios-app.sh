#!/bin/bash

# Copyright 2026 Glenn Lewis. All rights reserved.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.

# build-ios-app.sh builds, tests and packages the gonomadnet iPadOS/iOS appliance.
#
# It exists for the same reason the Android one does, and for one reason more. The
# Android appliance is eight Go programs and a Kotlin application that starts them.
# The iOS appliance is one Go program and a Swift application that talks to it across
# a C ABI, because an iOS process may not start a program at all. Getting a Go
# library, a Swift package, a terminal font, a 4.4 MB Bible and a set of icons into
# one installable artifact has to be one command, or it is a checklist that goes stale.
#
#   ./scripts/build-ios-app.sh              build a release .ipa in dist/ios/
#   ./scripts/build-ios-app.sh --debug      build an installable debug .app
#   ./scripts/build-ios-app.sh --simulator  build a debug .app for the Simulator
#   ./scripts/build-ios-app.sh --test-only  run the Go and Swift unit tests and stop
#   ./scripts/build-ios-app.sh --acceptance run the device acceptance test on a device
#   ./scripts/build-ios-app.sh --help       show this text
#
# A device build is signed, and a release build is signed the same way, with no
# environment to set: the team is read from the developer certificate in the login
# keychain, which is where Xcode put it and where Apple keeps the team identifier — in the
# certificate's own subject. Nothing is stored, and the private key never leaves the
# keychain. GONOMADNET_TEAM overrides the certificate for a machine with more than one team.
#
# A build for one named device needs GONOMADNET_DEVICE as well, because a device is not
# something a certificate can say: it is what registers the device on the developer portal
# and puts it in the provisioning profile.
#
#   GONOMADNET_DEVICE=<udid> ./scripts/build-ios-app.sh --debug
#
# It is an environment variable rather than an argument because a device UDID is the
# operator's, not the project's. A release build needs neither: it is built for
# `generic/platform=iOS`, so no device has to be attached to sign one.
#
# --acceptance needs a device, and a signing team: it is the one mode that cannot run
# anywhere else. It drives the installed appliance the way a reader would and asserts what a
# reader can check — that the first launch starts nothing, that Start Stack brings up the
# shared instance and the sensor feed, that Stop Stack releases them and they come back, that
# the console renders and takes a keystroke, that a drag scrolls the panel it is over, that
# rotating reflows it, and that Start Sensors turns the sensor path on. It is deliberately NOT
# part of --test-only or run-all-tests.sh: everything they run, runs on a machine with no
# device.
#
# The iOS tests are deliberately NOT part of run-all-tests.sh: that gate is the Go
# repositories' gate and has to keep working on a machine with no Xcode. `swift test`
# is invoked here, before the archive is built, so the split is one command rather
# than two habits.
#
# The Go half is built for GOOS=ios GOARCH=arm64 with Apple's clang, as a C archive.
# It is built from ./cmd/gonomadnet rather than from ios/bridge because a C archive
# needs a main package and the NomadNet client is one: the client's own package is
# where the runTextUI call lives (see cmd/gonomadnet/ios-host.go), and everything else
# the appliance needs is a library package under ios/.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IOS_DIR="$REPO_ROOT/ios"
ANDROID_ASSETS="$REPO_ROOT/android/app/src/main/assets"
RETICULUM_DIR="$(cd "$REPO_ROOT/../go-reticulum" 2>/dev/null && pwd || echo "")"

# The staging and archive directories live under ios/build/, which is gitignored, and
# deliberately not under /tmp: scripts/clean-test-tmp.sh sweeps /tmp during a test run
# and would delete a release's staging directory out from under it.
BUILD_DIR="$IOS_DIR/build"
STAGING_DIR="$BUILD_DIR/staging"
ARCHIVE_DIR="$BUILD_DIR/archive"
DIST_DIR="$REPO_ROOT/dist/ios"

MODE="release"
for arg in "$@"; do
  case "$arg" in
    --help|-h)
      sed -n '2,49p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    --debug) MODE="debug" ;;
    --simulator) MODE="simulator" ;;
    --test-only) MODE="test" ;;
    --acceptance) MODE="acceptance" ;;
    *)
      echo "unrecognized argument: $arg" >&2
      echo "run with --help" >&2
      exit 2
      ;;
  esac
done

# ---------------------------------------------------------------------------
# The version is derived, never hand-maintained
# ---------------------------------------------------------------------------

# versionFile is the one place a release version is written, and the release
# publisher already reads it. CFBundleVersion has to increase for a build to be
# installable over an older one, so it is computed from the same string rather than
# typed.
VERSION_FILE="$REPO_ROOT/nomadnet/version/version.go"
VERSION_NAME="$(sed -n 's/^const VERSION = "\(.*\)"/\1/p' "$VERSION_FILE" | head -1)"
if [ -z "$VERSION_NAME" ]; then
  echo "could not read VERSION from $VERSION_FILE" >&2
  exit 1
fi
IFS=. read -r v_major v_minor v_patch <<< "$VERSION_NAME"
BUILD_NUMBER=$((v_major * 10000 + v_minor * 100 + v_patch))
echo "building gonomadnet $VERSION_NAME (build $BUILD_NUMBER) for iOS"

# ---------------------------------------------------------------------------
# The Swift half's tests: this repository's second gate
# ---------------------------------------------------------------------------

# Xcode's own Swift, because Xcode is what builds the app and the package is a dependency
# of it: one toolchain from the tests to the bundle means a module cache that only ever has
# one compiler's output in it. Two Swift toolchains writing `ios/build/swift` produce a
# build that fails with "module compiled with Swift X cannot be imported by the Swift Y
# compiler" and nothing else, which reads like a broken package rather than a shared cache.
echo "running the Swift unit tests"
( cd "$IOS_DIR" && xcrun swift test --scratch-path "$BUILD_DIR/swift" )
echo "the Swift unit tests passed"

# ---------------------------------------------------------------------------
# The Go half's tests
# ---------------------------------------------------------------------------

# The bridge is ordinary Go and is tested by ordinary Go tests, on whatever machine
# is building the appliance. That is the point of keeping the C entry points thin:
# everything behind them is reachable from a test that needs no device.
echo "running the Go bridge tests"
( cd "$REPO_ROOT" && go test -race -count=1 ./ios/bridge/... )
echo "the Go bridge tests passed"

if [ "$MODE" = "test" ]; then
  echo "test-only: stopping before the archive"
  exit 0
fi

if [ -z "$RETICULUM_DIR" ]; then
  echo "go-reticulum is expected beside this repository at ../go-reticulum" >&2
  exit 1
fi

if ! xcrun --sdk iphoneos --show-sdk-path >/dev/null 2>&1; then
  echo "no iOS SDK: install Xcode, or run ./scripts/build-ios-app.sh --test-only" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# The Go half, as a C archive
# ---------------------------------------------------------------------------
#
# There are two slices and they are different builds rather than one archive with two
# architectures. Both are GOOS=ios: a Go binary built for darwin installs the alternate
# signal stack, and the simulator's runtime — like iOS itself — does not support one, so
# the first asynchronous preemption arrives on a thread with no signal stack and the
# runtime aborts with "non-Go code disabled sigaltstack". Go's ios port knows this and
# skips the signal stack; it is the *platform* that differs between the two slices, not
# the runtime's idea of it. So the simulator slice is an ios/arm64 build against the
# simulator's SDK and `-target arm64-apple-ios16.0-simulator`.
#
# Both are built with the `ios` build tag as well, which is what compiles the two files
# that are the appliance rather than the desktop client: cmd/gonomadnet/ios-host.go and
# ios/bridge/bridge.go.

MIN_IOS="${GONOMADNET_MIN_IOS:-16.0}"
mkdir -p "$ARCHIVE_DIR"

# verify_archive fails the build if any of the seven entry points is missing: a build that
# quietly dropped one produces an app that launches, shows its interface, and does
# nothing at all, and the linker would not have said a word.
verify_archive() {
  local archive="$1" symbol
  for symbol in GonomadnetStart GonomadnetStop GonomadnetStartConsole \
                GonomadnetSetScanEndpoint \
                GonomadnetFeedLocation GonomadnetFeedHeading GonomadnetVersion; do
    if ! nm -g "$archive" 2>/dev/null | grep -q "_${symbol}\$"; then
      echo "$symbol is missing from $archive" >&2
      exit 1
    fi
  done
  echo "  the archive exports every entry point"
}

build_archive() {
  local sdk_name="$1" target_flag="$2" archive="$3"
  local sdk cc
  sdk="$(xcrun --sdk "$sdk_name" --show-sdk-path)"
  cc="$(xcrun --sdk "$sdk_name" -f clang)"

  ( cd "$REPO_ROOT" && \
    GOOS=ios GOARCH=arm64 CGO_ENABLED=1 \
    CC="$cc" \
    CGO_CFLAGS="-isysroot $sdk $target_flag" \
    CGO_LDFLAGS="-isysroot $sdk $target_flag" \
    go build -tags ios -buildmode=c-archive -o "$archive" ./cmd/gonomadnet )
  verify_archive "$archive"
}

ARCHIVE="$ARCHIVE_DIR/libgonomadnet.a"
if [ "$MODE" = "simulator" ]; then
  ARCHIVE="$ARCHIVE_DIR/libgonomadnet-simulator.a"
  echo "building the Go bridge for the arm64 simulator:"
  build_archive iphonesimulator \
    "-target arm64-apple-ios$MIN_IOS-simulator -mios-simulator-version-min=$MIN_IOS" \
    "$ARCHIVE"
else
  echo "building the Go bridge for ios/arm64:"
  build_archive iphoneos "-miphoneos-version-min=$MIN_IOS -arch arm64" "$ARCHIVE"
fi

# ---------------------------------------------------------------------------
# Assets, staged rather than duplicated
# ---------------------------------------------------------------------------

# Nothing here is copied into the repository. The Bible text is 4.4 MB and the terminal
# font is 2.3 MB; both already live in the Android project's assets, and the icons are
# rendered into assets/ by the icon generator. A second copy of any of them would be a
# second thing to keep in step, and would be committed by accident.
echo "staging assets:"

stage_asset() {
  local source="$1" dest="$2"
  if [ ! -f "$source" ]; then
    echo "  missing: $source" >&2
    return 1
  fi
  mkdir -p "$(dirname "$dest")"
  cp "$source" "$dest"
  echo "  $(basename "$dest")  ($(du -h "$source" | cut -f1))"
}

rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR"

stage_asset "$ANDROID_ASSETS/kjv.txt"                          "$STAGING_DIR/Resources/kjv.txt"
stage_asset "$ANDROID_ASSETS/AtkynsonMonoNerdFontMono-Regular.otf" "$STAGING_DIR/Resources/AtkynsonMonoNerdFontMono-Regular.otf"
stage_asset "$ANDROID_ASSETS/AtkinsonHyperlegibleMono-OFL.txt" "$STAGING_DIR/Resources/AtkinsonHyperlegibleMono-OFL.txt"
stage_asset "$ANDROID_ASSETS/colors.properties"               "$STAGING_DIR/Resources/colors.properties"
stage_asset "$ANDROID_ASSETS/tmux.conf"                       "$STAGING_DIR/Resources/tmux.conf"

# The client's own default configuration is the template the appliance edits, and it
# lives in the Go module so that the client and the appliance can never disagree about
# what a default configuration contains.
stage_asset "$REPO_ROOT/nomadnet/config/testdata/default.conf" "$STAGING_DIR/Resources/default.conf"

# The asset catalog is composed here rather than committed twice. The color set the
# launch screen names lives in the repository's own catalog; the launcher icons are
# rendered into assets/ by the icon generator and mapped into the icon set by the name
# the renderer gave them, so a new size is picked up by adding the file to assets/ and
# editing the icon set's manifest, and nothing else.
stage_assets_catalog() {
  local src="$REPO_ROOT/ios/App/Assets.xcassets" dst="$STAGING_DIR/Resources/Assets.xcassets"
  local icons="$REPO_ROOT/assets" icon_set="$dst/AppIcon.appiconset"
  local f base staged=0

  if [ ! -d "$src" ]; then
    echo "  missing: $src" >&2
    return 1
  fi
  cp -R "$src" "$dst"

  for f in "$icons"/gonomadnet-ios-*.png; do
    [ -f "$f" ] || continue
    base="${f##*/}"
    cp "$f" "$icon_set/$base"
    staged=$((staged + 1))
  done

  if [ "$staged" -eq 0 ]; then
    echo "no iOS icons found in $icons: the app would install with no icon" >&2
    return 1
  fi
  echo "  Assets.xcassets  ($staged launcher icons)"
}
stage_assets_catalog

# ---------------------------------------------------------------------------
# The pinned contract between the appliance's Go half and its Swift half
# ---------------------------------------------------------------------------

# shared_instance_port, instance_name and the sensor feed port have to agree between
# the Reticulum configuration the Swift half renders and the ports the Go half binds.
# A mismatch produces "no shared instance is running" and nothing else, which is the
# least debuggable failure this project can produce.
#
# Both sides are read here from the one place each is written, and each is asserted by
# its own unit tests, so what is printed below is the pin itself rather than a copy.
pinned_assertions() {
  local go_bridge="$IOS_DIR/bridge/feed.go"
  local swift_config="$IOS_DIR/Sources/ApplianceKit/NodeConfig.swift"

  local feed_go feed_swift port_swift name_swift
  feed_go="$(sed -n 's/.*DefaultFeedPort *= *\([0-9]*\).*/\1/p' "$go_bridge" | head -1)"
  port_swift="$(sed -n 's/.*defaultSharedInstancePort *= *\([0-9]*\).*/\1/p' "$swift_config" | head -1)"
  name_swift="$(sed -n 's/.*defaultInstanceName *= *"\([^"]*\)".*/\1/p' "$swift_config" | head -1)"
  feed_swift="$(sed -n 's/.*feedPort *= *\([0-9]*\).*/\1/p' "$swift_config" | head -1)"

  if [ -z "$feed_go" ] || [ -z "$port_swift" ] || [ -z "$name_swift" ] || [ -z "$feed_swift" ]; then
    echo "could not read the pinned ports (feed_go=$feed_go port=$port_swift name=$name_swift feed=$feed_swift)" >&2
    exit 1
  fi

  echo "pinned shared instance: tcp $name_swift on 127.0.0.1:$port_swift"
  echo "pinned sensor feed:     tcp 127.0.0.1:$feed_go"

  if [ "$feed_go" != "$feed_swift" ]; then
    echo "the sensor feed port disagrees: Go binds $feed_go and Swift renders $feed_swift" >&2
    exit 1
  fi
  if [ "$feed_go" = "$port_swift" ]; then
    echo "the sensor feed is on the shared instance's port ($port_swift)" >&2
    exit 1
  fi
}
pinned_assertions

# ---------------------------------------------------------------------------
# The application
# ---------------------------------------------------------------------------

# The Xcode project is committed, because a project that is generated at build time is
# a project whose build settings are prose nobody reads. It takes the four values below
# as build settings, so it never has to be edited for a release. See docs/iOS-Build.md.
PROJECT="$IOS_DIR/App/Gonomadnet.xcodeproj"
if [ ! -d "$PROJECT" ]; then
  echo "ios/App/Gonomadnet.xcodeproj is missing: it is committed with the sources it" >&2
  echo "builds, so a checkout without it did not bring the whole repository." >&2
  exit 1
fi

SCHEME="Gonomadnet"
DERIVED="$BUILD_DIR/derived"
ARCHIVE_PATH="$BUILD_DIR/Gonomadnet.xcarchive"

# A device build is signed and a simulator build is not.

# signing_team prints the team the appliance is signed for, or nothing when this machine
# cannot sign at all. It reads the one place a Mac already keeps the answer: the developer
# certificate in the login keychain, whose OU field is the team identifier — which is how
# Xcode itself knows it, and why no team is ever written into this repository.
#
# Nothing secret is involved and nothing is stored. A team identifier is not a secret: it
# is inside every app that team has ever signed, and inside every provisioning profile. What
# is secret is the private key that goes with the certificate, and that never leaves the
# keychain — this only reads the certificate's public subject. GONOMADNET_TEAM, when it is
# set, is used instead, which is what a machine with more than one team needs.
signing_team() {
  if [ -n "${GONOMADNET_TEAM:-}" ]; then
    printf '%s' "$GONOMADNET_TEAM"
    return 0
  fi

  local name subject ou
  # A distribution certificate is what a release wants, so it is looked for first; a
  # development one signs a build that installs on a registered device, which is what this
  # appliance is given anyway.
  for name in "Apple Distribution" "Apple Development"; do
    subject="$(security find-certificate -a -c "$name" -p 2>/dev/null       | openssl x509 -noout -subject 2>/dev/null || true)"
    ou="$(printf '%s' "$subject" | sed -n 's/.*OU=\([A-Za-z0-9]*\).*/\1/p' | head -1)"
    if [ -n "$ou" ]; then
      printf '%s' "$ou"
      return 0
    fi
  done
  return 1
}

TEAM="$(signing_team || true)"

SIGN_ARGS=()
# A signed build has to be allowed to talk to the developer portal: automatic signing
# creates the provisioning profile for the target's bundle identifier on the first run, and
# refreshes it when it expires. Without this flag xcodebuild refuses to make one and fails
# before it reaches the compiler, so a team is what asks for the update and this is how it is
# granted.
SIGN_FLAGS=()
SIGNING="not-applicable"
if [ "$MODE" = "acceptance" ]; then
  # The acceptance test is run by a second app on the device, so both it and the appliance
  # it drives have to be signed and installed. There is nothing to fall back to: a test
  # runner that cannot be installed tests nothing.
  if [ -z "$TEAM" ] || [ -z "${GONOMADNET_DEVICE:-}" ]; then
    echo "--acceptance needs a signing team and a device" >&2
    echo "no team was found in the keychain: set GONOMADNET_TEAM, and GONOMADNET_DEVICE" >&2
    exit 2
  fi
  SIGN_ARGS=(DEVELOPMENT_TEAM="$TEAM" CODE_SIGN_STYLE=Automatic)
  SIGN_FLAGS=(-allowProvisioningUpdates -allowProvisioningDeviceRegistration)
  SIGNING="signed"
elif [ "$MODE" != "simulator" ]; then
  if [ -n "$TEAM" ]; then
    SIGN_ARGS=(DEVELOPMENT_TEAM="$TEAM" CODE_SIGN_STYLE=Automatic)
    SIGN_FLAGS=(-allowProvisioningUpdates)
    SIGNING="signed"
  else
    # A machine with no developer certificate — a build server — still builds: the
    # resources, the C archive and the link line are all verified, and the .ipa it produces
    # is a valid package that has to be signed before it will install.
    SIGN_ARGS=(CODE_SIGNING_ALLOWED=NO CODE_SIGNING_REQUIRED=NO CODE_SIGN_IDENTITY="")
    SIGNING="unsigned"
  fi
fi

DESTINATION_ARGS=(-destination "generic/platform=iOS")
CONFIG="Release"
ACTION="archive"
case "$MODE" in
  debug)
    CONFIG="Debug"
    ACTION="build"
    # A build for one named device is what registers that device on the developer portal:
    # automatic signing adds a connected device to the provisioning profile when the
    # destination is the device itself, and a generic destination cannot, because there is
    # no device for it to name. Without this an install fails with "This provisioning
    # profile cannot be installed on this device" even though the app signed cleanly.
    #
    # Registering it is a separate permission from updating the profile, and xcodebuild is
    # not granted it by default: without -allowProvisioningDeviceRegistration the build
    # stops with "Device ... isn't registered in your developer account". Both are asked
    # for only when the build names the device, so a generic or simulator build never
    # touches the portal's device list.
    if [ -n "${GONOMADNET_DEVICE:-}" ]; then
      DESTINATION_ARGS=(-destination "id=$GONOMADNET_DEVICE")
      if [ "$SIGNING" = "signed" ]; then
        SIGN_FLAGS+=(-allowProvisioningDeviceRegistration)
      fi
    fi
    ;;
  simulator)
    CONFIG="Debug"
    ACTION="build"
    # A generic simulator destination is a universal one, and Xcode builds both of its
    # architectures; the Go half is arm64 and nothing else, so the build is stated as the
    # one architecture the simulator on an Apple silicon Mac runs. An Intel Mac cannot run
    # this slice at all, and the script says so rather than failing inside the linker.
    DESTINATION_ARGS=(-destination "generic/platform=iOS Simulator" ARCHS=arm64)
    if [ "$(uname -m)" != "arm64" ]; then
      echo "--simulator needs an Apple silicon Mac: the Go half is built for arm64" >&2
      exit 1
    fi
    ;;
  acceptance)
    CONFIG="Debug"
    ACTION="test"
    DESTINATION_ARGS=(-destination "id=$GONOMADNET_DEVICE" -resultBundlePath "$BUILD_DIR/acceptance.xcresult")
    # A result bundle from an earlier run is refused rather than replaced, and the run that
    # would replace it is the one worth keeping. It is read by the time the run ends.
    rm -rf "$BUILD_DIR/acceptance.xcresult"
    ;;
esac

# An archive path belongs to the action that makes an archive; asking a test run for one
# is asking for something it will not produce.
ARCHIVE_ARGS=(-archivePath "$ARCHIVE_PATH")
if [ "$ACTION" = "test" ]; then
  ARCHIVE_ARGS=()
fi

echo "building the $MODE application"
xcodebuild -project "$PROJECT" -scheme "$SCHEME" \
  -configuration "$CONFIG" \
  -derivedDataPath "$DERIVED" \
  ${ARCHIVE_ARGS[@]+"${ARCHIVE_ARGS[@]}"} \
  GONOMADNET_VERSION="$VERSION_NAME" \
  GONOMADNET_BUILD="$BUILD_NUMBER" \
  GONOMADNET_ARCHIVE="$ARCHIVE" \
  GONOMADNET_ASSETS="$STAGING_DIR/Resources" \
  ${SIGN_ARGS[@]+"${SIGN_ARGS[@]}"} \
  ${SIGN_FLAGS[@]+"${SIGN_FLAGS[@]}"} \
  ${DESTINATION_ARGS[@]+"${DESTINATION_ARGS[@]}"} \
  "$ACTION"

case "$MODE" in
  acceptance)
    echo
    echo "the device acceptance test passed"
    echo "screenshots: xcrun xcresulttool export attachments \\"
    echo "               --path $BUILD_DIR/acceptance.xcresult --output-path <dir>"
    ;;
  release)
    APP="$ARCHIVE_PATH/Products/Applications/Gonomadnet.app"
    IPA="$DIST_DIR/gonomadnet-$VERSION_NAME-ios-arm64.ipa"
    mkdir -p "$DIST_DIR/ipa/Payload"
    rm -rf "$DIST_DIR/ipa/Payload/Gonomadnet.app"
    cp -R "$APP" "$DIST_DIR/ipa/Payload/"
    ( cd "$DIST_DIR/ipa" && zip -qry "$IPA" Payload )
    rm -rf "$DIST_DIR/ipa"

    # The directory the publisher enumerates is a staging area, and it only ever added: every
    # local appliance build left another IPA behind for good, and the publisher re-read and
    # re-skipped all of them on every run. The skipping is deliberate — an appliance built from
    # another release's binaries must not be published as this one's — but the accumulation that
    # makes it necessary is not, so the directory is left holding exactly the appliance this
    # build produced. scripts/build-android-apk.sh does the same for dist/android.
    for stale in "$DIST_DIR"/gonomadnet-*-ios-*.ipa; do
      if [ -e "$stale" ] && [ "$stale" != "$IPA" ]; then
        rm -f "$stale"
        echo "removed the appliance staged for another release: $(basename "$stale")"
      fi
    done

    echo
    echo "IPA:    $IPA"
    echo "sha256: $(shasum -a 256 "$IPA" | cut -d' ' -f1)"
    echo "size:   $(du -h "$IPA" | cut -f1)"
    if [ "$SIGNING" = "unsigned" ]; then
      echo "note:   the app is unsigned: no developer certificate was found in the keychain"
    fi
    ;;
  simulator)
    echo
    echo "app: $DERIVED/Build/Products/Debug-iphonesimulator/Gonomadnet.app"
    ;;
  *)
    echo
    echo "app: $DERIVED/Build/Products/Debug-iphoneos/Gonomadnet.app"
    ;;
esac
