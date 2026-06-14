# go-nomadnet — Nomad Network Client (Go)

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

Install directly from GitHub — no clone needed:

```bash
go install github.com/gmlewis/go-nomadnet/cmd/gonomadnet@latest
```

This puts the `gonomadnet` binary on your `$GOPATH/bin` (or `$GOBIN`). Make
sure that directory is on your `PATH`.

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
| `--textui` | `-t` | Run in text-UI mode |
| `--daemon` | `-d` | Run in daemon mode (no UI) |
| `--console` | `-c` | In daemon mode, log to console instead of file |
| `--version` | | Show version and exit |

### Running Alongside Python NomadNet

The Go binary is named `gonomadnet` so it can coexist with the Python
`nomadnet` on the same machine. Both read from `~/.nomadnetwork` by default
and share the same Reticulum identity and message storage.

```bash
# Compare side by side
nomadnet --textui &       # Python version
gonomadnet --textui       # Go version
```

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
| `nomadnet/config` | INI-style config file parsing and I/O |
| `nomadnet/conversation` | LXMF conversation management and message storage |
| `nomadnet/directory` | Peer directory with trust levels and announce streams |
| `nomadnet/micron` | Micron markup parser (headings, formatting, colors, links) |
| `nomadnet/node` | NomadNet node: serves pages and files over RNS |
| `nomadnet/rrc` | Reticulum Relay Chat: hubs, rooms, CBOR persistence |
| `nomadnet/storage` | Storage directory management |
| `nomadnet/util` | Text sanitization utilities |
| `nomadnet/version` | Version constant |
| `nomadnet/asciichart` | ASCII chart renderer for bandwidth display |
| `tui` | Terminal UI framework (themes, glyphs, menu bar) |

## Terminal UI

The TUI provides a tabbed interface with these displays:

- **Network** — Announce stream, known nodes/peers, propagation nodes
- **Conversations** — Message list, compose, read/reply
- **Channels** — RRC chat rooms, member list, message history
- **Directory** — Known peers with trust levels
- **Guide** — Help content rendered as Micron pages
- **Config** — View/edit configuration
- **Log** — Log file viewer
- **Interfaces** — RNS interface status and bandwidth charts

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Tab` / `Shift-Tab` | Switch between menu items |
| `1`-`9`, `0` | Jump to menu item |
| `q` | Quit |
| `Esc` | Quit |

## Testing

```bash
# Run all tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run tests for a specific package
go test ./nomadnet/micron/...
```

## Development

### Project Structure

```
go-nomadnet/
├── cmd/gonomadnet/        # CLI entry point
├── nomadnet/              # Core library packages
│   ├── app/               # App singleton
│   ├── config/            # Configuration
│   ├── conversation/      # Messages
│   ├── directory/         # Peer directory
│   ├── micron/            # Micron parser
│   ├── node/              # Node serving
│   ├── rrc/               # Relay chat
│   └── ...
├── tui/                   # Terminal UI
└── go.work               # Workspace (links to go-reticulum)
```

### Dependencies

- `github.com/gmlewis/go-reticulum` — Reticulum Network Stack (local workspace)
- `github.com/rivo/tview` — Terminal UI framework
- `github.com/gdamore/tcell/v2` — Terminal cell library
- `github.com/fxamacker/cbor/v2` — CBOR codec
- `github.com/vmihailenco/msgpack/v5` — Msgpack codec

### Running Tests

```bash
go test ./...                    # All tests
go test -run TestMicron ./...    # Specific test
go test -bench=. ./...           # Benchmarks
```

## Status

Phase 1 (Core Library) and Phase 3 (CLI) are complete. Phase 2 (TUI) is
in progress with the framework, themes, and vendor ports done. Individual
display implementations are pending.

| Phase | Status |
|-------|--------|
| Core Library | ✅ Complete |
| TUI Framework | ✅ Complete |
| TUI Displays | 🚧 In Progress |
| CLI Entry Point | ✅ Complete |
| Integration Testing | 📋 Planned |

## License

Reticulum License — see [LICENSE](LICENSE) for details.
