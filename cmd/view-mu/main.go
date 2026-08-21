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

// Command view-mu renders micron markdown (.mu) to stdout with ANSI 24-bit
// colors for terminal display.
//
// The single positional argument is either a local .mu file or a remote
// nomadnet node address. When it is an address, view-mu connects to the node's
// "nomadnetwork.node" destination over Reticulum, fetches the requested page
// (the node's home page /page/index.mu by default), and renders the returned
// micron markup to stdout — the same fetch path the nomadnet browser uses.
//
// Usage:
//
//	view-mu [options] <file.mu | node-address>
//
// A node address is the 32-hex destination hash of a node's
// nomadnetwork.node destination, bare or with any of the common nomadnet
// prefixes, and an optional page path:
//
//	view-mu c388d720f56483a8dc8668ee5bea3577
//	view-mu lxm:c388d720f56483a8dc8668ee5bea3577
//	view-mu nomadnetwork://c388d720f56483a8dc8668ee5bea3577
//	view-mu c388d720f56483a8dc8668ee5bea3577:/page/conversations.mu
//	view-mu nomadnetwork://c388d720f56483a8dc8668ee5bea3577/page/conversations.mu
//
// When the argument names an existing file it is rendered directly; otherwise it
// is parsed as a node address. Status and diagnostics go to stderr so the
// rendered page on stdout can be piped (e.g. `view-mu <hash> | less -R`).
//
// Options:
//
//	-width int       Terminal width for rendering (default: auto-detect or 80)
//	-theme string    Color theme: "dark" or "light" (default: "dark")
//	-no-color        Disable ANSI color output
//	-raw             Write the raw micron markup bytes to stdout (no rendering)
//	-json            Emit per-line styled JSON (logical-line parity schema)
//	-rnsconfig DIR   Reticulum config dir (default: ~/.reticulum)
//	-timeout SECS    Seconds to wait for path/link/request (default 25)
//	-v               Verbose RNS logging
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/browser"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/go-reticulum/rns"
	"golang.org/x/term"
)

type config struct {
	width     int
	theme     string
	noColor   bool
	rnsConfig string
	timeout   float64
	verbose   bool
	raw       bool
	jsonOut   bool
}

// remotePrefixes are stripped (case-insensitively) from the start of an
// address before it is handed to browser.ParseURL. They cover the common
// nomadnet/LXMF spellings. Longest first so "lxmf://lxmf@hash" sheds the full
// scheme rather than just "lxmf://".
var remotePrefixes = []string{
	"lxmf://lxmf@",
	"nomadnetwork://",
	"lxmf://",
	"node://",
	"lxmf:",
	"lxmf@",
	"lxm:",
}

func main() {
	// Status and diagnostics go to stderr via the log package (no date prefix,
	// since this is a one-shot CLI whose rendered page is on stdout).
	log.SetFlags(0)

	cfg := config{
		theme:   "dark",
		timeout: 25,
	}

	flag.IntVar(&cfg.width, "width", 0, "Terminal width for rendering (0 = auto-detect)")
	flag.StringVar(&cfg.theme, "theme", "dark", "Color theme: dark or light")
	flag.BoolVar(&cfg.noColor, "no-color", false, "Disable ANSI color output")
	flag.StringVar(&cfg.rnsConfig, "rnsconfig", "", "Reticulum config dir (default: ~/.reticulum)")
	flag.Float64Var(&cfg.timeout, "timeout", 25, "Seconds to wait for path/link/request")
	flag.BoolVar(&cfg.verbose, "v", false, "Verbose RNS logging")
	flag.BoolVar(&cfg.raw, "raw", false, "Write the raw micron markup bytes to stdout (no rendering)")
	flag.BoolVar(&cfg.jsonOut, "json", false, "Emit per-line styled JSON (logical-line parity schema) instead of ANSI")

	flag.Usage = func() {
		log.Printf("Usage: %s [options] <file.mu | node-address>\n", os.Args[0])
		log.Printf("Renders micron markdown (.mu) to stdout with ANSI colors.")
		log.Printf("The argument is a local .mu file, or a nomadnet node address")
		log.Printf("(32-hex destination hash, bare or prefixed, with an optional path).")
		log.Printf("Options:")
		flag.PrintDefaults()
	}

	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
		os.Exit(2)
	}

	// Auto-detect terminal width if not specified.
	if cfg.width <= 0 {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			cfg.width = w
		} else {
			cfg.width = 80
		}
	}

	theme := micron.ThemeDark
	if cfg.theme == "light" {
		theme = micron.ThemeLight
	}

	arg := args[0]

	// Gather the raw micron markup bytes, either from a local file or by
	// fetching a remote node page. The bytes are then either dumped raw
	// (-raw), rendered to structured JSON (-json), or rendered to ANSI
	// (default).
	var markup []byte
	var source string
	if info, err := os.Stat(arg); err == nil && !info.IsDir() {
		content, err := os.ReadFile(arg)
		if err != nil {
			log.Printf("Error reading file: %v", err)
			os.Exit(1)
		}
		markup = content
		source = arg
	} else {
		destHash, path, requestData, display, err := parseRemoteAddress(arg)
		if err != nil {
			log.Printf("Error: %q is not an existing file and not a valid node address: %v", arg, err)
			os.Exit(2)
		}
		markup = fetchRemote(cfg, destHash, path, requestData, display)
		source = display
	}

	if cfg.raw {
		if _, err := os.Stdout.Write(markup); err != nil {
			log.Printf("Error writing output: %v", err)
			os.Exit(1)
		}
		return
	}

	if cfg.jsonOut {
		if err := renderToJSON(os.Stdout, markup, theme, source); err != nil {
			log.Printf("Error rendering JSON: %v", err)
			os.Exit(1)
		}
		return
	}

	render(string(markup), theme, cfg.width, cfg.noColor)
}

// render renders micron markup to stdout, exiting the process on error.
func render(markup string, theme micron.Theme, width int, noColor bool) {
	output, err := renderToANSI(markup, theme, width, noColor)
	if err != nil {
		log.Printf("Error rendering: %v", err)
		os.Exit(1)
	}
	fmt.Print(output)
}

// renderToJSON emits the logical-line parity schema: one entry per micron source
// line with its styled spans (including link/field targets, which the ANSI path
// drops) and layout metadata. It is pre-wrap — both view-mu and the gonomadnet
// browser share micron.RenderToStyledLines for segmentation, so this is faithful
// to the browser's logical-line structure. Wrapping/layout parity is compared
// separately by the live loopback comparator, not here.
func renderToJSON(w io.Writer, markup []byte, theme micron.Theme, source string) error {
	lines := micron.RenderToStyledLines(string(markup), theme)
	doc := jsonDoc{Source: source, Theme: themeName(theme), Lines: make([]jsonLine, 0, len(lines))}
	for i, line := range lines {
		jl := jsonLine{Index: i, Align: alignName(micron.AlignLeft), Spans: []jsonSpan{}}
		if line != nil {
			jl = jsonLine{
				Index:        i,
				Align:        alignName(line.Align),
				Indent:       line.Indent,
				HeadingLevel: line.HeadingLevel,
				Divider:      line.Divider,
				DividerChar:  line.DividerChar,
				DividerRight: line.DividerRight,
				Anchor:       line.Anchor,
				Spans:        make([]jsonSpan, 0, len(line.Spans)),
			}
			for _, sp := range line.Spans {
				js := jsonSpan{
					Text:      sp.Text,
					FG:        sp.FG,
					BG:        sp.BG,
					Bold:      sp.Bold,
					Underline: sp.Underline,
					Italic:    sp.Italic,
				}
				if sp.Link != nil {
					js.Link = &jsonLink{Label: sp.Link.Label, URL: sp.Link.URL, Fields: sp.Link.Fields}
				}
				if sp.Field != nil {
					js.Field = &jsonField{
						Name:       sp.Field.Name,
						Type:       sp.Field.Type,
						Data:       sp.Field.Data,
						Value:      sp.Field.Value,
						Width:      sp.Field.Width,
						Masked:     sp.Field.Masked,
						Prechecked: sp.Field.Prechecked,
					}
				}
				jl.Spans = append(jl.Spans, js)
			}
		}
		doc.Lines = append(doc.Lines, jl)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// jsonDoc is the top-level parity schema document.
type jsonDoc struct {
	Source string     `json:"source"`
	Theme  string     `json:"theme"`
	Lines  []jsonLine `json:"lines"`
}

type jsonLine struct {
	Index        int        `json:"index"`
	Align        string     `json:"align"`
	Indent       int        `json:"indent"`
	HeadingLevel int        `json:"heading_level"`
	Divider      bool       `json:"divider"`
	DividerChar  string     `json:"divider_char,omitempty"`
	DividerRight int        `json:"divider_right,omitempty"`
	Anchor       string     `json:"anchor,omitempty"`
	Spans        []jsonSpan `json:"spans"`
}

type jsonSpan struct {
	Text      string     `json:"text"`
	FG        string     `json:"fg"`
	BG        string     `json:"bg"`
	Bold      bool       `json:"bold"`
	Underline bool       `json:"underline"`
	Italic    bool       `json:"italic"`
	Link      *jsonLink  `json:"link,omitempty"`
	Field     *jsonField `json:"field,omitempty"`
}

type jsonLink struct {
	Label  string `json:"label"`
	URL    string `json:"url"`
	Fields string `json:"fields"`
}

type jsonField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Data       string `json:"data"`
	Value      string `json:"value"`
	Width      int    `json:"width"`
	Masked     bool   `json:"masked"`
	Prechecked bool   `json:"prechecked"`
}

func themeName(t micron.Theme) string {
	if t == micron.ThemeLight {
		return "light"
	}
	return "dark"
}

func alignName(a micron.Alignment) string {
	switch a {
	case micron.AlignCenter:
		return "center"
	case micron.AlignRight:
		return "right"
	default:
		return "left"
	}
}

// fetchRemote initializes Reticulum, fetches path from the nomadnetwork.node
// destination destHash, and returns the raw micron bytes. Status is written to
// stderr. It exits the process on failure.
func fetchRemote(cfg config, destHash []byte, path string, requestData map[string]string, display string) []byte {
	logger := rns.NewLogger()
	level := rns.LogError
	if cfg.verbose {
		level = rns.LogInfo
	}
	// RNS logs default to stdout, which would corrupt the rendered page on
	// stdout. Route every log line to stderr via a callback instead.
	logger.SetLogCallback(func(msg string) { log.Print(msg) })
	logger.SetLogDest(rns.LogCallback)
	logger.SetLogLevel(level)

	ts := rns.NewTransportSystem(logger)
	ret, err := rns.NewReticulumWithLogger(ts, cfg.rnsConfig, logger)
	if err != nil {
		log.Printf("Could not initialize Reticulum: %v", err)
		os.Exit(1)
	}
	// applyConfig may have reset the level from the config file's [logging]
	// section; re-assert the caller's choice after initialization.
	logger.SetLogLevel(level)
	defer func() {
		if err := ret.Close(); err != nil {
			logger.Warning("Could not close Reticulum properly: %v", err)
		}
	}()

	log.Printf("view-mu — go-reticulum %v", rns.VERSION)
	if dir := ret.ConfigDir(); dir != "" {
		log.Printf("RNS config: %s", dir)
	}
	log.Printf("Fetching %s from %s (timeout %.0fs)", path, display, cfg.timeout)

	// Give AutoInterface discovery a moment to bring interfaces up before the
	// fetch starts issuing path requests.
	time.Sleep(1500 * time.Millisecond)

	timeout := time.Duration(cfg.timeout * float64(time.Second))
	ctx := context.Background()
	data, err := browser.FetchPage(ctx, ts, destHash, path, requestData, timeout, nil, nil)
	if err != nil {
		log.Printf("Fetch failed: %v", err)
		os.Exit(1)
	}
	log.Printf("Received %d bytes.", len(data))
	return data
}

// parseRemoteAddress parses a nomadnet node address into the destination hash,
// page path, and request-data map. It accepts a bare 32-hex hash, any of the
// common nomadnet/LXMF prefixes, and an optional path in either colon
// ("hash:/path") or slash ("nomadnetwork://hash/path") form. The friendly
// return is a "hex:/path" display string for diagnostics.
func parseRemoteAddress(arg string) (destHash []byte, path string, requestData map[string]string, friendly string, err error) {
	s := strings.TrimSpace(arg)

	// Strip a known scheme/address prefix (longest match wins).
	for _, p := range remotePrefixes {
		if len(s) > len(p) && strings.EqualFold(s[:len(p)], p) {
			s = s[len(p):]
			break
		}
	}

	// Convert the slash form ("hash/path") into the colon form ("hash:/path")
	// that browser.ParseURL expects, but only when the leading 32 chars are the
	// hex hash and the next char is '/'. The '/' is kept as the path's leading
	// slash (a path itself contains '/', so only the separator right after the
	// hash is rewritten).
	if len(s) >= hexHashLen+1 && isHexRun(s[:hexHashLen]) && s[hexHashLen] == '/' {
		s = s[:hexHashLen] + ":" + s[hexHashLen:]
	}

	// Allow "hash:path|x=1" to mean "hash:path`x=1" without typing a backtick.
	s = browser.NormalizeEnteredURL(s)

	destHash, path, requestData, err = browser.ParseURL(s, nil, requestData)
	if err != nil {
		return nil, "", nil, "", err
	}
	friendly = hex.EncodeToString(destHash) + path
	return destHash, path, requestData, friendly, nil
}

const hexHashLen = rns.TruncatedHashLength / 4 // 32 hex chars

// isHexRun reports whether s is a non-empty run of hex digits.
func isHexRun(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !isHexByte(byte(c)) {
			return false
		}
	}
	return true
}

func isHexByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
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

		// Heading lines: Python wraps the heading in urwid.AttrMap(...,
		// heading_style) (MicronParser.py:318), whose fallback attribute fills
		// the ENTIRE row — the left-indent spaces, the heading text, and the
		// right padding — with the heading background. The general path below
		// only colors the text characters, so the highlight stops after the
		// last letter. Reproduce the full-width highlight here so this
		// offline renderer matches the browser (tui.StyledLinesToTviewText)
		// and the Python source of truth.
		if line.HeadingLevel > 0 && len(line.Spans) > 0 {
			base := line.Spans[0]
			hfg, hbg := base.FG, base.BG
			if !noColor {
				writeANSICode(&sb, hfg, hbg, false, false, false)
			}
			sb.WriteString(strings.Repeat(" ", line.Indent))
			colPos := line.Indent
			var lastFG, lastBG string
			var lastBold, lastUnderline, lastItalic bool
			for _, span := range line.Spans {
				runes := []rune(span.Text)
				for len(runes) > 0 {
					remaining := width - colPos
					if remaining <= 0 {
						if !noColor {
							sb.WriteString("\033[0m")
						}
						sb.WriteByte('\n')
						if !noColor {
							writeANSICode(&sb, hfg, hbg, false, false, false)
						}
						sb.WriteString(strings.Repeat(" ", line.Indent))
						colPos = line.Indent
						lastFG, lastBG = "", ""
						lastBold, lastUnderline, lastItalic = false, false, false
						remaining = width - colPos
					}
					toWrite := min(len(runes), remaining)
					if !noColor {
						styleChanged := span.FG != lastFG || span.BG != lastBG ||
							span.Bold != lastBold || span.Underline != lastUnderline || span.Italic != lastItalic
						if styleChanged {
							writeANSICode(&sb, span.FG, span.BG, span.Bold, span.Underline, span.Italic)
							lastFG, lastBG = span.FG, span.BG
							lastBold, lastUnderline, lastItalic = span.Bold, span.Underline, span.Italic
						}
					}
					sb.WriteString(string(runes[:toWrite]))
					colPos += toWrite
					runes = runes[toWrite:]
				}
			}
			// Pad the final row to the full width with the heading background.
			if pad := width - colPos; pad > 0 {
				if !noColor {
					writeANSICode(&sb, hfg, hbg, false, false, false)
				}
				sb.WriteString(strings.Repeat(" ", pad))
			}
			if !noColor {
				sb.WriteString("\033[0m")
			}
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

	sb.WriteString("\033[")
	sb.WriteString(strings.Join(codes, ";"))
	sb.WriteString("m")
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
