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
#   ./scripts/build-ios-app.sh --help       show this text
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
      sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    --debug) MODE="debug" ;;
    --simulator) MODE="simulator" ;;
    --test-only) MODE="test" ;;
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

# verify_archive fails the build if any of the six entry points is missing: a build that
# quietly dropped one produces an app that launches, shows its interface, and does
# nothing at all, and the linker would not have said a word.
verify_archive() {
  local archive="$1" symbol
  for symbol in GonomadnetStart GonomadnetStop GonomadnetStartConsole \
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
  echo "ios/App/Gonomadnet.xcodeproj is missing: run ./scripts/build-ios-app.sh --test-only" >&2
  exit 1
fi

SCHEME="Gonomadnet"
DERIVED="$BUILD_DIR/derived"
ARCHIVE_PATH="$BUILD_DIR/Gonomadnet.xcarchive"

# A device build is signed and a simulator build is not. With no team in the
# environment the device build is still made, unsigned, so that the resources, the C
# archive and the link line are all verified on a machine that has no certificate —
# the .ipa it produces is a valid package that has to be signed before it will install.
SIGN_ARGS=()
SIGNING="not-applicable"
if [ "$MODE" != "simulator" ]; then
  if [ -n "${GONOMADNET_TEAM:-}" ]; then
    SIGN_ARGS=(DEVELOPMENT_TEAM="$GONOMADNET_TEAM" CODE_SIGN_STYLE=Automatic)
    SIGNING="signed"
  else
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
esac

echo "building the $MODE application"
xcodebuild -project "$PROJECT" -scheme "$SCHEME" \
  -configuration "$CONFIG" \
  -derivedDataPath "$DERIVED" \
  -archivePath "$ARCHIVE_PATH" \
  GONOMADNET_VERSION="$VERSION_NAME" \
  GONOMADNET_BUILD="$BUILD_NUMBER" \
  GONOMADNET_ARCHIVE="$ARCHIVE" \
  GONOMADNET_ASSETS="$STAGING_DIR/Resources" \
  ${SIGN_ARGS[@]+"${SIGN_ARGS[@]}"} \
  ${DESTINATION_ARGS[@]+"${DESTINATION_ARGS[@]}"} \
  "$ACTION"

case "$MODE" in
  release)
    APP="$ARCHIVE_PATH/Products/Applications/Gonomadnet.app"
    IPA="$DIST_DIR/gonomadnet-$VERSION_NAME-ios-arm64.ipa"
    mkdir -p "$DIST_DIR/ipa/Payload"
    rm -rf "$DIST_DIR/ipa/Payload/Gonomadnet.app"
    cp -R "$APP" "$DIST_DIR/ipa/Payload/"
    ( cd "$DIST_DIR/ipa" && zip -qry "$IPA" Payload )
    rm -rf "$DIST_DIR/ipa"

    echo
    echo "IPA:    $IPA"
    echo "sha256: $(shasum -a 256 "$IPA" | cut -d' ' -f1)"
    echo "size:   $(du -h "$IPA" | cut -f1)"
    if [ "$SIGNING" = "unsigned" ]; then
      echo "note:   the app is unsigned; set GONOMADNET_TEAM to sign it"
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
