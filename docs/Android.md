# Running gonomadnet on Android (Termux)

`gonomadnet` is a terminal application, so on Android it runs inside
[Termux](https://termux.dev), the terminal emulator and Linux environment that
gives an unprivileged Android app a real PTY, a shell, and a writable home
directory. No Android GUI port is needed: the same `tview`/`tcell` interface,
the same Reticulum stack, and the same LXMF router run unmodified.

This guide is written from a verified end-to-end run on a Samsung Galaxy Tab A9+
(`SM-X210`, `arm64-v8a`, Android 16 / API 36). It documents not just the steps
but the three Android platform restrictions that will otherwise cost you an
afternoon, each with its exact error message so you can recognise it.

---

## 1. What works, and what does not

| Capability | On Android | Notes |
| --- | --- | --- |
| Text UI (tview/tcell) | **Works** | Full menu bar, colours, wide characters, mouse |
| Reticulum transport, identity, storage | **Works** | Interface enumeration is the exception — see below |
| LXMF messaging, announces, path learning | **Works** | Verified sending and receiving |
| Serving your own NomadNet node | **Works** | Peers can open links to it |
| `TCPClientInterface` / `TCPServerInterface` | **Works** | Including IPv6 literals |
| `RNodeInterface` over USB-OTG | **Works** | Android grants USB serial access through Termux |
| `AutoInterface` | **Does not work** | Two independent Android restrictions; use TCP or RNode |
| System clipboard integration | **Partial** | Needs the `Termux:API` app — see [§7](#7-clipboard-and-other-polish) |

`AutoInterface` is disabled by the platform, not by a bug. Termux runs in
Android's `untrusted_app` SELinux domain, which is denied netlink route sockets,
so Reticulum cannot enumerate interfaces at all:

```
[Error] Failed to initialize Auto interface Default Interface:
        route ip+net: netlinkrib: permission denied
```

Even if enumeration worked, Android's Wi-Fi driver drops multicast for
applications that do not hold a `WifiManager.MulticastLock`, and Termux has no
API to acquire one. Configure a `TCPClientInterface` (or `RNodeInterface`)
instead, as shown in [§5](#5-configure-reticulum).

---

## 2. Install the correct Termux build — do not skip this

**Install Termux from [F-Droid](https://f-droid.org/packages/com.termux/), not
from Google Play.**

This is not a preference. Android 10 (API 29) and later forbid an application
whose `targetSdkVersion` is 29 or higher from calling `exec()` on files inside
its own data directory. Termux keeps its entire `$PREFIX` (its `bin`, `lib`, and
bootstrapped packages) under its data directory, so a Termux built against a
modern `targetSdk` cannot run anything it installs.

- F-Droid's `com.termux` is built with `targetSdkVersion 28` and therefore works.
- The Play Store build uses a modern `targetSdk` and **cannot run
  `gonomadnet`**, or its own bootstrap, at all.

You can confirm which you have:

```sh
# From a host with adb, or read it in Termux's App Info:
adb shell dumpsys package com.termux | grep -E "versionName|targetSdk"
```

The `targetSdk` line must read `28`.

> **Signing keys are not interchangeable.** F-Droid builds are signed by F-Droid;
> upstream GitHub release APKs are signed by the Termux maintainers. Switching
> between the two always requires uninstalling first, which deletes your Termux
> home directory and everything in it. Pick one source and stay with it.

If F-Droid refuses to install with `INSTALL_FAILED_VERIFICATION_FAILURE`,
Android's package verifier is rejecting the sideload:

```sh
adb shell settings put global verifier_verify_adb_installs 0
adb shell settings put global package_verifier_enable 0
```

Revert later with `adb shell settings delete global <name>`.

---

## 3. Build a binary for the device

Android is Linux under the hood, so build for `linux`, not `android`. Building
with `GOOS=android` requires cgo and the NDK (`golang.design/x/clipboard` ships
a cgo JNI implementation behind an `android` build tag), and buys nothing for a
Termux-hosted process. A `CGO_ENABLED=0` build is a statically linked binary
that needs no Termux packages installed at all.

Map the device's ABI to a Go target:

| Device ABI (`getprop ro.product.cpu.abi`) | `GOARCH` |
| --- | --- |
| `arm64-v8a` (most devices since ~2017) | `arm64` |
| `armeabi-v7a` | `arm` (`GOARM=7`) |
| `x86_64` (emulators, some tablets) | `amd64` |
| `x86` (old emulators) | `386` |

```bash
git clone https://github.com/gmlewis/go-nomadnet
cd go-nomadnet
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o gonomadnet ./cmd/gonomadnet
```

The result is a static ELF, roughly 20 MB:

```console
$ file gonomadnet
gonomadnet: ELF 64-bit LSB executable, ARM aarch64, version 1 (SYSV), statically linked, Go BuildID=..., with debug_info, not stripped
```

Copy it to the device over the shared filesystem:

```bash
adb push gonomadnet /sdcard/Download/gonomadnet
```

---

## 4. Install it inside Termux

Open Termux and run:

```sh
termux-setup-storage          # one-time: grants access to /sdcard
cp /sdcard/Download/gonomadnet ~/
chmod 755 ~/gonomadnet
~/gonomadnet --version
```

The first real run creates `~/.reticulum/config`, `~/.nomadnetwork/`, an
identity, and the storage tree. Termux's home directory cannot be written from
`adb` — it is a different application's private data — so configuration changes
are made from inside Termux or by copying files in from `/sdcard`.

If you prefer to keep configuration elsewhere, both directories are
overridable:

```sh
~/gonomadnet --config ~/.nomadnetwork --rnsconfig ~/.reticulum -t
```

---

## 5. Configure Reticulum

Replace the generated `~/.reticulum/config` with a TCP-based configuration.
`AutoInterface` is deliberately disabled for the reason in [§1](#1-what-works-and-what-does-not).

```ini
[reticulum]
  enable_transport = no
  share_instance = yes
  instance_name = default

[logging]
  loglevel = 4

[interfaces]
  # Point this at any reachable Reticulum TCP gateway. The public gonomadnet
  # hub is documented in the project README.
  [[Reticulum Hub]]
    type = TCPClientInterface
    interface_enabled = yes
    target_host = go-nomadnet.duckdns.org
    target_port = 4242
    name = Reticulum Hub

  # Disabled: Android denies netlink route sockets to applications, so
  # Reticulum cannot enumerate interfaces, and multicast is filtered without a
  # WifiManager.MulticastLock that Termux cannot acquire.
  [[Home LAN]]
    type = AutoInterface
    interface_enabled = no
```

IPv6 literals are supported directly:

```ini
    target_host = 2603:900b:3300:a::1be9
```

### Using a hostname instead of a literal IP

**Read this if `target_host` is a name.** Android has no `/etc/resolv.conf` —
`/etc` is a read-only symlink to `/system/etc` — and Android's resolver lives
behind `netd` over a Unix socket that only the C library (bionic) can reach. A
`CGO_ENABLED=0` Go binary resolves with Go's own resolver, which falls back to
`defaultNS = ["127.0.0.1:53", "[::1]:53"]` when that file is missing. Every
lookup therefore fails:

```
Go TCPClientInterface Home Hub connect failed: dial tcp: lookup
go-nomadnet.duckdns.org on [::1]:53: read udp [::1]:36058->[::1]:53:
read: connection refused
```

The interface is not broken and DNS is not broken — Android simply gives a Go
process no way to ask. Two workarounds, both in use:

**Option A — pin a literal address.** Resolve the name on another machine and
put the address in `target_host`. Simple, but you must update it when the
address changes.

**Option B — resolve with bionic before launch.** Android's own binaries do
resolve correctly (they use bionic), so let the launcher resolve the name and
pin the result. `/system/bin/ping6 -n` prints a numeric address and is
executable by an unprivileged app:

```console
$ /system/bin/ping6 -n -c 1 -W 3 go-nomadnet.duckdns.org
PING go-nomadnet.duckdns.org(2603:900b:3300:a::1be9) 56 data bytes
```

This is what the launcher script in [§6](#6-add-a-home-screen-icon) does, so the
name stays the source of truth and a DNS change is picked up on the next launch.

---

## 6. Add a home-screen icon

Termux has no launcher shortcut of its own; the supported mechanism is the
official **Termux:Widget** add-on, which runs a script from `~/.shortcuts/`.

1. Install **Termux:Widget** from
   [F-Droid](https://f-droid.org/packages/com.termux.widget/).
2. **Grant Termux "Draw over other apps".** This step is mandatory and is the
   single most common cause of a shortcut that appears to do nothing. Since
   Android 10, a background application may not start a foreground activity, so
   the Termux service cannot open the terminal session for your script. The
   attempt fails silently, and Termux's own notification explaining it is
   suppressed unless notifications are allowed:

   ```
   W Termux:PermissionUtils: com.termux does not have Display over other apps
     (SYSTEM_ALERT_WINDOW) permission
   E NotificationService: Suppressing notification from package com.termux
   ```

   Grant it from *Settings → Apps → Termux → Advanced → Draw over other apps*,
   or from a host:

   ```sh
   adb shell appops set com.termux SYSTEM_ALERT_WINDOW allow
   adb shell pm grant com.termux android.permission.POST_NOTIFICATIONS
   ```

3. Create the launcher script at `~/.shortcuts/gonomadnet`. It must begin with a
   Termux shebang (`#!/data/data/com.termux/files/usr/bin/bash`, not
   `/usr/bin/bash`), because the shortcut does not load the full Termux
   environment. The `~/.shortcuts` directory must be mode `0700`.

```bash
#!/data/data/com.termux/files/usr/bin/bash
# Termux:Widget target. Tap the home-screen icon to run gonomadnet.

HUB_HOST=go-nomadnet.duckdns.org
CONF="$HOME/.reticulum/config"

cd "$HOME" || exit 1

# Android has no /etc/resolv.conf and this binary is CGO_ENABLED=0, so it
# cannot resolve names (see docs/ANDROID.md §5). Resolve with bionic and pin
# the literal address so a DuckDNS/address change is picked up automatically.
if [ -f "$CONF" ]; then
  line=$(/system/bin/ping6 -n -c 1 -W 3 "$HUB_HOST" 2>/dev/null | head -n 1)
  addr=${line#*(}; addr=${addr%)*}
  [ -n "$addr" ] || addr="$HUB_HOST"
  old=$(grep -m 1 'target_host' "$CONF" 2>/dev/null); old=${old#*=}; old=${old//[[:space:]]/}
  if [ "$addr" != "$old" ]; then
    echo "gonomadnet: hub $HUB_HOST -> $addr"
    sed "s|^\([[:space:]]*target_host[[:space:]]*=\).*|\1 $addr|" "$CONF" > "$CONF.new" &&
      mv "$CONF.new" "$CONF"
    chmod 600 "$CONF"
  fi
fi

# Keep the CPU and Wi-Fi alive; without this Doze suspends the node when the
# screen is off. Ships with core termux-tools.
termux-wake-lock >/dev/null 2>&1

exec "$HOME/gonomadnet" -t
```

   Make it executable:

```sh
chmod 700 ~/.shortcuts
chmod 755 ~/.shortcuts/gonomadnet
```

4. Long-press an empty area of the home screen → **Widgets** → search
   **Termux** → drag the **"Termux shortcut" (1×1)** widget onto the home
   screen, then pick `gonomadnet` from the list it presents.

Tapping the icon now opens Termux and starts the client. Use
`termux-wake-unlock` to release the wake lock when you stop it.

---

## 7. Clipboard and other polish

- **Clipboard.** On Linux the clipboard backend targets X11/Wayland, neither of
  which exists on Android, so selection and copy silently do nothing. Install
  the **Termux:API** app and `pkg install termux-api`, then have the copy path
  shell out to `termux-clipboard-set`.
- **Glyphs.** `[textui] glyphs = nerdfont` needs a Nerd Font in your terminal
  font; if box-drawing and icons render as empty boxes, set `glyphs = unicode`.
- **Log level.** `[logging] loglevel = 7` (extreme) writes megabytes of log in
  minutes. `4` (info) is a better default on battery and flash storage.
- **Editing files.** `[textui] editor = nano` assumes a Termux package that
  provides it; `pkg install nano`, or point `editor` at `vi`.

---

## 8. Verify it is working

Watch `~/.nomadnetwork/logfile`. A healthy start looks like this:

```
[Info]     Nomad Network Client 0.163.0 starting...
[Info]     Initializing RNS transport...
[Info]     Go TCPClientInterface Home Hub connected
[Info]     Announce sent for <your lxmf hash>
[Info]     Node announce received: hash=<peer> name="<some node>"
[Info]     LXMF announce received: hash=<peer> name="<some peer>"
```

The decisive lines are `connected` (the transport is up), `Announce sent` (you
are announcing), and any `announce received` (you are hearing the network).

The node's own addresses appear in the UI, or in the log:

```sh
grep -E "LXMF Router ready|nomadnetwork.node" ~/.nomadnetwork/logfile | tail
```

---

## 9. Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| Tapping the icon does nothing | Termux lacks "Draw over other apps" | [§6 step 2](#6-add-a-home-screen-icon) |
| `bash: gonomadnet: Permission denied` | Termux from Play Store (`targetSdk` ≥ 29) | [§2](#2-install-the-correct-termux-build--do-not-skip-this) |
| `SIGSYS: bad system call` at startup | An `exec` of a bare program name hit Android's seccomp filter (fixed upstream for `ps` in this repo; other call sites may exist) | Prefer absolute paths; report it |
| `lookup <name> on [::1]:53: connection refused` | No `/etc/resolv.conf`; Go cannot resolve | [§5](#using-a-hostname-instead-of-a-literal-ip) |
| `netlinkrib: permission denied` | `AutoInterface` cannot enumerate interfaces | Use `TCPClientInterface` / `RNodeInterface` |
| `too many colons in address` | An IPv6 literal was joined without brackets | Update `go-reticulum`; fixed in `hostPortAddr` |
| TUI starts then exits immediately | No terminal on stdin; the client fell back to daemon mode | Run with `-t` inside a Termux session |
| Node stops when the screen is off | Doze suspended it | `termux-wake-lock` |
| `INSTALL_FAILED_VERIFICATION_FAILURE` | Android package verifier | [§2](#2-install-the-correct-termux-build--do-not-skip-this) |

---

## Appendix: NomadNet configuration

`~/.nomadnetwork/config` is the ordinary client configuration; nothing about it
is Android-specific. Two settings are worth attention on a tablet:

```ini
[logging]
loglevel = 4          ; 7 (extreme) is very heavy on flash and battery

[textui]
glyphs = unicode      ; use nerdfont only if your terminal font provides them

[node]
node_name = gonomadnet on <your device>   ; must be unique per node
```

Everything else — `[client]`, `[textui]` theme and colour depth, announce
intervals, propagation settings — behaves exactly as it does on desktop.
