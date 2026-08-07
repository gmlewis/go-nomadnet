// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Command view-mu renders micron markdown (.mu) files to stdout with ANSI
// colors for terminal display.
//
// Usage:
//
//	view-mu [options] file.mu
//
// Options:
//
//	-width int     Terminal width for rendering (default: auto-detect or 80)
//	-theme string  Color theme: "dark" or "light" (default: "dark")
//	-no-color      Disable ANSI color output
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"golang.org/x/term"
)

type config struct {
	width   int
	theme   string
	noColor bool
}

func main() {
	cfg := config{
		theme: "dark",
	}

	flag.IntVar(&cfg.width, "width", 0, "Terminal width for rendering (0 = auto-detect)")
	flag.StringVar(&cfg.theme, "theme", "dark", "Color theme: dark or light")
	flag.BoolVar(&cfg.noColor, "no-color", false, "Disable ANSI color output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] file.mu\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Renders micron markdown (.mu) files to stdout with ANSI colors.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	inputPath := args[0]

	// Read file
	content, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Auto-detect terminal width if not specified
	if cfg.width <= 0 {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			cfg.width = w
		} else {
			cfg.width = 80
		}
	}

	// Select theme
	theme := micron.ThemeDark
	if cfg.theme == "light" {
		theme = micron.ThemeLight
	}

	// Render
	output, err := renderToANSI(string(content), theme, cfg.width, cfg.noColor)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(output)
}

// renderToANSI converts micron markup to ANSI-colored text
func renderToANSI(markup string, theme micron.Theme, width int, noColor bool) (string, error) {
	lines := micron.RenderToStyledLines(markup, theme)
	var sb strings.Builder

	for _, line := range lines {
		if line == nil {
			sb.WriteByte('\n')
			continue
		}

		if line.Divider {
			ch := line.DividerChar
			if ch == "" {
				ch = "─"
			}
			n := max(1, width-line.Indent-line.DividerRight)
			sb.WriteString(strings.Repeat(" ", line.Indent))
			sb.WriteString(strings.Repeat(ch, n))
			sb.WriteByte('\n')
			continue
		}

		// Handle alignment
		if line.Align == micron.AlignCenter || line.Align == micron.AlignRight {
			textWidth := 0
			for _, span := range line.Spans {
				textWidth += len(span.Text)
			}
			avail := max(textWidth, width-line.Indent)
			pad := 0
			switch line.Align {
			case micron.AlignCenter:
				pad = (avail - textWidth) / 2
			case micron.AlignRight:
				pad = avail - textWidth
			}
			if pad > 0 {
				sb.WriteString(strings.Repeat(" ", pad))
			}
		}

		// Write indent
		if line.Indent > 0 {
			sb.WriteString(strings.Repeat(" ", line.Indent))
		}

		// Write spans with line wrapping
		var lastFG, lastBG string
		var lastBold, lastUnderline, lastItalic bool
		colPos := line.Indent

		for _, span := range line.Spans {
			runes := []rune(span.Text)
			for len(runes) > 0 {
				// Check if we need to wrap
				remaining := width - colPos
				if remaining <= 0 {
					// Wrap to next line
					if !noColor {
						sb.WriteString("\033[0m")
					}
					sb.WriteByte('\n')
					if line.Indent > 0 {
						sb.WriteString(strings.Repeat(" ", line.Indent))
					}
					colPos = line.Indent
					// Re-apply current style after wrap
					if !noColor {
						writeANSICode(&sb, lastFG, lastBG, lastBold, lastUnderline, lastItalic)
					}
				}

				// Determine how many runes fit on this line
				toWrite := min(len(runes), remaining)

				if noColor {
					sb.WriteString(string(runes[:toWrite]))
				} else {
					// Check if style changed
					styleChanged := span.FG != lastFG || span.BG != lastBG ||
						span.Bold != lastBold || span.Underline != lastUnderline || span.Italic != lastItalic

					if styleChanged {
						writeANSICode(&sb, span.FG, span.BG, span.Bold, span.Underline, span.Italic)
						lastFG = span.FG
						lastBG = span.BG
						lastBold = span.Bold
						lastUnderline = span.Underline
						lastItalic = span.Italic
					}

					sb.WriteString(string(runes[:toWrite]))
				}

				colPos += toWrite
				runes = runes[toWrite:]
			}
		}

		// Reset at end of line
		if !noColor {
			sb.WriteString("\033[0m")
		}
		sb.WriteByte('\n')
	}

	return sb.String(), nil
}

// writeANSICode writes an ANSI escape code for the given style
func writeANSICode(sb *strings.Builder, fg, bg string, bold, underline, italic bool) {
	var codes []string

	// Reset first
	codes = append(codes, "0")

	// Attributes
	if bold {
		codes = append(codes, "1")
	}
	if underline {
		codes = append(codes, "4")
	}
	if italic {
		codes = append(codes, "3")
	}

	// Foreground color
	if fg != "" && fg != "default" {
		if rgb := parseHexColor(fg); rgb != "" {
			codes = append(codes, "38;2;"+rgb)
		}
	}
	// Background color
	if bg != "" && bg != "default" {
		if rgb := parseHexColor(bg); rgb != "" {
			codes = append(codes, "48;2;"+rgb)
		}
	}

	sb.WriteString("\033[" + strings.Join(codes, ";") + "m")
}

// parseHexColor converts "#rrggbb" to "r;g;b" format for ANSI 24-bit color
func parseHexColor(color string) string {
	if len(color) != 7 || color[0] != '#' {
		return ""
	}

	r, err1 := strconv.ParseUint(color[1:3], 16, 8)
	g, err2 := strconv.ParseUint(color[3:5], 16, 8)
	b, err3 := strconv.ParseUint(color[5:7], 16, 8)

	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}

	return fmt.Sprintf("%d;%d;%d", r, g, b)
}
