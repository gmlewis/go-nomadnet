# view-mu

A Go tool that renders micron markdown (`.mu`) files to stdout with ANSI 24-bit color support for terminal display.

## Overview

This tool uses the nomadnet micron parser to render `.mu` files with proper colors, formatting, and layout. It outputs ANSI escape codes for truecolor (24-bit) terminal support.

## Usage

```bash
view-mu [options] file.mu
```

### Options

| Option | Description |
|--------|-------------|
| `-width int` | Terminal width for rendering (0 = auto-detect) |
| `-theme string` | Color theme: `dark` or `light` (default: `dark`) |
| `-no-color` | Disable ANSI color output (plain text) |

## Examples

### Render with auto-detected terminal width

```bash
./view-mu page.mu
```

### Render with specific width

```bash
./view-mu -width 80 page.mu
```

### Use light theme

```bash
./view-mu -theme light page.mu
```

### Disable colors (plain text output)

```bash
./view-mu -no-color page.mu
```

### Pipe to less for scrolling

```bash
./view-mu page.mu | less -R
```

## Building

```bash
cd tooling/view-mu
go build -o view-mu .
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

## Viewing Generated ASCII Art

To view PNG images converted to micron format:

```bash
# First convert a PNG
./tooling/png-to-mu/png-to-mu -width 60 image.png image.mu

# Then view it
./tooling/view-mu/view-mu image.mu
```

## License

GPLv3 - See the main project LICENSE file.
