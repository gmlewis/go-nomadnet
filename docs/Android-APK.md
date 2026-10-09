# gonomadnet node — the Android app

**gonomadnet node** turns an Android tablet into a self-contained Reticulum node.
The app runs the Reticulum transport, an RRC hub and a chat bot under its own
uid, and publishes the tablet's GNSS fix and compass heading as a sensor feed.
The `gonomadnet` client can then use the tablet's real position, and the bot
answers position and heading commands with it.

You get two things from one install:

- a **node** that runs on the tablet with no computer and no server, and
- **real sensors**, so position commands answer with where the tablet actually is.

Nothing about your position leaves the device unless you ask for it — see
[Privacy](#privacy).

If you only want the terminal client and do not need the tablet's sensors, you do
not need this app at all. See [`docs/Android.md`](Android.md) for the Termux-only
client, and go-reticulum's
[Android guide](https://github.com/gmlewis/go-reticulum/blob/master/docs/guides/android.md)
for the platform-level details.

---

## Getting started

Everything on this page happens **on the device**. No computer, and no Android
development tools, are needed — though `adb` is mentioned as an alternative for
anyone who has them.

You need two things: **this app**, and **Termux**. Both are one download each, and
the app installs Termux for you.

### 1. Install the app

Download `gonomadnet-<version>-android-arm64-v8a.apk` from the
[latest release](https://github.com/gmlewis/go-nomadnet/releases) and open it from
your notifications or your Downloads. Android will refuse the first time and offer
a settings link — allow installing from the app you opened it with (**Chrome**,
**Files**, or your browser), then tap **Install** again. The app appears as
**gonomadnet node**.

With a computer: `adb install -r gonomadnet-<version>-android-arm64-v8a.apk`.

Grant the location permission it asks for. It uses the permission only to read the
device's own GNSS receiver — see [Privacy](#privacy).

### 2. Let the app install Termux

Open **gonomadnet node** and tap **Install Termux**. The app downloads the build
Termux's own project publishes, checks its signing key, and hands it to Android's
installer for you to confirm.

You want this and not the Play Store: Google's build of Termux **cannot run
programs it installs**, and the failure it produces names a permission that has
nothing to do with the problem.

Android will ask you to allow **install unknown apps** for this app the first
time. The app opens that screen for you; the toggle is the only thing on it.

When the install finishes, the button's own label says which build you now have.

### 3. Tap "Publish files for Termux"

The app writes everything Termux needs into your Downloads — the **gonomadnet
client** for this device's architecture, the two launchers, and the terminal's
font, colors and settings — and none of it is a choice you have to make. You do
not need to know whether your tablet is `arm64`, which release asset is yours, or
what a release asset is.

The screen lists every file with its path, and reports whether Android's own font
engine can load the font — that engine is the one Termux draws through, so its
answer is the difference between the interface's icons and empty boxes.

### 4. Tap "Set up Termux"

Termux does the last part, because only Termux may write into its own home
directory. The app asks it to run the setup script that step 3 published, and
Termux reports what it did in the new session it opens.

**The first time on a tablet, Android will refuse that request**, and the app says
so. Tap **Copy the setup line** instead and paste it into a Termux session you open
yourself:

```sh
bash /sdcard/Download/gonomadnet-setup.sh
```

That one line performs the whole installation. It is also what tells Termux to
accept commands from the app at all (`allow-external-apps = true`), so it is the
step that makes the app's own buttons work from then on.

### 5. Grant RUN_COMMAND once

**Settings → Apps → gonomadnet node → Permissions → Additional permissions →
RUN_COMMAND.** Android gives no way for an app to request this permission or to
grant it for itself; only you can. The app's startup readout shows whether it is
granted, and without it the app's **Open gonomadnet** buttons do nothing — the
client itself still runs, you just start it yourself.

### 6. Start the node

Tap **Start stack**. Three daemons start — `gornsd` (transport), `gorrcd` (hub) and
`gorrcbot` (bot) — plus three `gonsensor` converters, and the screen reports the
node's address when they are up. The screen is also where the appliance's failures
appear, so it is worth watching for a moment. The sensors start with the location
permission; **Start sensors** and **Stop sensors** control them on their own.

### 7. Open the client

Tap **Open gonomadnet (attached)**, or the *gonomadnet+stack* home-screen icon if
you installed [Termux:Widget](https://f-droid.org/packages/com.termux.widget/). The
icons are a convenience, not a requirement: the app's buttons run the same
launchers, and the launchers are what refresh the hub's address before the client
starts.

If you want the client to own its own network instead of using this app's
transport, use **Open gonomadnet (standalone)**. See [The two modes](#the-two-modes).

### 8. Ask the bot where you are

In the client, Tab to **Channels** in the menu bar and press Enter, connect to
the hub and join a room (`Ctrl-a` adds one), then send `/msg gobot whereami` from
the room's editor with `Ctrl-d`. The reply tells you where the device actually
is, from its own receiver.

### What the app does, and what you still do

| Step | Who |
| --- | --- |
| Install Termux, and check it is the build that works | **the app** |
| Get the right client binary for this device | **the app** |
| Install the client, the launchers and the attached configuration | the setup script, **run for you** |
| Set the terminal font, colors and `allow-external-apps` | the setup script |
| Allow a Termux that is not the Play build | **the app** |
| Allow installing apps at all | you, once (one toggle) |
| Grant RUN_COMMAND | you, once (one toggle) |
| Run the setup script the first time | you, once (one paste) |
| Start the node, start the sensors, open the client | you, by button |

### How the pieces fit

```
GNSS + compass -> JSON -> gonsensor -> FIFO -> gorrcbot -> gorrcd -> gornsd -> the fleet
                                              the client reads the same fix from the feed
```

## The two modes

There are two independent axes.

**Who owns Reticulum.**

| Mode | Config | Behaviour |
| --- | --- | --- |
| **Standalone** | `~/.reticulum/config` | Owns its own interfaces and network. |
| **Attached** | `~/.reticulum-stack/config` | Owns no interface; requires the app's shared instance (which the app owns). |

Both share `~/.nomadnetwork`, so the tablet keeps **one** node identity and one
message store whichever way it is started. They are mutually exclusive at run
time because they lock the same config directory.

The attached configuration sets `require_shared_instance = yes`, so the client
attaches to the app's stack or fails loudly — it can never win a race for
ownership and quietly become the transport.

**Is the sensor service running.** The sensor service runs with the daemon stack
or without it. Running it alone gives the standalone client a real position on a
tablet that runs no local bot at all, and costs the least battery.

### The two icons

| Icon | Mode | Notes |
| --- | --- | --- |
| `gonomadnet` | Standalone | Owns its own network. Resolves the hub address with bionic and pins the literal. |
| `gonomadnet+stack` | Attached | Requires the app's shared instance. Needs no hostname resolution — it has no interfaces of its own. |

Both launchers **refuse to start when the other mode is already up**, in plain
language, so you can never end up with two Reticulum networks on one device. If a
launcher says the other mode is running, stop that one first.

### Ports

| Port | Used by |
| --- | --- |
| `127.0.0.1:37428` | Reticulum shared instance (the app owns it) |
| `127.0.0.1:37429` | Reticulum's own control port — reserved, do not use |
| `127.0.0.1:37430` | the sensor feed |

All three are **loopback only**: they are reachable from this device and from
nowhere else. The shared instance is a TCP socket rather than an abstract Unix
socket, because two Android applications cannot be assumed to reach each other's
abstract sockets.

## The terminal font

The client draws its icons from the Nerd Font private-use area by default —
`glyphs = nerdfont` is the shipped setting — and Termux's own font has no such
glyphs, so the menu bar and the Users pane render as empty boxes. The app carries
**Atkynson Mono Nerd Font Mono** and hands it to Termux for this.

**The setup script installs it**, as `~/.termux/font.ttf` — the name is fixed
whatever the published file was called in Downloads. You do not copy anything by
hand, and Termux is asked to re-read its settings so the font takes effect in the
session you are looking at rather than the next one.

`font.ttf` is one of the files the script never replaces once it exists: a font you
chose yourself stays yours. To take the app's font after all, delete the file and
run the setup line again.

The readout names the file it installed. The app's own screen also reports whether
Android's font engine could load the font, and that engine is the one Termux draws
through — so it is the difference between glyphs and empty boxes.

The **license file** published beside the font is the font's own (SIL Open Font
License 1.1). It travels with the font so the license travels with the copy.

If you would rather not install a font at all, set `glyphs = unicode` in
`~/.nomadnetwork/config` and the client falls back to the symbols every terminal
has.

## The terminal colors

The client draws most of its chrome — the frame borders, the pane titles, the key
hints along the bottom — in the terminal's **default** foreground and background,
and its accents from the terminal's own **16-color palette**. Those are resolved
by whichever terminal is drawing, not by the client, so the same client under
Termux's stock white-on-black palette and under a themed desktop terminal renders
the same interface in visibly different colors. Everything the client names
explicitly (message text, nick colors, the editor bar) is identical either way.

The app therefore publishes **`colors.properties`**, which is the desktop
terminal's own theme — its `foreground`, its `background` and its sixteen palette
colors — in the form Termux reads. **You do not install this one by hand**: the
client copies it to `~/.termux/colors.properties` the first time it runs inside
Termux and asks Termux to re-read its settings. It never replaces a file that is
already there.

To see what a terminal is resolving those defaults to, ask it:

```sh
printf '\e[39mdefault foreground\e[49m\n'   # your terminal's default fg/bg
```

To go back to Termux's own colors, delete `~/.termux/colors.properties` and run
`termux-reload-settings`.

## The tmux configuration (for debugging)

**You do not need tmux to run gonomadnet on Android, and neither the app nor the
client ever starts it.** The client is meant to run directly in Termux: on a
tablet with a virtual keyboard open, tmux's status bar and its window list cost
lines that the interface wants for messages.

The app nevertheless publishes **`tmux.conf`** to `/sdcard/Download` for the
person who chooses to debug under tmux. **You do not install this one by hand**:
the client does it the first time it runs inside Termux, copying the published
file to `~/.tmux.conf`. It installs it only when nothing is there already, so a
configuration you have edited stays yours.

tmux is not part of the app. It is a Termux package you install yourself, once,
if you want it:

```sh
pkg install tmux
```

It puts the status bar on top with blue accents, numbers windows and panes from
one, and — the part worth having — **passes 24-bit color through instead of
reducing it to 256**. Drop the color settings and the whole interface shifts to
the nearest 256-color approximation. To check a running pane, run this inside it:

```sh
echo "$TERM"        # tmux-256color
echo "$COLORTERM"   # truecolor
```

## The sensors

### Heading: the screen-back normal

The heading is the direction the **back of the device** points — the way the
operator faces when they are looking at a screen on a stand. The alternative is
the **top edge of the screen**, which is right for a device lying flat and face
up. The two agree only when the device is flat, so choose the one that matches
how you mount it:

| Mounting | Reference |
| --- | --- |
| Upright on a stand, operator facing the screen | **screen-back** (default) |
| Lying flat, face up | screen-top |

Rotating the screen is corrected for separately: turning the device on its stand
does not turn your heading with it.

When the chosen reference cannot be trusted, the app reports **no heading**
rather than a wrong one, and leaves the field out of the sample entirely. That
happens when the device lies flat while the screen-back axis is selected, and
when it stands up in portrait while the screen-top axis is selected. If a command
tells you the heading is unavailable, stand the device up to use the screen-back
axis — or pick the other axis on the app's screen — and ask again.

### Compass

The heading comes from the device's fused orientation sensor, not from its raw
magnetic field. A raw field reading on a device like this one is dominated by the
case, the stand and the driver, and carries no indication of how good it is.

### GNSS

The receiver works, but **a cold start with no sky view does not converge**, and
there is no timeout that turns an absence of satellites into a position. Indoors
the app sits in an "acquiring satellites" state — that is the normal waiting
state, not an error. Expect tens of seconds to the first fix with a clear view.

The heading never comes from GNSS: on a device that is standing still, a
satellite-derived course is tens of degrees off. It is used only as a cross-check
on the compass.

### Sample rate and battery

Sensors are registered at 50 Hz. All of them are **non-wakeup**, so their
readings stop when no wake lock is held.

The sensor service holds a partial wake lock for as long as it runs, and releases
it when you tap **Stop sensors**. That is what keeps the readings flowing with the
screen off, and it is also the app's main battery cost: run the sensors only while
you want a live position, and the client and node work without them.

## The sensor feed

The sensor service publishes position and heading as a feed the bot and the
client both read. Two things about it fail quietly, so they are worth knowing.

**The service holds the feed open at both ends** (the FIFOs are opened
`O_RDWR`). Keep it that way. A reader that opens the pipe read-only **blocks
forever** when nothing holds the write end, with no error and no timeout.

| The service is | A reader that opens the feed | A reader already attached |
| --- | --- | --- |
| running | opens at once | reads samples as they arrive |
| stopped | **blocks forever, silently** | sees end of file and stops |

**Restarting the bot is safe.** Kill `gorrcbot` and start it again while the
service is running; it re-reads the feed immediately.

**Start the service before the bot.** With the service stopped, a newly started
bot parks in the open with no log line to say so. The app's **Start stack**
button does this in the right order.

## The bot

The bot answers a private request only when the hub can deliver the reply. Two
conditions must hold, and the app sets both for the hub it starts:

| Requirement | Why | How it is set |
| --- | --- | --- |
| The bot and its hub are attached to the **same Reticulum instance** | A bot on one instance cannot hear a hub on another | Pass the hub `--configdir`, and make sure the config file does not blank it out |
| The hub **names the joiner** in the member notification it fans out | A bot already in a room otherwise never learns a peer that joins later, and drops that peer's requests | Run the hub with `--include-joined-member-list` |

If you run your own hub and the bot answers nothing, check those two first.

Every position command is answered from the tablet's own receiver:

| Ask | Uses the live fix? |
| --- | --- |
| `whereami` | yes |
| `loc C984+5V` | yes — a shortened Plus Code is completed against the live fix |
| `sun` | yes |
| `dist <a> to <b>`, `proj <point> <course> <distance>` | no — the points are named explicitly |
| `moon` | no — an almanac is the same for everybody |

`loc` with no argument prints its usage line (`loc <pluscode|coords|grid>`) rather
than an answer.

## Privacy

The app knows where the tablet is, because that is the point: it publishes the
position and heading so position commands can answer with them. What matters is
what happens next.

**Nothing about your position is sent unless you ask for it.**

- **Announces carry no coordinates.** The app announces the node and its
  messaging address, as any Reticulum node does.
- **Commands answer only the person who asked.** `/msg gobot whereami` replies
  with direct notices to that one client; nothing is posted into a room.
- **The client never shows your position to anyone else.** A `L` location
  construct in a page is resolved for *the reader*; your own coordinate is never
  rendered into a page, a status line, or a message.
- **Nothing is stored in a form that can leak later.** With a live fix in hand,
  no notation of that position — decimal, DMS, Plus Code, Maidenhead grid, or the
  raw bits — appears in the node's state on disk.

These are not just rules the code follows: the parts of the client that announce,
fetch pages, send messages and write state are given no access to the position at
all. The client's interface is the only thing that holds it, and the only thing
that renders it.

**You can always see what the app knows.** The client logs the reader's position
whenever it changes:

```
reader position: no fix from the sensor feed tcp://127.0.0.1:37430
reader position: a live fix, just now, +-3.8 m
```

A static coordinate pasted into a config file is not part of this design: the
live feed always takes precedence over a configured `fix`.

## Debugging

Logs first, always.

The commands below need a computer with `adb`, and the ones that use `run-as`
need a **debug build** of the app (`./scripts/build-android-apk.sh --debug`): a
release APK is not debuggable, and Android refuses `run-as` on it.

```sh
PKG=com.gmlewis.gonomadnet

# The daemons' logs and the sensor service's
adb shell run-as $PKG ls files/logs
adb shell run-as $PKG cat files/logs/gorrcd.log
adb shell run-as $PKG cat files/logs/gorrcbot.log

# The app's own log, for permission, install and start-up problems
adb logcat -s gonomadnet

# The sensor service's log, which is a tag of its own
adb logcat -s gonomadnet-sensors

# What the setup script did, which is the only report from inside Termux
adb shell cat /sdcard/Download/gonomadnet-setup-status.txt

# What the appliance published, and under which names
adb shell ls -l /sdcard/Download | grep -E 'gonomadnet|NerdFont|reticulum|tmux|colors'

# Is the app running, and as which uid?
adb shell ps -A -o PID,USER,NAME | grep -E 'gonomadnet|gornsd|gorrcd|gorrcbot'
```

Then check what is supposed to be listening, because a service can be running
while its port is not bound:

```sh
# The shared instance and the sensor feed (37428 and 37430 in hex)
adb shell 'cat /proc/net/tcp' | grep -iE '9234|9236'
adb shell 'cat /proc/net/unix' | grep -i reticulu

# From inside the app, where /proc is unreadable: bash's /dev/tcp
adb shell run-as $PKG sh -c 'exec 3<>/dev/tcp/127.0.0.1/37428 && echo shared instance up'
```

For the sensors themselves:

```sh
adb shell dumpsys sensorservice | head -40
adb shell dumpsys location | head -40   # whether a provider has ever produced a fix
```

## Troubleshooting

| What you see | What it means | What to do |
| --- | --- | --- |
| **Open gonomadnet** and **Set up Termux** do nothing at all | Termux ignores commands from other apps until it is told not to, and RUN_COMMAND has to be granted by hand | Tap **Copy the setup line** and paste it into Termux; then grant RUN_COMMAND in Settings (step 5 of [Getting started](#getting-started)) |
| The app says the Termux on this device "cannot run programs it installs" | It is the Play Store build (`targetSdk` 29 or more) | Uninstall it — that deletes Termux's home directory — and use **Install Termux** in the app |
| The app says it is "not allowed to install applications yet" | Android's "install unknown apps" is off for this app | Allow it on the screen the app opened; the app cannot turn it on itself |
| The app refuses the download because it is "signed by … not by F-Droid's key" | Whatever answered on that URL is not the build the app pins | Nothing to do here — it was not installed. Check the network for a captive portal, or install Termux from F-Droid's own client |
| The setup script says it "cannot read shared storage" | Termux has never been granted access; `~/storage` does not exist | Run the setup line again and accept the prompt Termux shows. The script waits twenty seconds for it |
| The setup script says the client "was not found" | The app published nothing, or published to a different directory | Tap **Publish files for Termux**, as the script's own report says |
| `~/gonomadnet: Permission denied` | The client is installed and not executable | Run the setup line again; it chmods the client every time |
| The client says it cannot attach; `37428` refuses | The node stack is not running | Tap **Start stack**, and wait for the app to report the node address |
| `SIGSYS: bad system call` in a daemon's log | A daemon resolved a bare program name; Android kills the process for it | Every subprocess must be launched by an absolute path |
| `route ip+net: netlinkrib: permission denied` | `AutoInterface` is blocked on Android | Use a `TCPClientInterface` instead; see [`docs/Android.md`](Android.md) |
| `Failed to initialize TCP server interface …: listen tcp: address [[::]]:4242: missing port in address` | `listen_ip = [::]` was bracketed a second time | Write `listen_ip = ::` without brackets |
| `dial tcp: lookup "2603:…": no such host` | A value in the Reticulum config is quoted; its INI parser keeps the quotes as part of the value | Write every config value bare, as the shipped configuration does |
| `Could not initialize Reticulum: … 127.0.0.1:37429: bind: address already in use` | Something else took Reticulum's own control port | Do not publish anything on 37429; the sensor feed uses 37430 |
| The bot never connects; `hub "…": cannot connect: Hub identity unknown` | The hub is attached to a **different** Reticulum instance than the bot | Start the hub with the same configuration directory the clients use |
| The bot connects, requests arrive, and **nothing** is answered | The hub is not naming the joiner, so the bot never learns the asker | Run the hub with `--include-joined-member-list` |
| A restarted bot never starts, and logs nothing | The sensor service is not running, so opening the feed blocks forever | Start the service first; the app's **Start stack** does this in order |
| `Termux:API NOT INSTALLED` | The optional clipboard add-on is absent | Nothing — only the clipboard is affected |

### Daemon command-line conventions

These differ per program, and passing the wrong kind of path is the usual cause of
a daemon that will not start:

| Program | Option | Takes |
| --- | --- | --- |
| `gornsd` | `--config` | the Reticulum configuration **directory** |
| `gorrcbot` | `--config` | the Reticulum configuration **directory** |
| `gorrcbot` | `--bot-config` | the bot's own TOML **file** |
| `gorrcbot` | `--home` | the bot's home directory |
| `gorrcd` | `--config` | the hub's TOML **file** |
| `gorrcd` | `--configdir` | the Reticulum configuration **directory** |

**`gorrcd` and `gorrcbot` bootstrap and exit 0 on their first run**, writing their
default configuration. Starting them as daemons the first time leaves a
supervisor believing it has a running service when what it has is a file and a
dead process. Run each to completion once before starting it for real; the
appliance's supervisor does.

## What the app publishes, and where it goes

The app writes these into your Downloads. **Publish files for Termux** does it, and
the screen lists each one with the path it actually got — Android's own type table
renames files as it stores them, so a font published as `…Regular.otf` arrives as
`…Regular.otf.ttf`, and the app reports the name it really has rather than the one
it asked for.

| Published | The setup script installs it as |
| --- | --- |
| `gonomadnet-client` | `~/gonomadnet` — always replaced, it is the app's own build |
| `gonomadnet-standalone` | `~/.shortcuts/gonomadnet` |
| `gonomadnet-stack` | `~/.shortcuts/gonomadnet+stack` |
| `reticulum-stack-config` | `~/.reticulum-stack/config` |
| `AtkynsonMonoNerdFontMono-Regular.otf` | `~/.termux/font.ttf` |
| `AtkinsonHyperlegibleMono-OFL.txt` | (the font's license — not installed) |
| `gonomadnet-setup.sh` | (the script itself — run, not installed) |
| `colors.properties` | `~/.termux/colors.properties` — installed by **the client**, on its first run |
| `tmux.conf` | `~/.tmux.conf` — installed by **the client**, for anyone debugging under tmux |

Everything except the client is installed only when it is not already there. The
client is replaced every time because it is the app's binary and not yours.

The script writes what it did to `gonomadnet-setup-status.txt` beside those files,
as well as to its own session. Termux's home directory can be read neither by the
app nor by `adb`, so shared storage is the only way a setup that half worked can be
told from one that did nothing.

## By hand, if you prefer

Nothing above needs the app's buttons. Every step is a command, and these are them.

**Termux**, from F-Droid and not from Play — [the package
page](https://f-droid.org/packages/com.termux/) — plus
[Termux:Widget](https://f-droid.org/packages/com.termux.widget/) if you want the
home-screen icons. Check which build you got:

```sh
adb shell dumpsys package com.termux | grep -E "versionName|targetSdk|installerPackageName"
```

The `targetSdk` line must read `28`. A higher number, or an
`installerPackageName` of `com.android.vending`, is the Play build, which cannot
run programs it installs. See [`docs/Android.md`](Android.md) for what else that
build breaks and why signing keys are not interchangeable.

**Everything else**, in Termux, once the app has published the files or you have
put them in `/sdcard/Download` yourself:

```sh
bash /sdcard/Download/gonomadnet-setup.sh
```

That is the whole installation — the client, both launchers, the attached
configuration, the font and `allow-external-apps`. It is idempotent, so running it
again after a new release updates the client and reports everything else as
already there. Give it a directory if you published somewhere else:
`bash gonomadnet-setup.sh /sdcard/MyDownloads`.

Then grant **RUN_COMMAND** in Settings, if you want the app's buttons to work, and
start the client:

```sh
~/gonomadnet -t --rnsconfig ~/.reticulum-stack    # attached to the app's transport
~/gonomadnet -t                                   # or with its own network
```

With a computer, the equivalent of the publish step is
`adb push gonomadnet-<version>-linux-arm64 /sdcard/Download/gonomadnet-client`,
and every Android device made since about 2017 is `arm64`.

## Building it yourself

You do not need to build anything to use the app: the signed APK is attached to
the [latest release](https://github.com/gmlewis/go-nomadnet/releases). To build
one from source, sign it, or publish it with a release, see
[Android-Build.md](Android-Build.md).
