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

// Command windows-diagnostic inspects the terminal layer gonomadnet draws
// through and names the mechanism behind a garbled terminal.
//
// It exists because the same client can run cleanly in one terminal window and
// scribble over another, and the variables are invisible from inside the
// client: which tcell renderer was built, what the Windows console reports for
// its screen buffer and its window, whether CONIN$/CONOUT$ exist, whether a
// previous TUI left the console modes changed, and what escape stream tcell
// actually writes. The tool gathers those facts, then applies a rule set and
// prints a diagnosis with a fix for each finding.
//
// Run it in the same way gonomadnet is launched - same terminal, same
// environment, same redirections - once in a window that works and once in one
// that does not, and compare the DIAGNOSIS blocks.
//
// By default the tool also runs a live probe: it builds the screen tcell and
// tview build, initializes it, paints a calibration frame with the size tcell
// believes it has (and, on Windows, the size of the console window), and waits
// for a key before finalizing the screen and printing the report. That
// measurement is the ground truth the rest of the report defers to. Pass
// -screen=false to skip it, or -trace=false to skip capturing the escape
// stream.
//
// The report is written after the screen has been finalized, so it appears on
// the primary screen rather than being lost with the alternate one.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tcell/v2/terminfo"
	"golang.org/x/term"
)

// windowsDefaultTerm is the entry tcell substitutes on Windows when TERM is
// unset. It is set by tcell's own init: gmlewis/tcell v2.13.13 tscreen_win.go
// sets defaultTerm, and NewTerminfoScreenFromTtyTerminfo uses it when TERM is
// empty. It resolves to the xterm-direct entry, which emits direct colour.
const windowsDefaultTerm = "xterm-truecolor"

// Environment variables shown in the report: the ones that change how tcell
// or the terminal behaves.
var reportedEnv = []string{
	"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION",
	"LINES", "COLUMNS", "MSYSTEM", "SHELL", "OSTYPE",
	"WT_SESSION", "WT_PROFILE_ID", "ConEmuANSI", "ConEmuPID",
	"TCELL_ALTSCREEN", "TCELL_TRUECOLOR",
}

// maxReportSequences caps the escape sequences listed for one stream.
const maxReportSequences = 40

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// run gathers the facts, probes the screen, and writes the report.
func run(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("windows-diagnostic", flag.ContinueOnError)
	fs.SetOutput(w)
	runProbe := fs.Bool("screen", true,
		"run the live screen probe (takes over the terminal until a key is pressed)")
	timeout := fs.Duration("timeout", 20*time.Second,
		"how long the live probe waits for a key before finalizing the screen")
	trace := fs.Bool("trace", true,
		"capture the escape sequences tcell writes (terminfo renderer only)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(w, "windows-diagnostic - inspect the terminal layer gonomadnet draws through\n\n")
		_, _ = fmt.Fprintf(w, "Run it the same way gonomadnet is launched, once where the client works\n")
		_, _ = fmt.Fprintf(w, "and once where it does not, then compare the DIAGNOSIS blocks.\n\n")
		_, _ = fmt.Fprintf(w, "Flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	facts := gatherFacts(*runProbe, *timeout, *trace)
	writeReport(w, facts, Diagnose(facts))
	return nil
}

// gatherFacts collects the environment, the renderer tcell selects, the
// Windows console state, and optionally the live screen probe.
func gatherFacts(runProbe bool, timeout time.Duration, trace bool) Facts {
	f := Facts{GOOS: runtime.GOOS, Env: currentEnv()}

	f.TermValue, f.TermSet = f.Env.Get("TERM")
	f.EffectiveTerm = effectiveTerm(f.GOOS, f.TermValue)
	if f.EffectiveTerm != "" {
		if entry, err := terminfo.LookupTerminfo(f.EffectiveTerm); err == nil {
			f.Entry = entry
		}
	}
	if path, err := exec.LookPath("infocmp"); err == nil {
		f.InfocmpPath = path
	}

	// Ask tcell for a screen exactly as tview does. This performs the TERM
	// lookup and the choice of renderer, but opens no terminal: tcell only
	// opens CONIN$/CONOUT$ (or /dev/tty) in Init.
	screen, err := tcell.NewScreen()
	if err != nil {
		f.Backend = backendNone
		f.NewScreenError = err.Error()
	} else {
		f.Backend = backendOf(screen)
	}

	f.StdinIsTerminal = term.IsTerminal(int(os.Stdin.Fd()))
	f.Console = gatherConsole()

	if !runProbe {
		return f
	}
	if !f.StdinIsTerminal {
		f.Screen = &ScreenFacts{
			SkipReason: "stdin is not a terminal, so the probe will not take over the screen",
		}
		return f
	}
	f.Screen = probeScreen(f.Console, timeout, trace)
	return f
}

// effectiveTerm returns the entry name tcell actually resolves on goos.
//
// On Windows it is the default entry regardless of TERM: tcell's Windows init
// sets defaultTerm, and NewTerminfoScreenFromTtyTerminfo reads TERM only when
// defaultTerm is empty. Everywhere else TERM is the name tcell uses, and an
// empty one is what makes tcell fail to build a screen.
func effectiveTerm(goos, term string) string {
	if goos == "windows" {
		return windowsDefaultTerm
	}
	return term
}

// currentEnv returns the process environment as a map.
func currentEnv() Env {
	env := make(Env)
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		env[name] = value
	}
	return env
}

// writeReport renders the facts and findings as plain text.
func writeReport(w io.Writer, f Facts, findings []Finding) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }

	p("windows-diagnostic - gonomadnet terminal diagnosis\n\n")

	p("PLATFORM\n")
	p("  go               : %v/%v\n", f.GOOS, runtime.GOARCH)
	p("  terminal         : %v (heuristic, from the environment)\n", classifyTerminal(f.GOOS, f.Env))
	p("  stdin tty        : %v\n", yesNo(f.StdinIsTerminal))

	p("\nENVIRONMENT\n")
	for _, name := range reportedEnv {
		p("  %-20v : %v\n", name, envValue(f.Env, name))
	}

	p("\nRENDERER\n")
	switch {
	case f.GOOS == "windows":
		p("  TERM lookup      : ignored on Windows; tcell always uses %v\n", f.EffectiveTerm)
	case f.TermSet && f.Entry != nil:
		p("  TERM lookup      : %v resolved from tcell's built-in database\n", f.TermValue)
	case f.TermSet && f.InfocmpPath != "":
		p("  TERM lookup      : %v is not built in; tcell will run %v\n", f.TermValue, f.InfocmpPath)
	case f.TermSet:
		p("  TERM lookup      : %v cannot be resolved\n", f.TermValue)
	default:
		p("  TERM lookup      : TERM is unset and there is no default entry\n")
	}
	if f.InfocmpPath != "" {
		p("  infocmp          : %v\n", f.InfocmpPath)
	} else {
		p("  infocmp          : <not on PATH>\n")
	}
	if f.Entry != nil {
		p("  entry            : %v (%v columns x %v lines, %v colors, mouse=%v, xterm=%v, direct-color=%v)\n",
			f.Entry.Name, f.Entry.Columns, f.Entry.Lines, f.Entry.Colors,
			yesNo(f.Entry.Mouse != ""), yesNo(f.Entry.XTermLike),
			yesNo(f.Entry.TrueColor || f.Entry.SetFgRGB != "" || f.Entry.SetFgBgRGB != ""))
	}
	p("  tcell renderer   : %v\n", rendererDescription(f.Backend))
	if f.Backend == backendNone {
		p("  NewScreen error  : %v\n", consoleError(f.NewScreenError))
	}
	if f.GOOS == "windows" {
		p("  note             : tcell's Windows init sets defaultTerm, and TERM is only\n")
		p("                     consulted when defaultTerm is empty, so TERM and infocmp\n")
		p("                     do not change the renderer here. TCELL_TRUECOLOR=disable\n")
		p("                     is the lever for direct colour.\n")
	}

	writeConsoleReport(p, f.Console)
	writeScreenReport(p, f)

	p("\nDIAGNOSIS\n")
	if len(findings) == 0 {
		p("  every check passed - no problems, notes, or confirmations\n")
	}
	for _, finding := range findings {
		p("  [%v] %v\n", finding.Severity, finding.Code)
		p("      %v\n", finding.Title)
		p("      %v\n", finding.Detail)
		if finding.Fix != "" {
			p("      FIX: %v\n", finding.Fix)
		}
		p("\n")
	}
	p("\nCompare a good run with a bad run: the finding that appears in only one\n")
	p("of them is the difference between the two terminals.\n")
}

// writeConsoleReport prints the Windows console sections.
func writeConsoleReport(p func(string, ...any), c *ConsoleFacts) {
	p("\nWINDOWS CONSOLE\n")
	if c == nil {
		p("  not available on this platform\n")
		return
	}
	p("  attached         : %v\n", yesNo(c.Attached))
	p("  stdin            : %v\n", consoleLine(c.StdinIsConsole, c.StdinType, c.StdinMode, decodeInputMode))
	p("  stdout           : %v\n", consoleLine(c.StdoutIsConsole, c.StdoutType, c.StdoutMode, decodeOutputMode))
	p("  stderr           : %v\n", consoleLine(c.StderrIsConsole, c.StderrType, c.StderrMode, decodeOutputMode))
	p("  CONIN$           : %v\n", deviceLine(c.ConInOpens, c.ConInError, c.ConInModeKnown, c.ConInMode, decodeInputMode))
	p("  CONOUT$          : %v\n", deviceLine(c.ConOutOpens, c.ConOutError, c.ConOutModeKnown, c.ConOutMode, decodeOutputMode))
	if c.BufferInfo {
		p("  screen buffer    : %v x %v\n", c.BufferWidth, c.BufferHeight)
		p("  window           : %v x %v\n", c.WindowWidth, c.WindowHeight)
		p("  max window       : %v x %v\n", c.MaxWindowWidth, c.MaxWindowHeight)
		p("  cursor           : %v,%v\n", c.CursorX, c.CursorY)
	} else {
		p("  screen buffer    : <unavailable>\n")
	}
	if c.ConsoleWindow != 0 {
		p("  console HWND     : %#x (%v)\n", c.ConsoleWindow, visibleWord(c.ConsoleVisible))
	} else {
		p("  console HWND     : <none>\n")
	}
	p("  code pages       : input %v, output %v\n", c.InputCP, c.OutputCP)
}

// writeScreenReport prints the live probe section.
func writeScreenReport(p func(string, ...any), f Facts) {
	p("\nLIVE SCREEN PROBE\n")
	s := f.Screen
	if s == nil {
		p("  not run (-screen=false)\n")
		return
	}
	if s.Backend != "" {
		p("  renderer         : %v\n", s.Backend)
	}
	switch {
	case s.SkipReason != "":
		p("  skipped          : %v\n", s.SkipReason)
	case s.NewScreenError != "":
		p("  NewScreen error  : %v\n", s.NewScreenError)
	case s.InitError != "":
		p("  init             : FAILED: %v\n", s.InitError)
	default:
		p("  init             : ok\n")
		p("  tcell size       : %v x %v\n", s.Width, s.Height)
		if f.Console != nil && f.Console.BufferInfo {
			p("  console window   : %v x %v\n", f.Console.WindowWidth, f.Console.WindowHeight)
		}
		p("  colors           : %v\n", s.Colors)
		p("  mouse            : %v\n", yesNo(s.HasMouse))
		p("  charset          : %v\n", s.Charset)
		if s.TtyPresent {
			p("  tty              : %v\n", s.TtyType)
		}
	}
	if s.TraceNote != "" {
		p("  trace            : %v\n", s.TraceNote)
	}
	if s.Traced {
		p("  bytes written    : %v\n", s.BytesOut)
		p("  enter stream     : %v\n", s.EnterQuoted)
		writeSequences(p, "  enter sequences  : ", s.Enter)
		p("  exit stream      : %v\n", s.ExitQuoted)
		writeSequences(p, "  exit sequences   : ", s.Exit)
	}
}

// writeSequences lists distinct escape sequences and their counts.
func writeSequences(p func(string, ...any), label string, seqs []SequenceCount) {
	if len(seqs) == 0 {
		p("%v<none>\n", label)
		return
	}
	shown := min(len(seqs), maxReportSequences)
	for i := range shown {
		if i == 0 {
			p("%v%v x%v\n", label, seqs[i].Sequence, seqs[i].Count)
			continue
		}
		p("%v%v x%v\n", strings.Repeat(" ", len(label)), seqs[i].Sequence, seqs[i].Count)
	}
	if len(seqs) > shown {
		p("%v(%v more)\n", strings.Repeat(" ", len(label)), len(seqs)-shown)
	}
}

// envValue renders one environment variable, distinguishing unset from empty.
func envValue(env Env, name string) string {
	value, ok := env.Get(name)
	if !ok {
		return "<unset>"
	}
	if value == "" {
		return `"" (set but empty)`
	}
	return strconv.Quote(value)
}

// consoleLine describes one standard handle.
func consoleLine(isConsole bool, kind string, mode uint32, decode func(uint32) []string) string {
	if !isConsole {
		return fmt.Sprintf("not a console (%v)", kind)
	}
	return fmt.Sprintf("console, mode %v", modeText(mode, decode))
}

// deviceLine describes a console device opened by name.
func deviceLine(opens bool, errText string, modeKnown bool, mode uint32, decode func(uint32) []string) string {
	if !opens {
		return fmt.Sprintf("open failed: %v", consoleError(errText))
	}
	if !modeKnown {
		return "opens, mode <unknown>"
	}
	return fmt.Sprintf("opens, mode %v", modeText(mode, decode))
}

// modeText renders a console mode with the names of its set bits.
func modeText(mode uint32, decode func(uint32) []string) string {
	names := decode(mode)
	if len(names) == 0 {
		return fmt.Sprintf("%#04x", mode)
	}
	return fmt.Sprintf("%#04x: %v", mode, strings.Join(names, ", "))
}

// rendererDescription explains what a renderer does at the terminal.
func rendererDescription(backend string) string {
	switch backend {
	case backendTerminfo:
		return "terminfo - an ANSI byte stream (on Windows written to CONIN$/CONOUT$)"
	case backendConsole:
		return "console - the Win32 console API, not ANSI"
	case backendNone:
		return "none - tcell could not build a screen"
	default:
		return backend
	}
}

// yesNo renders a boolean as a word.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// visibleWord describes a console window's visibility.
func visibleWord(visible bool) string {
	if visible {
		return "visible"
	}
	return "hidden"
}
