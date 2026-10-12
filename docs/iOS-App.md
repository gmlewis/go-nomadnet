# The gonomadnet iOS appliance

This is the manual for the appliance itself: what it is made of, what it decides, and
where each decision is made and tested. If you want to run it, start with
[docs/iOS.md](iOS.md). If you want to build it, start with
[docs/iOS-Build.md](iOS-Build.md).

## 1. Shape of the thing

The Android appliance is eight programs and a Kotlin application that starts them.
iOS forbids one process from starting another — no `fork`, no `exec`, no
`posix_spawn` — so none of that survives the move. What replaces it is a single
process, and the parts are the same five:

| Part | Android | iOS |
|---|---|---|
| Reticulum transport | `gornsd`, its own process | a Go subsystem in-process |
| RRC hub | `gorrcd`, its own process | a Go subsystem in-process |
| Chat bot | `gorrcbot`, its own process | a Go subsystem in-process |
| Sensor feed | a Kotlin TCP server fed by `gonsensor` | a Go TCP server, fed directly |
| Client terminal | `gonomadnet` on a PTY, hosted by `gorcons` | the client in-process, on a terminal the app built |

The client is the same client. It runs on a `tcell.Tty` the appliance implements rather
than on `/dev/tty`, because an iOS process has no controlling terminal to open, and
everything above that interface — tview, the menus, the browser, the Micron renderer —
is unchanged.

## 2. The halves, and the seam between them

```
ios/
├── bridge/                       the Go half
│   ├── frames.go                 the frame codec (the wire contract)
│   ├── console.go                the terminal the client draws on
│   ├── feed.go                   the sensor feed server
│   ├── engine.go                 the subsystem supervisor
│   ├── subsystems.go             transport, hub and bot as subsystems
│   ├── appliance.go              the process's one host, and the status codes
│   ├── host.go                   the host lifecycle, and no C at all
│   └── bridge.go                 the C boundary: the only file that imports "C"
├── Sources/
│   ├── TerminalKit/              the Swift half's terminal
│   ├── ApplianceKit/             the Swift half's decisions
│   └── GonomadnetBridge/         the C header both halves agree on
└── App/                          the SwiftUI application
```

The seam is the C ABI declared in `Sources/GonomadnetBridge/include/bridge.h` and
implemented by the static archive the build script produces from `./cmd/gonomadnet`.
It is six functions: start, stop, start a console, feed a position, feed a heading,
and ask why the last call failed.

## 3. The console, and why it is not a PTY

On Android the appliance opens a pseudo-terminal, runs the client on it, and bridges
the PTY master to the app over a Unix socket. The client gets a real terminal, and the
app gets a byte stream to parse.

iOS has the socket and the parser but not the child process. So the appliance keeps the
socket and the framing, and replaces the PTY with the terminal itself: `ios/bridge/console.go`
implements `tcell.Tty` over the frame connection. The client's `Read` is fed by the app's
keystrokes, its `Write` becomes DATA frames, and its `WindowSize` follows the app's
RESIZE frames. tcell never reaches for a file descriptor: it uses only that interface.

The wire format is byte for byte the one the Android console host speaks, and the same
golden table of bytes is asserted in Go and in Swift, because the two halves are written
in different languages by different tools and a codec disagreement between them produces
a console that renders nothing and reports no error.

| Frame | Direction | Payload |
|---|---|---|
| `0x01` DATA | both | raw terminal bytes |
| `0x02` RESIZE | app → bridge | columns then rows, big-endian `uint16` |
| `0x03` EXIT | bridge → app | the client's exit status |

A frame is `type(1) | length(4, big-endian) | payload`, and a declared length over
1 MiB is refused before anything is allocated.

## 4. The socket, and its length

The app dials a Unix socket the bridge binds. On Android that socket is in the abstract
namespace and its name is random, so two apps can never collide. Darwin has no abstract
namespace, so the socket has a path — and a path has a limit: the kernel's `sun_path` is
104 bytes, and iOS's own temporary directory is long enough that a careless name reaches
it. The bridge therefore refuses an over-long path with an error that says how long it
was and how long it may be, rather than binding a truncated one that the app would never
match.

## 5. The sensor feed

The feed carries **NMEA-0183 sentences**, not JSON, on `127.0.0.1:37430`.

That is not a style choice. The two readers are the client's own sensor source and the
bot, and both parse NMEA. The appliance converts a sample once, at the point it arrives,
and every subscriber sees the same bytes — which is what makes the client's `[location]`
forms and the bot's `where` answer agree with each other.

**One set of rules decides what is published, and it is `SensorPipeline`'s.** The pipeline
holds the rate limit and the sanity checks; what it accepts is handed to the Go half as a
`SensorReading`, and the Go half emits the NMEA sentences from exactly that. Nothing
downstream re-decides what the pipeline decided or reshapes what it produced — which is why
the pipeline publishes a reading rather than a serialized line. It published a JSON line
once, and the appliance threw it away, converting the same sample a second time from the
raw device callback; the line, the serializer and the serializer's tests are gone rather
than left as a component whose output nobody read.

The port is pinned and loopback-only, and the two Reticulum ports (`37428` for the shared
instance, `37429` for its control channel) are refused: a feed that took one of those
would leave the client talking to a sensor stream where it expected its transport.

Two rules about what is published, both of which exist to stop the appliance lying:

- **A measurement nobody took is absent, not zero.** A height the receiver did not report
  becomes an empty NMEA field. A fix whose speed was not measured carries no speed.
- **A withheld reading publishes nothing.** A heading the device could not measure is
  silence. Zero degrees is due north, and the reader cannot tell the two apart.

A heading is published as one sentence, not two: a reader holds one heading, so a magnetic
sentence and a true one from the same device would leave the reader with only whichever
arrived last. The true heading wins when the device knows it.

## 6. Configuration

The Swift half renders the configuration; the Go half only reads it. That is the same
split as Android's, and it is what lets the templates be asserted against golden files
in Swift tests that need no device.

**Reticulum's `config`** is rendered by `ApplianceKit.NodeConfigRenderer`:

- The shared instance is pinned: TCP, `127.0.0.1:37428`, instance name `default`. The
  transport enables it; the client requires it.
- Values are emitted **unquoted**, because Reticulum's INI parser keeps quotes as part of
  the value.
- A host **name** is refused. It resolves to nothing on a device that may have no DNS at
  all, and a transport that silently never connects is worse than one that says why. An
  IPv6 literal is written without brackets.
- A section with no interfaces says so in a comment rather than being silently blank.

**The hub is resolved before anything is rendered.** The renderer refuses names and the Go
half that reads its output has no resolver, so `HubResolver` turns the configured host into
an address *while it is still a name the system can look up*: a literal address is passed
through untouched, an IPv6 literal has its brackets removed, and a name the resolver cannot
answer leaves the appliance with no hub interface and a reason that names the host. That
last case is not a failure — an appliance with no hub still runs, still serves its own
pages and still dials a radio — and it is what the Android appliance renders in the same
situation. The rendering, the resolution and the radio are one call
(`SettingsEdit.render`), which is what makes the screen a reader types into and the stack
that starts the transport unable to disagree about what an edit means.

**The client's `config`** is not rendered from scratch. It is the client's own default
configuration with exactly three values changed — the node enabled, the node named
`gonomadnet on iOS`, and the editor left **empty** — and every other byte, comment and blank
line preserved. It is asserted as a byte comparison against the template the Go module ships.

The empty editor is the honest answer on a platform with no editor programs: the setting
asks which program to start, and there is no program — the editing happens in the app. A
command written there would be a command that can never run, and the client's Config page
would fail blaming a program that does not exist.

**The channel store** is seeded with three channels: **RNS Community**
(`28c7c1a68c735693aa8e6b8193ed44b2`), **gonomadnet Public Hub**
(`a012129c10205c0b9441fcd2b755b2a7`), and the appliance's own **appliance-hub**. It is
CBOR, and the encoder is asserted byte for byte against the fixtures the Go reader
already loads, which is the only proof that matters: what iOS seeds is what the client
reads.

**Editing.** The appliance's editor is a screen in the app, not a program, and it treats
the two files differently because they have two different owners:

- Reticulum's configuration **is rendered** from the appliance's settings on every start,
  so editing its text by hand would be an edit the next Start Stack discards. The editor
  therefore edits the *values* — the hub's address or name, and its port — and shows the
  exact text they produce, resolution included.
- The client's configuration is the client's own file. The appliance states its three
  values over whatever is there, so every other line a reader writes survives; that is what
  `ClientConfig.applied(to:)` is, and it is also what the stack writes on every start,
  which is what stops the next Start Stack from undoing the editor's work.

Validation is the renderers' own, in both cases; neither screen re-implements a rule.

## 7. The radio, and why the transport is told so little

An iPad cannot open a USB serial adapter, which is how every other platform reaches an
RNode. What it can do is talk to one over Bluetooth Low Energy, which the firmware exposes
as the Nordic UART Service: one characteristic the appliance writes to and one it is
notified on. That is a byte pipe, and the appliance's shape for a byte pipe is the one it
already has for its own console — a socket at one end and the device at the other.

So the transport is not told about Bluetooth at all. It is handed
`tcp://127.0.0.1:37431`, which is the `tcp://` RNode mode it already speaks, and
`RadioManager` serves that address: it listens there and moves every byte between the
transport's connection and the radio's two characteristics.

**The pump is transparent, and that is the whole design.** The RNode protocol — KISS
framing, the detect and firmware-version handshake, the radio configuration commands, and
the data frames — is spoken by the transport on one side and answered by the firmware on
the other. A bridge that framed, split or merged anything would be a second framer in the
middle of a conversation it is not a party to. What it does have to get right is that bytes
survive it exactly: a frame the transport writes arrives at the radio in as many writes as
the characteristic accepts, and the radio's notifications arrive at the transport as the one
stream they are, because a notification boundary is not a frame boundary.

That the reference implementation's TCP mode runs the same protocol is what made this a
reasonable bet, and it was checked rather than assumed: the Python RNS interface writes its
KISS commands through the same BLE connection object it would use for a serial port, so the
framing is above the link on both sides.

The radio's interface is rendered **only when a radio is paired**, because one enabled
against an address nothing is listening on reports itself disconnected for the life of the
install. Pairing and unpairing are therefore a change to the rendered configuration, and
the transport picks it up on the next start.

What cannot be tested without hardware is the Bluetooth half: the scan, the connection, the
characteristic discovery. What *can* be, and is, is everything that decides: the service and
characteristic identifiers, the scan's name rule, the address, and — the part that matters —
the pump's byte transparency, against a fake radio.

## 8. The heading reference

A device's attitude says where the case points. Which way the *reader* means depends on
how the device is held, so the appliance asks:

- **Screen back** measures from the normal to the back of the case. That is the direction
  a device on a stand, in a folio case or in a keyboard case points.
- **Screen top** measures along the top edge of the screen. That is the direction a device
  lying flat on a table points.

`HeadingReference.attitude` takes the rotation vector and the chosen axis and returns a
heading, a pitch, a roll and — this is the important one — whether the reading is
*ill-conditioned*. When the reference axis is within 10° of vertical the compass has no
direction to point in, and the appliance withholds the heading rather than publishing a
wild one. That is why a device lying flat reports no *screen back* heading, and why a
device standing up reports no *screen top* one.

A magnetic heading is corrected to true with the World Magnetic Model's declination at
the device's own position. With no position there is no declination, so the heading stays
magnetic and says so: a magnetic heading reported as true is wrong by up to twenty
degrees, which on a device whose purpose is to say which way to walk is not a rounding
error.

## 9. What is tested, and where

`./scripts/build-ios-app.sh --test-only` runs both halves.

**The Go half** (`go test ./ios/bridge/...`) covers the frame codec against the golden
table, the frame codec's refusals (oversize, unknown type, truncation, malformed length),
the console's delivery of both directions and its release when the app goes away, the
feed's delivery to every subscriber and its refusal to stall on one that stopped reading,
the engine's start and stop order, and the host's console lifecycle — including that an
app disconnecting ends the session and that shutting down releases the socket.

Two of them are worth naming because they are the appliance's real contract: a position
published by the engine is fed to **the client's own reader**, and the fix that comes out
is compared with the fix that went in; and a heading likewise.

**The console's join to a real terminal** is tested too, and it is a test that found two
bugs rather than confirming one. It builds the client's screen exactly the way the host
binary does — the same terminfo lookup, the same constructor — over a console bound to a
real socket, and asserts the four things the join has to do: the terminal's own setup and a
drawn cell arrive at the app as escape sequences, a keystroke sent as a frame arrives as a
key event, a resize frame changes what the client believes its window to be, and the app
going away ends the session without a panic or a leaked goroutine.

- The first bug was `Console.Drain`, which was a no-op. tcell's shutdown closes its stop
  channel, calls `Drain`, and then waits for its input loop to return — but that loop is
  parked inside `Read`, and a parked read never sees a closed channel. The client could
  therefore never finish shutting down. `Drain` now releases the read, which is what the
  interface is for.
- The second was the terminal the appliance claims to be. It was `xterm-256color`, whose
  entry has no direct-color capability, so tcell degraded every color in the appliance's
  own theme to the 256-color cube before the app ever saw it. It is now a direct-color
  entry (`bridge.TerminalName`), and a test asserts both that the entry carries `38;2` and
  that a cell drawn in a color outside the cube arrives at the app as those bytes.

**The Swift half** (`swift test`) covers the frame codec against the same golden table,
the escape-sequence parser and the screen model, the palette (asserted against the
shipped `colors.properties`, so the code and the theme cannot drift), the key and mouse
encodings, the touch gesture, the configuration renderers, the hub resolver, the channel
store's CBOR against the fixtures the Go client reads, the setting edit that the Config
editor and the stack both render through, the radio's byte pump, and the heading maths.

`./run-all-tests.sh` is the Go repositories' own gate and deliberately does not run
`swift test`: it has to keep working on a machine with no Xcode. The bridge's Go tests
are ordinary Go tests and *are* part of it.

## 10. Why the C boundary is one file wide

`ios/bridge/bridge.go` is built only for `GOOS=ios`. It is not shyness about cgo: a file
that imports `"C"` makes the compiler generate Go sources for the package, and every
analyzer that walks the package is then shown code nobody here wrote and nobody here can
keep clean. Confining the boundary to one file keeps the rest of the appliance ordinary
Go, which is what every gate and every `go test` can see. The exported file is verified by
the build that uses it: `scripts/build-ios-app.sh` compiles it for `ios/arm64` and fails if
any of the six entry points is missing from the archive.

## 11. What is not there yet

- **Background operation.** iOS suspends an app that leaves the screen. The stack is
  built to be brought down and up cleanly — which is what `Host.Close` waiting for the
  console to release its socket is for — but the appliance does not claim to run with the
  screen off.
- **Acceptance on a real device.** The app builds, installs and launches in the Simulator,
  and the first launch has been watched writing both Reticulum configurations, the client's
  configuration and the seeded channels without starting anything — but nothing has run on
  an iPad. The parts that need hardware are listed in
  [docs/iOS-Build.md](iOS-Build.md) and in §7 of [docs/iOS.md](iOS.md).

## 12. The hub, the bot and the client, in one process

The engine starts the subsystems it is given, in order, and the appliance's own host wiring
gives it one: the transport. The RRC hub and the bot are written as subsystems
(`ios/bridge/subsystems.go`) and are not in that list, so an appliance started today
announces no hub and answers nothing on one.
