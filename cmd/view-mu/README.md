# view-mu

A Go tool that renders micron markdown (`.mu`) to stdout with ANSI 24-bit color support for terminal display.

## Overview

This tool uses the nomadnet micron parser to render `.mu` with proper colors, formatting, and layout, emitting ANSI escape codes for truecolor (24-bit) terminals.

It accepts either a **local `.mu` file** or a **remote nomadnet node address**. For a remote address it connects to the node's `nomadnetwork.node` destination over Reticulum, fetches the requested page (the node's home page `/page/index.mu` by default), and renders the returned micron markup to stdout — the same fetch path the nomadnet browser uses.

## Usage

```bash
view-mu [options] <file.mu | node-address>
```

When the argument names an existing file it is rendered directly; otherwise it is parsed as a node address. Status and diagnostics go to **stderr** so the rendered page on stdout can be piped (e.g. `view-mu <hash> | less -R`).

### Node addresses

A node address is the 32-hex destination hash of a node's `nomadnetwork.node` destination, bare or with any of the common nomadnet prefixes, and an optional page path in either colon or slash form:

```bash
view-mu c388d720f56483a8dc8668ee5bea3577
view-mu lxm:c388d720f56483a8dc8668ee5bea3577
view-mu nomadnetwork://c388d720f56483a8dc8668ee5bea3577
view-mu c388d720f56483a8dc8668ee5bea3577:/page/conversations.mu
view-mu nomadnetwork://c388d720f56483a8dc8668ee5bea3577/page/conversations.mu
```

A bare hash fetches `/page/index.mu` (the node's home page). Field data may be appended with `` ` `` or `|`:

```bash
view-mu c388d720f56483a8dc8668ee5bea3577:/page/search.mu`q=robots
view-mu c388d720f56483a8dc8668ee5bea3577:/page/search.mu|q=robots
```

### Options

| Option | Description |
|--------|-------------|
| `-width int` | Terminal width for rendering (0 = auto-detect) |
| `-theme string` | Color theme: `dark` or `light` (default: `dark`) |
| `-no-color` | Disable ANSI color output (plain text) |
| `-rnsconfig DIR` | Reticulum config dir (default: `~/.reticulum`) |
| `-timeout SECS` | Seconds to wait for path/link/request when fetching a remote page (default: 25) |
| `-v` | Verbose RNS logging (to stderr) |

## Examples

### Render a local file with auto-detected terminal width

```bash
go run ./cmd/view-mu page.mu
```

### Fetch and render a remote node's home page

```bash
go run ./cmd/view-mu c388d720f56483a8dc8668ee5bea3577
```

### Render with specific width / light theme / no color

```bash
go run ./cmd/view-mu -width 80 page.mu
go run ./cmd/view-mu -theme light c388d720f56483a8dc8668ee5bea3577
go run ./cmd/view-mu -no-color page.mu
```

### Pipe to less for scrolling

```bash
go run ./cmd/view-mu c388d720f56483a8dc8668ee5bea3577 | less -R
```

## Building

```bash
go build -o view-mu ./cmd/view-mu
```

## Technical Details

### Color Support

The tool outputs ANSI 24-bit color codes:
- Foreground: `\033[38;2;R;G;Bm`
- Background: `\033[48;2;R;G;Bm`

Requires a truecolor-capable terminal (most modern terminals support this).

### Theme Support

- **dark** (default) - Uses the dark micron palette (light text on dark background)
- **light** - Uses the light micron palette (dark text on light background)

The themes match the palettes used by the Python NomadNet micron renderer.

### Remote fetching

Remote fetches reuse `nomadnet/browser.FetchPage` (the Go port of Python `Browser.__load`): path resolution, identity recall, link establishment, and the page request all mirror what the nomadnet browser does. RNS log output is routed to stderr so it never corrupts the rendered page on stdout.

## Viewing Generated ASCII Art

To view PNG images converted to micron format:

```bash
# First convert a PNG
go run ./cmd/png-to-mu -width 60 image.png image.mu

# Then view it
go run ./cmd/view-mu image.mu
```

## License

GPLv3 - See the main project LICENSE file.