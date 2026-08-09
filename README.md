# gonomadnet — Go Nomad Network Client <a href="https://github.com/gmlewis/go-nomadnet/actions/workflows/build.yml"><img align="right" src="https://github.com/gmlewis/go-nomadnet/actions/workflows/build.yml/badge.svg"/></a>

![gonomadnet mascot
The Go gopher was designed by Renee French.
The design is licensed under the Creative Commons 4.0 Attribution license.](assets/gonomadnet-mascot.png)

A complete Go port of [NomadNet](https://github.com/markqvist/nomadnet), a
peer-to-peer messaging and information sharing system built on
[Reticulum](https://reticulum.network). NomadNet enables private, encrypted
communication over any network transport — including LoRa, packet radio, and
the internet.

## Features

- **Pure Go** — no CGO required; builds with `go build ./...`
- **Cross-platform** — works on Linux, macOS, and Windows
- **Terminal UI** — full-featured TUI built with [rivo/tview](https://github.com/rivo/tview)
- **LXMF messaging** — send/receive encrypted messages via Reticulum
- **RRC chat** — join relay chat rooms on Reticulum hubs
- **Node serving** — host Micron pages and files for browsing
- **Micron markup** — lightweight page rendering with headings, formatting, colors, links
- **Directory** — peer discovery with trust levels and propagation node selection
- **Daemon mode** — headless operation for servers and embedded devices
- **Dark/Light themes** — configurable color palettes with unicode/nerdfont glyph sets

## Installation

### Binary

If you already have [Go](https://go.dev/) installed, you can
install `gonomadnet` directly from GitHub without cloning the repo:

```bash
go install github.com/gmlewis/go-nomadnet/cmd/gonomadnet@v0.9.0
```

This puts the `gonomadnet` binary in your `$GOPATH/bin` (or `$GOBIN`)
(which should already be in your `$PATH`).

Alternatively, you can download the latest binary release for your platform
from: [Releases](https://github.com/gmlewis/go-nomadnet/releases).

### Build from Source

```bash
git clone https://github.com/gmlewis/go-nomadnet
cd go-nomadnet
go build -o gonomadnet ./cmd/gonomadnet/
```

## Quick Start

### First Run

```bash
# Start in text UI mode (default)
gonomadnet

# Start in daemon mode (no UI)
gonomadnet --daemon

# Use a custom config directory
gonomadnet --config ~/my-nomadnet-config

# Show version
gonomadnet --version
```

### Command-Line Options

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | | Path to alternative NomadNet config directory |
| `--rnsconfig` | | Path to alternative Reticulum config directory |
| `--textui` | `-t` | Run in text-UI mode (the default) |
| `--daemon` | `-d` | Run in daemon mode (no UI) |
| `--console` | `-c` | In daemon mode, log to console instead of file |
| `--version` | | Show version and exit |

### Installing Alongside Python NomadNet

The Go binary is named `gonomadnet` (not `nomadnet`) so it can be installed
on the same machine as the Python `nomadnet` without either overwriting the
other. Both read from `~/.nomadnetwork` by default and share the same
Reticulum identity, peer directory, and message store — so you can switch
between them and pick up the same conversations, trusted peers, and
announced nodes.

Run only **one** at a time. Because they share a single identity and
storage directory, launching both concurrently would have two processes
claim the same LXMF destination and contend for the same files and RNS
interfaces. Quit one before starting the other. (If you genuinely need both
running at once, give each its own `--config` and `--rnsconfig` so they use
separate identities, storage, and Reticulum interfaces.)

## Configuration

`gonomadnet` reads its configuration from `~/.nomadnetwork/config`. The
format is INI-style, identical to the Python version:

```ini
[logging]
    loglevel = 4
    destination = file

[client]
    enable_client = yes
    user_interface = text
    downloads_path = ~/Downloads
    announce_at_start = yes
    announce_interval = 360

[textui]
    intro_time = 1
    theme = dark
    colormode = 24bit
    glyphs = unicode
    editor = nano

[rrc]
    history_per_room_cap = 500
    nick_colors = yes
    render_micron = yes

[node]
    enable_node = no
    announce_interval = 360
```

See the Python NomadNet documentation for all available options.

## Package Overview

| Package | Description |
|---------|-------------|
| `nomadnet/app` | Central app singleton: config, identity, LXMF router, directory |
| `nomadnet/browser` | Browser backend: URL parsing, page fetching, downloads, page cache |
| `nomadnet/config` | INI-style config file parsing and I/O |
| `nomadnet/conversation` | LXMF conversation management and message storage |
| `nomadnet/directory` | Peer directory with trust levels and announce streams |
| `nomadnet/micron` | Micron markup parser (headings, formatting, colors, links) |
| `nomadnet/node` | NomadNet node: serves pages and files over RNS |
| `nomadnet/peersettings` | Peer settings management |
| `nomadnet/rrc` | Reticulum Relay Chat: hubs, rooms, CBOR persistence |
| `nomadnet/storage` | Storage directory management |
| `nomadnet/util` | Text sanitization utilities |
| `nomadnet/version` | Version constant |
| `nomadnet/asciichart` | ASCII chart renderer for bandwidth display |
| `tui` | Terminal UI: all menu pages (browser, conversations, channels, network, guide, config, log, interfaces), dialogs, micron styled renderer, themes & glyphs |

## Terminal UI

The TUI's top-level menu mirrors Python nomadnet: Conversations, Network,
Channels, Log, Interfaces, Config, Guide, and Quit. (Directory and Map are
sub-displays reached from within those pages, not top-level menu buttons.)

- **Conversations** — Message list, compose, read/reply
- **Network** — Announce stream, known nodes/peers, propagation nodes
- **Channels** — RRC chat rooms, member list, message history
- **Log** — Log file viewer
- **Interfaces** — RNS interface status and bandwidth charts
- **Config** — View/edit configuration
- **Guide** — Help content rendered as Micron pages

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Left` / `Right` | Move the menu highlight |
| `Enter` / `Space` | Activate the focused menu item (switch page) |
| `Tab` / `Down` | Drop focus to the body |
| `Up` (at top of a list) | Return focus to the menu |
| `Ctrl-Q` / `Ctrl-C` | Quit |
| `Esc` | Close the top dialog / return to the menu |

Per-page shortcuts (open a URL, sync, back, etc.) are shown in the
shortcut bar at the bottom of each display.

## Note for Ghostty users

If you find that your TUI seems sluggish and can't keep up with your mouse
scrolls and are using Ghostty, please try using a different terminal emulator
and see if performance improves. For some reason, event handling and rendering
in Ghostty appear to get extremely bogged down and can even crash with too much I/O.

## Testing

```bash
go test ./...          # all tests
go test -race ./...    # with the race detector
```

## Development

### Project Structure

```
go-nomadnet/
├── cmd/gonomadnet/        # CLI entry point (text-UI + daemon modes)
├── nomadnet/              # Core library packages
│   ├── app/               # App singleton, LXMF router, directory
│   ├── browser/           # Browser backend (URL parsing, page fetch, cache)
│   ├── config/            # Configuration
│   ├── conversation/      # LXMF messages
│   ├── directory/         # Peer directory
│   ├── micron/            # Micron parser
│   ├── node/              # Node serving
│   ├── peersettings/      # Peer settings
│   ├── rrc/               # Relay chat
│   └── ...
├── tui/                   # Terminal UI (all menu pages, dialogs, renderer)
├── tooling/               # Parity harnesses & screencast tooling
├── scripts/               # Test/run helper shell scripts
└── skills/                # Repo-local development skills
```

### Dependencies

- [`github.com/gmlewis/go-reticulum`](https://github.com/gmlewis/go-reticulum) — Reticulum Network Stack
- [`github.com/rivo/tview`](https://github.com/rivo/tview) — Terminal UI framework
- [`github.com/gdamore/tcell/v2`](https://github.com/gdamore/tcell/v2) — Terminal cell library
- [`github.com/fxamacker/cbor/v2`](https://github.com/fxamacker/cbor/v2) — CBOR codec

## Status

Although this port seems to be fully functional, there may still be bugs.
If you find bugs, please report them as new
[GitHub Issues](https://github.com/gmlewis/go-nomadnet/issues).

## License

GNU General Public License v3 — see [LICENSE](LICENSE) for details.
