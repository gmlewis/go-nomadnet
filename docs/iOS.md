# Running gonomadnet on iOS and iPadOS

gonomadnet on an iPad is a self-contained Reticulum appliance: a transport node, an
RRC chat hub, the gobot field assistant, a live position and compass feed, and the
full Nomad Network client in a terminal pane — all inside one app, with no server
anywhere else and nothing to install beside it.

The Android appliance achieves that by shipping eight small programs and starting
them. iOS does not allow a program to start another program at all, so the appliance
is one process: the same subsystems run as Go goroutines inside it, the client draws
into a terminal the app itself built, and the Swift half talks to the Go half across a
C interface. What a reader sees is much the same either way. What is different is
below.

## 1. Why an iPad, and what it is good at

An iPad makes a better Reticulum appliance than a phone and, in a few ways, a better
one than a laptop:

- **A screen you can leave on.** The appliance is meant to stay in the room: the
  transport stays connected and the hub stays announced while the console sits open.
- **A keyboard when you want one.** A Magic Keyboard or any Bluetooth keyboard gives
  the terminal 100% of the screen rows and real modifier keys. The on-screen key strip
  is what fills that gap when there is no keyboard, and it gets out of the way when
  there is one.
- **A position and a compass you can trust.** A cellular iPad has a real GNSS receiver
  and a fused attitude sensor. That is what makes the location forms in a Micron page
  and the gobot's `where` answer meaningful, and it is what turns the appliance into
  something worth carrying.
- **A radio you can add without a cable.** See §5.

## 2. First launch

The first launch writes the configuration the appliance needs and starts nothing:

- Reticulum's own `config`, with the shared instance pinned on `127.0.0.1:37428` and
  the transport enabled, under the app's own container.
- The client's `config`, which is the NomadNet default configuration with three values
  changed: the node is enabled, the node is named, and the editor is left empty because
  iOS has no editor program to name.
- The channel store, seeded with three channels: **RNS Community**, **gonomadnet
  Public Hub**, and the appliance's own local hub.

The hub is a host name by default, and it is resolved to an address as the configuration
is written: the programs that read the file cannot resolve one themselves.

Nothing is written outside the app's container, and nothing is started until you press
**Start Stack**.

## 3. Start the stack

Open the app and press **Start Stack** in the sidebar. What it brings up today is:

1. **The sensor feed**, on `127.0.0.1:37430`. It is up first because the client and the
   bot both read it, and a bot that opens a feed that is not there yet waits forever.
2. **The Reticulum transport**, which owns the shared instance everything else attaches
   to.

The status line says whether the transport is running. **Stop Stack** takes it down.

The RRC hub and the bot are written for this platform — they are `ios/bridge/subsystems.go`,
and the renderers that produce their configuration are tested — but they are not in the
list the appliance starts, so an installed appliance is a transport and a client rather
than a hub. That is the first thing to finish, and it is stated here rather than in a
changelog because "the appliance hosts a hub" is a promise a reader would otherwise only
find out about by trying.

## 4. The console

**Open gonomadnet** attaches the NomadNet client to a terminal and runs it. It is the
same client as on a desktop, down to the keystrokes: the arrow keys, the mouse, the
menu, the conversations, the browser and the guide.

- **Tap** anywhere in the console to click there. Links, menu entries and buttons all
  respond to a tap.
- **Drag** to scroll, which the terminal reports to the client as mouse wheel notches,
  one per row.
- **The key strip** sits above the on-screen keyboard and carries the keys touch
  keyboards lack: `ESC`, `/`, `-`, `HOME`, `↑`, `END`, `PGUP` on the first row, and
  `TAB`, `Ctrl`, `Alt`, `←`, `↓`, `→`, `PGDN` on the second. `Ctrl` and `Alt` latch:
  tap one and the next key you type is modified by it. Tap it twice to clear it.
- **A hardware keyboard** hides the strip and gives the client every row of the screen.

The console is what the appliance is for, and it is a real one: the client renders the
same frames it renders anywhere, and the app parses and draws them.

## 5. The radio

An iPad cannot open a USB serial adapter. What it can do is talk to an RNode over
Bluetooth Low Energy, which every current off-grid board supports — a Heltec LoRa32
v3, a LilyGO T-Beam, a T-Echo.

Press **Scan for a radio** in the Radio section, pair with the one you want, and the
transport is given it. The appliance serves the radio on a loopback address the transport
dials, so the transport is told no more about it than where to find it: the same protocol
as a network-attached RNode, and no Bluetooth anywhere in the transport.

- The radio must be switched on, in range, and not connected to another device. An RNode
  that is connected to a phone is not advertising.
- Pairing and unpairing change the configuration the transport is started from, so the
  radio is dialled on the **next start of the stack**. Stop the stack and start it again
  after pairing.
- **Unpair** releases it, and the interface it was rendered as with it.

A network-attached RNode works too, as a TCP or UDP KISS gateway on the same Wi-Fi or
hotspot: add it to the Reticulum configuration as a `TCPClientInterface` or a
`UDPInterface` pointed at the gateway.

## 6. Sensors

**Start Sensors** turns on the position and heading feed. It is off until it is asked
for, and everything about it is local:

- The position never leaves the device except where you send it. It is published to a
  loopback socket that only the two readers on the device can reach.
- The **heading axis** setting says which way the device measures the world. *Screen
  back* is for a device standing up — on a stand, in a keyboard case. *Screen top* is for
  one lying flat on a table. The appliance withholds the heading when the axis you chose
  points at the sky, because a compass has no answer then and a wild guess is worse than
  nothing.
- A heading is published as magnetic unless the appliance has a position, in which case
  the true heading is worked out from it and published as true. A magnetic heading is
  never reported as a true one: they differ by up to twenty degrees.
- **Publish sensors** is your standing answer: with it off, the appliance publishes no
  position and no heading at all. **Run the stack** is the same for the transport, hub and
  bot. Both are remembered, and both are honored when the app comes back from the
  background.

## 7. Configuration

Every configuration file is written on the first launch — Reticulum's, the client's, and
the seeded channels — and nothing is started. Nothing is written outside the app's own
container: there is no `~/.reticulum` on iOS to write to, and the appliance never tries.

**Edit Configuration** in the sidebar is where you change it, and it handles the two files
differently because they have two different owners:

- **Reticulum's configuration is rendered** from the appliance's settings every time the
  stack starts, so editing its text by hand would be work the next start discards. That
  pane edits the values — the hub's address or name, and its port — and shows you the exact
  file they produce.
- **The client's configuration is edited as text**, and everything you write in it except
  the three values the appliance states survives the next start.

A hub named by a **host name** is resolved here, while the appliance can still ask the
system, and the address it resolves to is what the transport is given: the program that
reads the file has no resolver, so a name written into it would be dialled as a name and
never connect. A name that resolves to nothing leaves the appliance with no hub to dial,
and says so rather than leaving you to find out.

Changing the hub or pairing a radio means the transport has to be started again from the
new configuration: stop and start the stack, or press **Restart Stack**.

## 8. What is not there yet

- **Background operation.** iOS suspends an app when it leaves the screen. The stack
  keeps running while the appliance is in the foreground; a transport that must stay up
  with the screen off is not something an iOS app is allowed to promise, and the
  appliance does not pretend otherwise.
- **Acceptance on a real device.** The app builds, installs and launches in the Simulator,
  and its first launch has been watched writing every configuration file without starting
  anything. The parts below have not been run on an iPad or an iPhone, because they need
  hardware the Simulator does not have:
  1. **Pairs with a radio.** A real RNode is reachable over Bluetooth, and the transport
     reports the interface up and a peer reachable over LoRa after the stack is restarted.
  2. **A fix and a heading reach the readers.** With a real GNSS receiver and motion
     sensor, the client's location card and the bot's `where` answer from the device's own
     position, and the heading is withheld when the chosen axis points at the sky.
  3. **The console on a touch screen.** Tapping activates a link, dragging scrolls, and
     rotating the device reflows the client for the new grid.
  4. **The stack comes down cleanly.** Stopping it releases the loopback ports, and the
     app relaunches without a stale console socket.

The rest of the appliance — the console, the stack, the sensors' math and the
configuration — is exercised by the Simulator and by unit tests; see
[docs/iOS-App.md](iOS-App.md) §9 for what is covered and where.

## 9. Troubleshooting

**"no shared instance is running" in the console.** The client attached before the
transport was up, or the transport did not start. Press **Stop Stack**, then **Start
Stack**, and check the status line before opening the console again.

**The console is empty but the stack is green.** The client draws into the terminal the
app built, and the app parses what it draws. A console that is silent and reports no
error means one of the two halves stopped speaking the frame protocol; the status line
says whether the session ended, and the log in the app's container says why.

**The heading is missing.** That is the appliance being honest. Stand the device up if
you chose *screen back*, or lay it flat if you chose *screen top*, or change the setting.

**The position is missing.** iOS needs location permission, and a device indoors needs
a moment and a view of the sky. The permission is asked for when you press **Start
Sensors**, never at launch.

**The radio does not appear.** It must be switched on, in range, and not already
connected to another device. An RNode that is connected to a phone is not advertising.

**The radio is paired and the transport still says the interface is down.** The interface
is rendered when the stack starts, so stop it and start it again. The log line at start-up
says which address the transport was given.

**The client's Config page says it cannot start an editor.** It is telling the truth: an
iOS application may not start a program, so there is no editor for it to run. Use **Edit
Configuration** in the app's sidebar — it edits the same file.

