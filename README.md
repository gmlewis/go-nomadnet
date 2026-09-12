# gonomadnet — Go Nomad Network Client <a href="https://github.com/gmlewis/go-nomadnet/actions/workflows/build.yml"><img align="right" src="https://github.com/gmlewis/go-nomadnet/actions/workflows/build.yml/badge.svg"/></a>

![gonomadnet mascot
The Go gopher was designed by Renee French.
The design is licensed under the Creative Commons 4.0 Attribution license.](assets/gonomadnet-mascot.png)

A complete Go port of [NomadNet](https://github.com/markqvist/nomadnet), a
peer-to-peer messaging and information sharing system built on
[Reticulum](https://reticulum.network). NomadNet enables private, encrypted
communication over any network transport — including LoRa, packet radio, and
the internet.

> [!TIP]
> ### Building a Dedicated Handheld NomadNet Device?
> Jump straight to the [**Reticulum Hardware Projects Guide**](https://github.com/gmlewis/asic-reticulum/tree/master/Hardware-Projects-Guide.md) for full step-by-step assembly instructions, hardware bills of materials, pre-compiled release binaries, and zero-install in-browser web flashing.
>
> Learn how to build and flash:
> - **Project 1: Pocket Linux Terminal**: Full interactive `gonomadnet` TUI on Raspberry Pi Zero 2W with a 2.8" SPI display and CardKB keyboard.
> - **Project 2: Pocket Communicator**: Ultra-low-power handheld communicator running on ESP32-C5 RISC-V SoC.
> - **Project 3: Pocket Hub & Repeater**: Autonomous standalone relay node with LoRa and Wi-Fi 6 SoftAP.

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
go install github.com/gmlewis/go-nomadnet/cmd/gonomadnet@v0.129.0
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

## The gonomadnet Public Hub

A public node runs the client, the node, and the demos from this repository, so
you can browse real `.wasm` executable pages without installing anything on your
own machine first:

- **Hub**: `go-nomadnet.duckdns.org` — a public RNS TCP gateway
- **Endpoint**: `go-nomadnet.duckdns.org:4242`

Add it to your Reticulum client config (`~/.reticulum/config`):

```
[[gonomadnet Public Hub]]
  type = TCPClientInterface
  interface_enabled = yes
  target_host = go-nomadnet.duckdns.org
  target_port = 4242
```

Then start `gonomadnet`, wait for the hub to announce, and open it from the
Nodes list — or type its destination hash into the URL bar (`Ctrl-U`):
`c7d0e7bbd883e595f53e14fa6986188c`. Its index page links to the executable
pages: the guestbook, the hit counter, and the other demos described under
[Wasm Executable Pages](#wasm-executable-pages).

The hub also hosts a public RRC room, `#general`, for the Go ports —
`rrc://a012129c10205c0b9441fcd2b755b2a7/#general` — and `rns://` mirrors of the
source, so the repositories can be cloned over Reticulum as well as from GitHub.

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

## Wasm Executable Pages

Any `.wasm` file placed in the node's pages directory is an **executable
page**: requests for it render through a sandboxed in-process WebAssembly
runtime (the [wago](https://github.com/wago-org/wago) engine, compiled in with
`-tags wago`) instead of being served statically. This mirrors Python
NomadNet's executable pages, which ran a page as a subprocess with the request
data as environment variables — the Go sandbox replaces that subprocess with a
deny-by-default wasm ABI.

- **Install**: copy a page's `.wasm` file into `<config-dir>/storage/pages/`
  (default `~/.nomadnetwork/storage/pages/`) and request it — no restart
  needed, since every request runs a fresh instance.
- **ABI**: the plugin exports `render_page(req_ptr, req_len) -> (ptr, len)`.
  The request payload is a JSON object (`path`, `request_data`, `link_id`,
  `remote_identity`, `requested_at`) and the response bytes are Micron markup.
- **Host capabilities**: a page may import `rns.kv_get` and `rns.kv_set` to
  keep state in its own key/value store, and `rns.log` to log a message to the
  node. Every other import is still refused at instantiation, so the
  deny-by-default policy holds as capabilities are added.
- **Limits**: 16 MiB linear memory, 1024 table entries, and a 2-second
  execution budget per request; a failing or runaway plugin renders as
  not-found instead of leaking its binary source.
- **Build tag**: binaries built without `-tags wago` serve `.wasm` files
  statically (the release builder adds the tag on supported platforms).

### Host capabilities and page state

`rns.kv_set(key_ptr, key_len, val_ptr, val_len) -> status` stores a value
(0 = ok, 1 = error) and `rns.kv_get(key_ptr, key_len, out_ptr, out_cap) ->
n` reads one (n = bytes written, 0 = no such key, -1 = buffer too small, in
which case nothing is written). Keys live under the page's own directory,
`<pages-path>/data/<page>/`, so two pages on one node never share state. The
store is capped at 10 MiB per page by the same rules the RRC plugin store
uses.

Each render still compiles, runs, and releases a fresh instance, so on-disk
state is the only thing that survives a request: a page that wants a counter
must read it, increment it, and write it back.

### Request data and forms

Requests carry `request_data` (the `field_*`/`var_*` values a form
submission collected) exactly as Python NomadNet passes an executable page's
environment map. A page can therefore declare a Micron form and read what the
visitor submitted:

```
Name: `B444`<name`>`b
Message: `B444`<32|message`>`b
`[Sign the guestbook`:/page/guestbook.wasm`name|message]
```

Micron spells a field `` `<NAME`VALUE> ``, and the submit link's third segment
names the fields to collect. On submit, the collected fields arrive in the
request payload as `field_name` and `field_message`, so a page scans its
request JSON for those keys. (The leading-colon URL above is relative to the
node being browsed, so the page works on any node that serves it.)

**`VALUE` is the field's pre-defined content, not a hint or a label.** Typing
appends to it, so a field written `` `<name`Your name> `` shows
`Your nameGlenn` once a visitor types their name — in Python NomadNet and
gonomadnet alike. The markup guide calls it "Pre-defined data", and a
pre-filled field is how a page offers a default value to keep or edit. Put
labels in the surrounding text and leave the field empty, as the example above
does.

An empty field is still **visible** because a text field occupies its whole
declared width (24 columns by default, or the number in the flags slot) and
paints its background across it — which is what the `B444` tag in the example
is for. The same rule means text following a field on the same line starts
past the field's width, exactly as Python's `urwid.Columns` lays it out.

**Editing a field.** A field is a readline-style editor, so the familiar keys
work while it is selected: `Ctrl-A` / `Ctrl-E` go to the start / end of the
line, `Ctrl-U` clears from the cursor back to the start, `Ctrl-K` clears from
the cursor to the end, `Ctrl-W` deletes the previous word, `Ctrl-L` clears the
whole field, and `Ctrl-Y` pastes back what you last cleared. `Down` / `Up`
(and `Tab`) move between fields, and `Enter` on the submit link posts them.
These keys reach the field because a field in edit mode is offered each key
before any page shortcut is — the priority Python's urwid gives the focused
widget. The shipped guestbook demo (`assets/wasm-pages/guestbook.wat`) opens
the page with its form, so a visitor never has to scroll past the whole history
to sign it, prints these keys as a tip under the fields, counts its own visits
on a line below that tip, and puts the entries below a divider, newest first.

**`request_data` is attacker-controlled and the node does not sanitize it.**
A page that re-emits it can inject Micron markup — links, formatting modes,
headings — into another visitor's view. Validate it in the page before
storing or rendering it, and treat every page's output as untrusted markup
when reviewing third-party pages.

### Inline hit counters with partials

A page can also render a `.wasm` module *inside itself* rather than linking to
it, by declaring a Micron **partial**:

```
`{<node-hash>:/page/hit-counter.wasm`0`page=index.mu}
```

A partial is `` `{URL`REFRESH`FIELDS} ``. It is fetched when the page loads and
its reply is spliced in where the directive sits, so the counter above appears
in the middle of the page it belongs to. Two details make it a per-page
counter rather than one global number:

- **`FIELDS` may carry literal values.** A `k=v` entry is sent as
  `request_data`'s `var_k=v`, so `` `page=index.mu `` tells the module which
  page is counting itself. (An entry without `=` names a form field to collect
  instead, which is how a partial can re-submit a page's inputs.)
- **`REFRESH` below one second means "load once".** The count then advances
  when a visitor arrives, not on a timer. One second or more re-fetches on that
  interval; `p:<id>` links in the page force a refresh on demand, matching
  Python NomadNet's partials.

Any page that embeds the partial under its own name gets its own independent
count, so copying the section into another page never disturbs the first. Drop
the `page=` field entirely and the module falls back to its own default `hits`
key, which counts every visit to the node instead — one number for the whole
site, the same value you get by opening the module directly.

The default index page ships both, so a node's home page shows its own count and
the site total side by side — see `nomadnet/app/default-index.mu`.

A partial is fetched over a normal Reticulum link, so a node that copies the
section needs no `.wasm` support of its own: the count is computed wherever the
partial's URL points. That cuts both ways — the default index page addresses the
hub by its absolute URL, so a node that copies that section shares the hub's
count. To keep a counter that is yours alone, point the partial at your own
node's hash and install `hit-counter.wasm` there:

```
`{<your-node-hash>:/page/hit-counter.wasm`0`page=index.mu}
```

When the URL points back at the node doing the browsing, the partial is served
straight from that node's pages directory, because a node cannot open an RNS
link to itself.

Ready-to-run examples with install instructions ship in
[`assets/wasm-pages/`](assets/wasm-pages/). Each is a `.wat` (WebAssembly text
source) with its assembled `.wasm` beside it:

| Page | What it demonstrates |
|------|----------------------|
| `dynamic-page.wat` | Echoes the request payload — proves pages are dynamic |
| `hit-counter.wat` | Reads, increments, and stores a visit counter, per page or site-wide, visibly or quietly (`rns.kv_*`) |
| `guestbook.wat` | Reads a submitted Micron form, stores and lists entries newest-first, and counts its own visits |

Build and install one with:

```bash
wat2wasm guestbook.wat -o guestbook.wasm
cp guestbook.wasm ~/.nomadnetwork/storage/pages/
```

Then browse the node (loopback or remote) and request
`/page/guestbook.wasm`. To put a counter on a page of your own, install
`hit-counter.wasm` and add one line to that page's markup:

```
This page has been viewed:

`{<your-node-hash>:/page/hit-counter.wasm`0`page=<that-page-name>}
```

A counter can also be triggered **without being displayed**. Add a `quiet`
field with any value and the module counts the visit and renders nothing at
all, so the partial substitutes to no text at the place you embedded it:

```
`{<your-node-hash>:/page/hit-counter.wasm`0`quiet=1}
```

With no `page` field that bumps the node's site-wide total, which is how the
guestbook demo adds its visitors to the same counter the index page prints
while showing only its own count: a page's KV store is private to it, so a
partial is the only way one page can reach another's counter. Install
`hit-counter.wasm` beside any page that does this — a node missing it shows the
client's partial-load error where the invisible partial was.

Validate and inspect plugins with the `wago` CLI (`wago validate`,
`wago module imports`).

### Page plugin ideas

Some things executable pages are designed to make safe and easy:

- Hit counters and "last browsed" markers, inline in the page via a partial
- Guestbooks and form processors that append submissions to local files
- Live status dashboards rendering Micron tables from local state
- Random-tip or quote-of-the-day generators
- Interactive calculators and converters driven by `request_data` fields

### Security considerations for public nodes

Read this before enabling the node on an internet-facing interface (for
example a public TCP server):

- **Only the operator can install pages.** There is no remote upload path:
  peers fetch pages and files, they cannot write to the pages directory or
  plant a `.wasm` file. The realistic threat is supply-chain — only serve
  `.wasm` pages you built or reviewed, since they run on your node and their
  responses carry your node's identity.
- **Anyone who can reach the node can execute a page.** Pages are served with
  the broadest access policy; a `.allowed` file placed next to a `.wasm` page
  restricts who may execute it (the check runs *before* the sandbox), and the
  sandbox itself caps every request at 16 MiB of memory and 2 seconds of
  execution. Public-node page plugins should stay trivial — each request
  compiles a fresh instance, so heavy pages are a CPU-exhaustion vector.
- **Page plugins get no network and no filesystem.** The only imports they
  can use are `rns.log` and `rns.kv_get`/`rns.kv_set`, whose keys are scoped
  to the page's own `data/<page>/` directory; everything else is refused at
  instantiation. A page can compute markup from its own logic, the request
  metadata (the requester's own link ID and identity hash), and whatever it
  has stored for itself.
- **Page state is node-local and unauthenticated.** Anyone who can reach the
  page can call its store through the page's own logic, so a public page that
  appends entries can have its store filled from the outside; the shipped
  examples cap each value and reject Micron markup, but a page that does not
  is a spamming target rather than a code-execution risk.
- **A page's markup is rendered by visitors' clients.** Micron/terminal
  formatting in the response is interpreted by the browsing TUI, so a page
  that reflects user-controlled data (its own metadata, or `request_data`
  form fields, which now reach pages) can inject links or markup into another
  user's view. Escape or strip user-controlled data before returning it, and
  treat every page's output as untrusted markup when reviewing third-party
  pages.

### Timestamps in pages

A page that shows a clock time should store the instant, not the spelling.
Unix seconds carry no timezone, and only the client that parses the page knows
which timezone the person reading it is in — and pages *are* parsed by the
client. So a `.wasm` page emits

```
`T1789178907|%a %b %d, %Y %-I:%M:%S%p %Z`T
```

and each reader's own client renders those seconds in that reader's own
timezone. The format language is
[strftime](https://docs.python.org/3/library/time.html#time.strftime), the
POSIX/C spelling shared by Python's `time.strftime`, the shell's `date` and
most other tooling, so `%Y-%m-%d %H:%M` needs no lookup. Leaving the format out
uses the default `%a %b %d, %Y %-I:%M:%S%p %Z` — `Fri Sep 11, 2026 9:08:27PM
EST` — and `%-` drops the padding of a numeric conversion. The `%q`-style
escape hatch is deliberate: an unsupported conversion passes through with its
`%`, so a typo in a page's format stays visible instead of vanishing.

This is a gonomadnet extension. Python's NomadNet has no timestamp construct:
its parser consumes the marker character and renders the payload as plain text,
so a reader on the original client sees the raw seconds where gonomadnet shows
a local time. Because pages are parsed client-side, a page cannot localize a
time for its reader by any other means; a page that must show every client the
same readable stamp should render that stamp itself.

The shipped guestbook (`assets/wasm-pages/guestbook.wat`) stamps each entry
with the request's own `requested_at` seconds, stores them alongside the
entry, and renders the construct — see that file's `TIMESTAMPS` section.

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

## Status

Although this port seems to be fully functional, there may still be bugs.
If you find bugs, please report them as new
[GitHub Issues](https://github.com/gmlewis/go-nomadnet/issues).

## License

GNU General Public License v3 — see [LICENSE](LICENSE) for details.
