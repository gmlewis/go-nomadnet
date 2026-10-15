# Building the gonomadnet iOS app

The appliance is two halves that are built by two different toolchains and joined by a
static archive: a Go library built for `ios/arm64`, and a Swift package built by Xcode.
`scripts/build-ios-app.sh` does the whole job in one command.

## 1. What you need

- **macOS with Xcode 15 or later** (the appliance is built and tested with Xcode 26 and
  targets iOS 16). The iOS SDK, `xcrun`, and a Swift toolchain come with it.
- **The iOS platform component installed.** Xcode ships the SDK but downloads the platform
  and its simulator runtime separately, and a machine that has only the SDK cannot compile
  an asset catalog (`No simulator runtime version … available to use with iphonesimulator
  SDK …`) and offers no iOS build destination at all. Xcode's **Settings › Components**
  installs it, as does `xcodebuild -downloadPlatform iOS` (about 8 GB).
- **Go 1.26 or later**, on `PATH`.
- **[go-reticulum](https://github.com/gmlewis/go-reticulum) checked out beside this
  repository** as `../go-reticulum`. The appliance's transport, hub and bot are its
  packages; the client is this repository's.
- A **free or paid Apple developer account** for a device build. A simulator build needs
  none, and neither does the test suite.

## 2. The one command

```bash
./scripts/build-ios-app.sh --test-only   # run the Go and Swift tests and stop
./scripts/build-ios-app.sh --debug       # build an installable debug .app
./scripts/build-ios-app.sh --simulator   # build a debug .app for the Simulator
./scripts/build-ios-app.sh --acceptance  # drive the appliance on a device (see §6)
./scripts/build-ios-app.sh               # build a release .ipa in dist/ios/
```

`--test-only` needs no SDK, no signing key and no Xcode project, and it is the gate that
has to pass before any of the rest is worth doing. It runs:

- `swift test` in `ios/`, which is 228 tests over the terminal engine, the configuration
  renderers and resolvers, the channel store, the radio's byte pump, and the heading maths.
- `go test -race ./ios/bridge/...`, which is the frame codec, the console, the sensor feed,
  the appliance engine, and the console's own join to a real tcell screen.

The full repository gate, `./run-all-tests.sh`, does **not** run `swift test`: it is the
Go repositories' gate and has to keep working on a machine with no Xcode. The bridge's Go
tests are ordinary Go tests and are part of it.

## 3. What the build does

1. **Reads the version** from `nomadnet/version/version.go` and computes the bundle
   version from it, so a release's version is written in exactly one place.
2. **Builds the Go half** as a C archive — twice, once per slice:

   ```bash
   GOOS=ios GOARCH=arm64 CGO_ENABLED=1 \
     CC="$(xcrun --sdk iphoneos -f clang)" \
     CGO_CFLAGS="-isysroot $SDK -miphoneos-version-min=16.0 -arch arm64" \
     go build -tags ios -buildmode=c-archive -o ios/build/archive/libgonomadnet.a ./cmd/gonomadnet
   ```

   The simulator slice is the same build against the simulator's SDK:

   ```bash
   GOOS=ios GOARCH=arm64 CGO_ENABLED=1 \
     CC="$(xcrun --sdk iphonesimulator -f clang)" \
     CGO_CFLAGS="-isysroot $SIMSDK -target arm64-apple-ios16.0-simulator -mios-simulator-version-min=16.0" \
     go build -tags ios -buildmode=c-archive -o ios/build/archive/libgonomadnet-simulator.a ./cmd/gonomadnet
   ```

   **Both are `GOOS=ios`**, and that is not a detail: a Go binary built for `darwin`
   installs an alternate signal stack, the Simulator's runtime — like iOS itself — does not
   support one, and the first asynchronous preemption then aborts the process with
   `fatal error: non-Go code disabled sigaltstack`. Go's `ios` port knows about this and
   skips the signal stack; what differs between the two slices is the platform they are
   built for, not the runtime's idea of it.

   It is built from `cmd/gonomadnet` and not from `ios/bridge` because a C archive needs a
   main package, and the NomadNet client is one. Everything else the appliance needs is a
   library package under `ios/`; `cmd/gonomadnet/ios-host.go` is the one file outside
   `ios/`, it is `//go:build ios`, and it exists only because Go forbids importing a main
   package.

   Each archive is then **verified**: the build fails if any of the six entry points is
   missing. A build that quietly dropped one produces an app that launches, shows its
   interface, and does nothing at all, and the linker would not have said a word.
3. **Stages the assets**, rather than committing copies: the Bible text and the terminal
   font come from `android/app/src/main/assets/`, the client's default configuration from
   `nomadnet/config/testdata/`, and the launcher icons from `assets/` into the asset
   catalog's icon set. The catalog itself is composed here — the repository's own
   `Assets.xcassets` supplies the color set the launch screen names, and the icons are
   copied into its `AppIcon.appiconset` by the name the icon generator gave them. A second
   copy of any of them would be a second thing to keep in step.
4. **Builds the app** with `xcodebuild`, and packages the `.ipa`.

## 4. The Xcode project

`ios/App/Gonomadnet.xcodeproj` is what assembles the app bundle. It is committed with the
sources it builds, and it takes four build settings from the script so that it never has
to be edited for a release:

| Setting | What it is |
|---|---|
| `GONOMADNET_VERSION` | the marketing version, from `version.go` |
| `GONOMADNET_BUILD` | the bundle version, computed from it |
| `GONOMADNET_ARCHIVE` | the Go static archive to link |
| `GONOMADNET_ASSETS` | the staged resources directory |

The target has:

- **Sources**: `App/*.swift`, `App/Views/*.swift`, `App/Services/*.swift`, and the two
  Swift packages in `ios/Sources/` as local package dependencies (`TerminalKit` and
  `ApplianceKit`).
- **Bridging header**: `Sources/GonomadnetBridge/include/bridge.h`. It declares the Go
  half's C entry points, which is what makes `ApplianceBridge` in `ios/App/Services/Bridge.swift`
  compile. `Sources/GonomadnetBridge/bridge.c` is compiled into the target as well, so that
  the header is compiled once on its own and a declaration that cannot work is a build
  error rather than a surprise at a call site.
- **Other linker flags**: `$(GONOMADNET_ARCHIVE) -framework CoreLocation -framework
  CoreMotion -framework CoreBluetooth`.
- **Info.plist**: `App/Info.plist`, with `GONOMADNET_VERSION` and `GONOMADNET_BUILD`
  substituted into the two version keys.
- **Resources**: everything in `$(GONOMADNET_ASSETS)` — the font, the Bible text, the color
  theme, the client's default configuration, and `Assets.xcassets`. The resource group's
  path *is* `$(GONOMADNET_ASSETS)`, so a resource added to the staging step is picked up by
  the target without the project being edited.
- **Deployment target**: iOS 16.0.
- **`ENABLE_DEBUG_DYLIB = NO`**, because the Go runtime is linked into the app: a debug
  build that puts the app's code in a separate dylib is not a layout the Go runtime was
  built for.

## 5. Signing

**Nothing has to be set, and nothing is stored in the repository.** The script reads the
team identifier from the developer certificate in your login keychain — Apple puts the team
in the certificate's own subject — and signs with it:

```bash
./scripts/build-ios-app.sh            # a signed release .ipa
./scripts/build-ios-app.sh --debug    # a signed .app
```

A team identifier is not a secret: it is inside every app that team has signed and every
provisioning profile it has. What is secret is the private key that goes with the certificate,
and that never leaves the keychain — the script reads the certificate's public subject and
nothing else. It looks for an `Apple Distribution` certificate first and an `Apple Development`
one if there is none; which of the two signs a build matters to who can install the result, so
a machine with both should say which it means with `GONOMADNET_TEAM`.

`GONOMADNET_TEAM=<team>` overrides the certificate, which is what a machine with more than
one team needs. On a machine with **no** certificate — a build server — the build still runs
and still verifies the resources, the C archive and the link line, and the `.ipa` it leaves is
a valid package that has to be signed before it will install, which the script says when it
packages it.

A **simulator** build is never signed and needs no certificate.

**A build for one device needs the device named as well.** Signing does not register a device;
`GONOMADNET_DEVICE` does, by naming it as the build's destination. A profile that does not list
the device installs nothing, and iOS says so only after the build is green:

```bash
GONOMADNET_DEVICE=<udid> ./scripts/build-ios-app.sh --debug
```

Find the UDID with `xcrun devicectl list devices`; `id=<udid>` or `id=<identifier>` both work
as destinations. Registering a device is a change to the developer account rather than to the
project, so a generic or simulator build never asks for it.

**A release build needs neither.** It is built for `generic/platform=iOS`, so no device has
to be attached to build or sign one, and `./scripts/build-ios-app.sh` signs it with whatever
certificate the machine has.

## 6. Running it

On a **device**, install with Xcode's own Run, or with `xcrun devicectl`:

```bash
xcrun devicectl device install app --device <UDID> \
  ios/build/derived/Build/Products/Debug-iphoneos/Gonomadnet.app
```

In the **Simulator**:

```bash
xcrun simctl boot "iPad Pro 13-inch (M5)"
xcrun simctl install booted ios/build/derived/Build/Products/Debug-iphonesimulator/Gonomadnet.app
xcrun simctl launch --console booted com.gmlewis.gonomadnet
```

The appliance needs location, motion and Bluetooth permission the first time each is
used; each is asked for when the reader starts the thing that needs it, never at launch.

To have the appliance drive itself instead of a reader doing it, run the acceptance test on
the device:

```bash
GONOMADNET_TEAM=ABCDE12345 GONOMADNET_DEVICE=<udid> ./scripts/build-ios-app.sh --acceptance
```

It installs both the appliance and a test runner, presses the buttons, and asserts what a
reader would check — see [docs/iOS-App.md](iOS-App.md) §10. The first launch starts nothing;
Start Stack brings up the shared instance and the sensor feed, and Stop Stack releases them
and they come back; the console renders, takes a keystroke, and scrolls the panel a drag is
over; rotating reflows it; Start Sensors turns the sensor path on; and a dialog's `< Scan >`
button reaches the appliance's scanner, which is the one part of that feature a machine with
no camera cannot test. It leaves its screenshots and its measurements in a result bundle under
`ios/build/acceptance.xcresult`, which `xcrun xcresulttool export attachments --path …
--output-path <dir>` unpacks.

While it runs, the appliance shows iOS's own *Automation Running* banner on the device. That
is the test driving the app, not the appliance: it goes away when the run ends.

The screenshots come out in the device's **native** orientation with the interface drawn
rotated, so a reader looking at one turns it 90° counter-clockwise and crops the black
remainder.

## 7. Testing the halves on their own

```bash
# The Swift half, with the toolchain Xcode builds the app with.
cd ios && xcrun swift test --scratch-path build/swift

# The Go half, with the race detector.
go test -race ./ios/bridge/...

# The app's own sources, type-checked against the iOS SDK without an app bundle.
xcrun swift build --triple arm64-apple-ios16.0 \
  --sdk "$(xcrun --sdk iphoneos --show-sdk-path)" --scratch-path build/swift-ios
xcrun --sdk iphoneos swiftc -typecheck -target arm64-apple-ios16.0 \
  -sdk "$(xcrun --sdk iphoneos --show-sdk-path)" \
  -I ios/build/swift-ios/arm64-apple-ios/debug/Modules \
  -import-objc-header ios/Sources/GonomadnetBridge/include/bridge.h \
  ios/App/*.swift ios/App/Views/*.swift ios/App/Services/*.swift
```

**Give each toolchain its own scratch path.** A module built by one Swift compiler cannot
be imported by another, and a machine with a second toolchain on `PATH` (a swift.org
toolchain, a beta, `swiftly`) produces `module compiled with Swift X cannot be imported by
the Swift Y compiler` — which reads like a broken package and is a shared cache directory.
`build/swift` belongs to the script, `build/swift-ios` to the iOS-triple build above, and
`.build` to a bare `swift test` run with whatever is on `PATH`.

**The device acceptance test is not one of these.** It is a UI test — `AcceptanceTests` in
`ios/App/AcceptanceTests/` — and it needs an installed appliance on real hardware, so it
runs only through `--acceptance` and never on the machine that builds. What it covers is in
[docs/iOS-App.md](iOS-App.md) §10.

## 8. Troubleshooting

**`-buildmode=c-archive requires exactly one main package`.** You are building the wrong
package. It is `./cmd/gonomadnet`, not `./ios/bridge`.

**The archive builds but the app cannot find `GonomadnetStart`.** The bridging header is
not set, or the archive is not in the linker flags. The build script checks the archive's
symbols for this reason.

**`no such module 'TerminalKit'`.** The app target does not have the local packages added
as dependencies, or the build ran before they were built.

**`No simulator runtime version … available to use with iphonesimulator SDK …`.** The iOS
platform component is not installed, so there is no runtime for `actool` to compile the
icon set against. See §1.

**`Found no destinations for the scheme` / `iOS … is not installed`.** The same missing
platform component. `xcodebuild -showdestinations` should list `Any iOS Device` and the
installed simulators; if it lists only an ineligible placeholder, nothing can be built.

**The app launches and dies immediately with a Go stack in the crash report.** Check which
`GOOS` the archive was built with: a `darwin` build in the Simulator aborts with
`fatal error: non-Go code disabled sigaltstack`, and the fix is the `GOOS=ios` simulator
slice §3 describes.

**A test that binds a socket fails with `invalid argument`.** The path is longer than the
kernel's `sun_path`. On macOS this is usually a path under `/var/folders/…` — use `/tmp`,
which is what the appliance's own tests do.
