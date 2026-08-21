# png-to-mu

A Go tool that converts PNG images to 24-bit color micron markdown (`.mu`) files using ASCII art characters.

## Overview

This tool renders PNG images as ASCII art using block characters with full 24-bit color support via micron's `FT` (foreground) and `BT` (background) color tags. The output can be displayed in any application that supports micron markup, including the NomadNet browser renderer.

## Usage

```bash
png-to-mu [options] input.png [output.mu]
```

### Options

| Option | Description |
|--------|-------------|
| `-width int` | Maximum output width in characters (0 = auto) |
| `-height int` | Maximum output height in characters (0 = auto) |
| `-scale float` | Scale factor for output (default: 1.0) |
| `-char-set string` | Character set to use: `blocks`, `simple`, `detailed` (default: `detailed`) |
| `-half-height` | Use half-height mode (2 pixels per char vertically) |
| `-output string` | Output file (empty = stdout) |
| `-no-comments` | Omit comment header from output |

### Character Sets

- **detailed** (default) - Uses Braille patterns (⣿⣀⡀⣰) for 4 pixels per character vertically, providing the highest detail
- **blocks** - Uses half-block characters (▄▀█░) for 2 pixels per character vertically
- **simple** - Uses gradient characters (░▒▓█) for 1 pixel per character

## Examples

### Convert with default settings (detailed Braille)

```bash
./png-to-mu image.png image.mu
```

### Convert with specific width

```bash
./png-to-mu -width 80 image.png image.mu
```

### Convert using block characters

```bash
./png-to-mu -char-set blocks -width 60 image.png
```

### Convert with scale factor

```bash
./png-to-mu -scale 0.5 image.png output.mu
```

### Output to stdout

```bash
./png-to-mu -width 40 image.png -
```

## Output Format

The output is a micron markdown file that uses 24-bit color codes:

```
# ASCII art generated from PNG image
# Original: image.png
# Dimensions: 80x60 characters
#

`FT000000   `FTffffff█`FT000000   ...
`FTrrggbb<char>`FTrrggbb<char>...
```

- `FTrrggbb` - Sets foreground color to 24-bit RGB value
- Characters are colored block/braille elements
- Color codes are optimized (only emitted when color changes)
- File ends with `f`b`= to reset formatting

## Building

```bash
go build -o png-to-mu ./cmd/png-to-mu
```

## Viewing Output

The generated `.mu` files can be viewed in:

1. **NomadNet Guide** - Place in the guide topics directory
2. **NomadNet Browser** - Load via the browser's page display
3. **Any micron renderer** - The output uses standard micron markup

## Technical Details

### Color Optimization

The tool minimizes output size by only emitting color codes when the color changes between adjacent characters. Consecutive pixels of the same color share a single color code.

### Character Mapping

- **Braille mode**: Each character represents a 2×4 pixel grid (8 dots)
- **Block mode**: Each character represents a 2×1 or 1×2 pixel area
- **Simple mode**: Each character represents a single pixel with density mapping

### Color Depth

Uses micron's 24-bit color format (`FTrrggbb`) for precise color matching, compatible with the Go micron renderer's `highColor` function.

## License

GPLv3 - See the main project LICENSE file.
