# The gonomadnet iOS appliance

What the appliance is made of, what it decides, and where each decision lives. Read
[docs/iOS.md](iOS.md) to run it, [docs/iOS-Build.md](iOS-Build.md) to build it.

## 1. Shape

Android runs eight programs and a Kotlin application that starts them. iOS allows one
process to start no other, so the appliance is a single process: the same subsystems run as
Go goroutines inside it, and the client draws into a terminal the app itself built.

| Part | Android | iOS |
|---|---|---|
| Reticulum transport | `gornsd`, its own process | a Go subsystem in-process |
| RRC hub | `gorrcd`, its own process | a Go subsystem in-process |
| Chat bot | `gorrcbot`, its own process | a Go subsystem in-process |
| Sensor feed | a Kotlin TCP server fed by `gonsensor` | a Go TCP server, fed directly |
| Client terminal | `gonomadnet` on a PTY, hosted by `gorcons` | the client in-process, on a terminal the app built |

The client is the same client: it runs on a `tcell.Tty` the appliance implements rather than
on `/dev/tty`, because an iOS process has no controlling terminal to open. Everything above
that interface — tview, the menus, the browser, the Micron renderer — is unchanged.

## 2. The halves, and the seam between them

```
ios/
├── bridge/                       the Go half
│   ├── frames.go                 the frame codec (the wire contract)
│   ├── console.go                the terminal the client draws on
│   ├── feed.go                   the sensor feed server
│   ├── engine.go                 the subsystem supervisor
│   ├── subsystems.go             transport, hub and bot as subsystems
│   ├── stack.go                  which directories they read, and their order
│   ├── appliance.go              the process's one host, and the status codes
│   ├── host.go                   the host lifecycle, and no C at all
│   └── bridge.go                 the C boundary: the only file that imports "C"
├── Sources/
│   ├── TerminalKit/              the Swift half's terminal
│   ├── ApplianceKit/             the Swift half's decisions
│   └── GonomadnetBridge/         the C header both halves agree on
└── App/                          the SwiftUI application
```

The seam is six C functions, declared in `Sources/GonomadnetBridge/include/bridge.h` and
implemented by the static archive the build script produces from `./cmd/gonomadnet`: start,
stop, start a console, feed a position, feed a heading, and ask why the last call failed.

## 3. The console

`ios/bridge/console.go` implements `tcell.Tty` over the frame connection: the client's
`Read` is fed by the app's keystrokes, its `Write` becomes DATA frames, and its
`WindowSize` follows the app's RESIZE frames. No file descriptor is involved.

| Frame | Direction | Payload |
|---|---|---|
| `0x01` DATA | both | raw terminal bytes |
| `0x02` RESIZE | app → bridge | columns, rows, then the width and height in pixels, big-endian `uint16` |
| `0x03` EXIT | bridge → app | the client's exit status |

A frame is `type(1) | length(4, big-endian) | payload`; a declared length over 1 MiB is
refused before anything is allocated. The wire format is byte for byte the one the Android
console host speaks, and the same golden table of bytes is asserted in Go and in Swift.

A window size is cells and pixels because the two answer different questions: the cells are
how much text fits and the pixels are how much room there is. The console's font draws a cell
about twice as tall as it is wide, so a console standing up holds more columns than rows — on
a 1200-by-1920 display in portrait, 100 columns of 12 pixels and 75 rows of 24 — and the
client's own interface turns on which way up it is: in portrait it hides the side panels of
its Conversations, Network, Channels and Guide screens, and restores them when the console is
lying down again. The pixels are the *display's* size, not the console rectangle's: the
on-screen keyboard takes half of a portrait screen's height, which would leave a rectangle
wider than it is tall while the device is standing up. Zero pixels mean the app has measured
no physical size yet, which the client reads as "no physical size to report" and falls back to
its cells.

Three rules the app side has to keep:

- **The client's size is what the client was told, not what the app is drawing.** Compare a
  new grid against the size the last RESIZE frame carried (`TerminalSizing`,
  `TerminalController.told`). The screen model is resized the moment the layout changes, so
  comparing against it always says "no change" and the client draws for its start-up size
  forever. The physical size is part of that size, so a console the app has just measured is
  a telling even when the cells are unchanged.
- **A view reports the grid it can show.** `TerminalView.layoutSubviews` recomputes it and
  calls back; SwiftUI's `updateUIView` runs when a representable's inputs change, which a
  window being laid out is not.
- **Core Text's y axis is the other way up.** UIKit's context has y growing downward and a
  glyph outline has y growing upward, so a line drawn with an identity text matrix is
  mirrored. Negate the text matrix's y, and place the baseline one ascent below the cell's
  top.

The **Console** row on the controls shows the size the client was told (`Open · 69×35`),
which is the first thing to look at when the client's layouts look wrong for its window.

## 4. The socket, and its length

The app dials a Unix socket the bridge binds. On Android that socket is in the abstract
namespace and its name is random; Darwin has no abstract namespace, so it has a path — and
`sun_path` holds 104 bytes. The bridge refuses an over-long path with an error naming both
lengths rather than binding a truncated one, and removes a socket left behind by a killed
process before it binds.

**The socket lives in the app's own temporary directory, not with the appliance's files.**
An iOS data container is 76 bytes before anything is in it, and `Library/Application
Support/gonomadnet` adds 37: a socket there is 121 bytes before its own name, past the
kernel's limit, so the console could never open at all. `ConsoleSocket` is the one place
that decides, and `ConsoleSocketTests` asserts both that a container path fits and that the
configuration directory does not.

## 5. The sensor feed

**NMEA-0183 sentences**, not JSON, on `127.0.0.1:37430`. The two readers are the client's
own sensor source and the bot, and both parse NMEA: the appliance converts a sample once,
and every subscriber sees the same bytes.

`SensorPipeline` decides what is published — the rate limit and the sanity checks are its —
and what it accepts is handed to the Go half as a `SensorReading`, which emits the sentences.
Nothing downstream re-decides or reshapes it.

The port is pinned and loopback-only, and Reticulum's own loopback ports (`37428` for the
shared instance, `37429` for its control channel) are refused.

Two rules about what is published:

- **A measurement nobody took is absent, not zero.** A height the receiver did not report is
  an empty NMEA field; a fix whose speed was not measured carries no speed.
- **A withheld reading publishes nothing.** A heading the device could not measure is
  silence, because zero degrees is due north and a reader cannot tell the two apart.

A heading is published as one sentence, not two: a reader holds one heading, and the true
one — corrected for declination when the device has a position — wins.

## 6. Configuration

The Swift half renders the configuration; the Go half only reads it. The templates are
asserted against golden files in Swift tests that need no device.

**Reticulum's `config`** (`ApplianceKit.NodeConfigRenderer`):

- The shared instance is pinned: TCP, `127.0.0.1:37428`, instance name `default`. The
  transport enables it, the client requires it.
- Values are written **unquoted**: Reticulum's INI parser keeps quotes as part of the value.
- A host **name** is refused, and an IPv6 literal is written without brackets. A device may
  have no DNS, and a transport that silently never connects is worse than one that says why.
- A section with no interfaces says so in a comment rather than being silently blank.

`HubResolver` turns a configured host name into an address while the system can still look
it up, because the renderer refuses names and the Go half has no resolver. A name that does
not resolve leaves the appliance with no hub interface and a reason naming the host — not a
failure: it still runs, serves its own pages and dials a radio.

**The client's `config`** is the client's own default with exactly three values changed — the
node enabled, the node named `gonomadnet on iOS`, the editor **empty** — and every other
byte preserved. An empty editor is the honest setting on a platform with no editor programs:
the appliance's editor is a screen in the app, and a command named there could never run.

**The channel store** is seeded with **RNS Community** (`28c7c1a6…`), the **gonomadnet
Public Hub** (`a012129c…`) and the appliance's own **appliance-hub**, which is added once the
hub has published an address. It is CBOR, asserted against the fixtures the client itself
loads.

**Editing.** The two files have two owners, and the editor treats them differently:

- Reticulum's configuration is **rendered** from the settings on every start, so the editor
  edits the *values* — the hub's address or name and its port — and shows the text they
  produce, resolution included.
- The client's configuration is the client's own file. The appliance states its three values
  over whatever is there, so every other line a reader writes survives.

Validation is the renderers' own in both cases; no screen re-implements a rule.

## 7. The radio

An iPad cannot open a USB serial adapter. What it can do is talk to an RNode over Bluetooth
Low Energy, where the firmware exposes the Nordic UART Service — one characteristic to write
and one to be notified on. That is a byte pipe, so the transport is not told about Bluetooth
at all: it is handed `tcp://127.0.0.1:37431`, the `tcp://` RNode mode it already speaks, and
`RadioManager` serves that address, moving bytes between the socket and the two
characteristics.

**The pump is transparent.** KISS framing, the detect and firmware-version handshake, the
radio configuration commands and the data frames are spoken by the transport on one side and
answered by the firmware on the other; a bridge that framed, split or merged anything would
be a second framer in a conversation it is not a party to. What it must get right is that
bytes survive exactly: a frame the transport writes reaches the radio in as many writes as
the characteristic accepts, and the radio's notifications reach the transport as the one
stream they are, because a notification boundary is not a frame boundary.

The radio's interface is rendered **only when a radio is paired** — one enabled against an
address nothing is listening on reports itself disconnected for the life of the install — so
pairing and unpairing change the rendered configuration and the transport picks it up on the
next start.

The Bluetooth half — the scan, the connection, the characteristic discovery — needs hardware.
Everything that decides is tested against a fake radio: the identifiers, the scan's name
rule, the address, and the pump's byte transparency.

## 8. The heading reference

A device's attitude says where the case points; which way the *reader* means depends on how
it is held, so the appliance asks:

- **Screen back** measures from the normal to the back of the case — a device on a stand, in
  a folio case or in a keyboard case.
- **Screen top** measures along the top edge of the screen — a device lying flat on a table.

`HeadingReference.attitude` returns a heading, a pitch, a roll and whether the reading is
*ill-conditioned*. Within 10° of vertical the compass has no direction to point in, and the
heading is withheld rather than published wild: that is why a device lying flat reports no
*screen back* heading, and a device standing up no *screen top* one.

A magnetic heading is corrected to true with the World Magnetic Model's declination at the
device's own position. With no position there is no declination, so the heading stays
magnetic and says so — the two differ by up to twenty degrees.

## 9. What is tested, and where

`./scripts/build-ios-app.sh --test-only` runs both halves: `swift test` in `ios/` (228 tests)
and `go test -race ./ios/bridge/...` (66 tests). `./run-all-tests.sh` is the Go repositories'
own gate and deliberately does not run `swift test`, so it keeps working on a machine with no
Xcode; the bridge's Go tests are ordinary Go tests and are part of it.

The **Go half** covers the frame codec against the golden table and its refusals, the
console's delivery in both directions and its release when the app goes away, the feed's
delivery to every subscriber and its refusal to stall on one that stopped reading, the
engine's start and stop order, the directories and order of the appliance's own subsystems,
and the host's console lifecycle. Two of them are the appliance's real contract: a position
published by the engine is fed to **the client's own reader** and compared with what went in,
and the console is joined to a **real tcell screen** built the way the host binary builds it
— asserting that a drawn cell arrives as escape sequences, that a keystroke arrives as a key
event, that a resize frame changes what the client believes its window to be, and that the
app going away ends the session without a panic or a leaked goroutine.

The **Swift half** covers the frame codec against the same golden table, the escape-sequence
parser and the screen model, the palette (asserted against the shipped `colors.properties`),
the key and mouse encodings, the touch gesture and where the console's socket is bound, what
the client's own startup asks the console for, the configuration renderers — including the
bot's, with and without a local hub to dial — the hub resolver, the channel store's CBOR
against the fixtures the Go client reads, the setting edit the Config editor and the stack
both render through, the radio's byte pump, and the heading maths.

`--acceptance` runs the appliance on a device, which is the only test that needs one: the
first launch starts nothing, Start Stack brings up the shared instance and the sensor feed,
Stop Stack releases them and they come back, the console renders and takes a keystroke, a
drag scrolls the panel it is over, rotating reflows the console, and Start Sensors turns the
sensor path on. See [docs/iOS-Build.md](iOS-Build.md) §6.

## 10. Why the C boundary is one file wide

`ios/bridge/bridge.go` is built only for `GOOS=ios`. A file that imports `"C"` makes the
compiler generate Go sources for the package, and every analyzer that walks the package is
then shown code nobody here wrote: confining the boundary to one file keeps the rest of the
appliance ordinary Go, which every gate and every `go test` can see. The exported file is
verified by the build that uses it — `scripts/build-ios-app.sh` compiles it for `ios/arm64`
and fails if any of the six entry points is missing from the archive.

## 11. The stack, in one process

The engine starts three subsystems in order, and the order is a dependency: the transport
owns the shared instance the other two attach to; the hub publishes its own address as it
comes up, which is how the bot and the client find it; the bot is last because it dials hubs
and the local one is one of them.

`ApplianceDirs` points the hub and the bot at the client's own Reticulum directory — **not**
the transport's, where a second user would try to own an instance that is already owned — and
puts both of their homes under the client's configuration directory. What each of them *says*
is decided elsewhere: the hub makes its own identity, because it has no bootstrap run to make
it in, and the bot's TOML is rendered by the Swift half.

Two rules that exist so the appliance does not promise more than it delivers:

- **A new install's first start names no local hub to the bot or to the client's channel
  list.** The hub's address comes from an identity it creates as it starts, so the rendering
  that runs before it has no address to give. The next start has one.
- **Every path the bot uses is stated, including its identity.** A path a configuration file
  leaves unsaid takes the bot's own default, which is under the *operator's* home directory —
  on iOS the container root, a directory the app may not write to. The identity is the one
  path a configuration file may still overrule the home directory with, so both are named.

## 12. What is not there yet

- **Background operation.** iOS suspends an app that leaves the screen. The stack is built to
  be brought down and up cleanly, but the appliance does not claim to run with the screen off.
- **A radio, a fix and a heading on real hardware.** The Bluetooth half of the radio and a
  real receiver need hardware; §8 of [docs/iOS.md](iOS.md) says which of those have been
  exercised and which have not.
