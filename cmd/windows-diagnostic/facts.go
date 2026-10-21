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

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gmlewis/tcell/v2/terminfo"
)

// Env is a process environment as a map, so that "set to an empty string" and
// "not set at all" can be told apart (both matter to tcell).
type Env map[string]string

// Get returns the value of name and whether it was set.
func (e Env) Get(name string) (string, bool) {
	v, ok := e[name]
	return v, ok
}

// Value returns the value of name, or the empty string when it is not set.
func (e Env) Value(name string) string { return e[name] }

// The renderers tcell can build a screen from.
const (
	backendTerminfo = "terminfo"
	backendConsole  = "console"
	backendNone     = "none"
)

// Terminal classes, as far as they can be told from the environment. They are
// deliberately broad: each one changes how a Windows binary's output reaches
// the user.
const (
	termClassWindowsTerminal = "Windows Terminal (ConPTY)"
	termClassVSCode          = "VS Code terminal (ConPTY)"
	termClassConEmu          = "ConEmu"
	termClassMintty          = "mintty (MSYS2/Cygwin)"
	termClassConhost         = "classic conhost"
	termClassMacOS           = "macOS terminal"
	termClassPOSIX           = "POSIX terminal"
)

// Windows console mode bits. The values are fixed by the Win32 ABI; they are
// repeated here (rather than imported from golang.org/x/sys/windows) so the
// decoder can be compiled and tested on every platform.
const (
	enableProcessedInput = 0x0001
	enableLineInput      = 0x0002
	enableEchoInput      = 0x0004
	enableWindowInput    = 0x0008
	enableMouseInput     = 0x0010
	enableInsertMode     = 0x0020
	enableQuickEditMode  = 0x0040
	enableExtendedFlags  = 0x0080
	enableVTInput        = 0x0200

	enableProcessedOutput    = 0x0001
	enableWrapAtEOLOutput    = 0x0002
	enableVTProcessing       = 0x0004
	disableNewlineAutoReturn = 0x0008
	enableLVBGridWorldwide   = 0x0010
)

// Finding codes. They are stable so a bad run can be compared with a good one
// without reading the prose.
const (
	codeNoTerm                    = "no-term"
	codeTermNotBuiltin            = "term-not-builtin"
	codeTermUnknown               = "term-unknown"
	codeWindowsDefaultTerm        = "windows-default-term"
	codeTerminfoNoConsole         = "terminfo-without-console"
	codeBufferSizeMismatch        = "buffer-size-mismatch"
	codeBufferSizeMismatchConsole = "buffer-size-mismatch-console"
	codeLeftoverConsoleMode       = "leftover-console-mode"
	codeNoConsole                 = "no-console-attached"
	codeStdoutNotConsole          = "stdout-not-console"
	codeStdinNotConsole           = "stdin-not-console"
	codeTerminfoOnConhost         = "terminfo-on-classic-conhost"
	codeHiddenConsoleOnMintty     = "hidden-console-on-mintty"
	codeConsoleOnMintty           = "console-backend-on-mintty"
	codeScreenNotSelected         = "screen-not-selected"
	codeScreenInitFailed          = "screen-init-failed"
	codeLiveSizeMismatch          = "live-size-mismatch"
	codeLiveSizeMatch             = "live-size-match"
	codeProbeSkipped              = "screen-probe-skipped"
)

// Severity ranks a finding: a problem is a mechanism that explains broken
// output, a note is a condition worth knowing, an info is a confirmation.
type Severity string

const (
	SeverityProblem Severity = "PROBLEM"
	SeverityNote    Severity = "NOTE"
	SeverityInfo    Severity = "INFO"
)

// Finding is one conclusion drawn from the collected facts.
type Finding struct {
	Severity Severity
	Code     string
	Title    string
	Detail   string
	Fix      string
}

// ConsoleFacts is the state of the Windows console the process is attached to,
// read without changing it.
type ConsoleFacts struct {
	Attached bool

	StdinIsConsole  bool
	StdoutIsConsole bool
	StderrIsConsole bool
	StdinType       string
	StdoutType      string
	StderrType      string
	StdinMode       uint32
	StdoutMode      uint32
	StderrMode      uint32

	ConInOpens      bool
	ConOutOpens     bool
	ConInError      string
	ConOutError     string
	ConInMode       uint32
	ConInModeKnown  bool
	ConOutMode      uint32
	ConOutModeKnown bool

	BufferInfo      bool
	BufferWidth     int
	BufferHeight    int
	WindowWidth     int
	WindowHeight    int
	MaxWindowWidth  int
	MaxWindowHeight int
	CursorX         int
	CursorY         int
	Attributes      uint16
	ConsoleWindow   uintptr
	ConsoleVisible  bool
	// ConsoleVisibleKnown records whether IsWindowVisible answered. When it did
	// not, ConsoleVisible is meaningless and must not be read as "hidden".
	ConsoleVisibleKnown bool
	InputCP             uint32
	OutputCP            uint32

	// UnavailableAPIs lists lazily-loaded Win32 procedures that could not be
	// resolved, so the report can distinguish "unknown" from a measured zero.
	UnavailableAPIs []string
}

// OutputMode returns the console output mode as it was found before the
// diagnostic ran, preferring stdout's own handle and falling back to a fresh
// CONOUT$ (which is openable even when stdout has been redirected).
func (c *ConsoleFacts) OutputMode() (uint32, bool) {
	if c == nil {
		return 0, false
	}
	if c.StdoutIsConsole {
		return c.StdoutMode, true
	}
	if c.ConOutModeKnown {
		return c.ConOutMode, true
	}
	return 0, false
}

// SequenceCount is one distinct escape sequence found in a captured stream and
// how many times it occurred. The sequence has ESC and BEL rendered as words.
type SequenceCount struct {
	Sequence string
	Count    int
}

// ScreenFacts is the result of the live screen probe.
type ScreenFacts struct {
	Backend        string
	NewScreenError string
	SkipReason     string

	InitError  string
	InitOK     bool
	Width      int
	Height     int
	Colors     int
	HasMouse   bool
	Charset    string
	TtyType    string
	TtyPresent bool

	Traced      bool
	TraceNote   string
	EnterQuoted string
	ExitQuoted  string
	Enter       []SequenceCount
	Exit        []SequenceCount
	BytesOut    int
}

// Facts is everything the probe learned, and the only input to Diagnose.
type Facts struct {
	GOOS string
	Env  Env

	TermSet   bool
	TermValue string
	// EffectiveTerm is the name tcell actually resolves: TERM when set, else
	// the Windows default entry. It is empty when there is nothing to resolve.
	EffectiveTerm string
	// Entry is the entry tcell's built-in database resolves for
	// EffectiveTerm, or nil when the name is not built in.
	Entry *terminfo.Terminfo
	// InfocmpPath is where infocmp was found, or empty. tcell shells out to it
	// for names that are not built in.
	InfocmpPath string

	// Backend is the renderer tcell.NewScreen() actually selects.
	Backend        string
	NewScreenError string

	StdinIsTerminal bool
	Console         *ConsoleFacts
	Screen          *ScreenFacts
}

// decodeInputMode names the set bits of a console input mode.
func decodeInputMode(mode uint32) []string {
	bits := []struct {
		bit  uint32
		name string
	}{
		{enableProcessedInput, "ENABLE_PROCESSED_INPUT"},
		{enableLineInput, "ENABLE_LINE_INPUT"},
		{enableEchoInput, "ENABLE_ECHO_INPUT"},
		{enableWindowInput, "ENABLE_WINDOW_INPUT"},
		{enableMouseInput, "ENABLE_MOUSE_INPUT"},
		{enableInsertMode, "ENABLE_INSERT_MODE"},
		{enableQuickEditMode, "ENABLE_QUICK_EDIT_MODE"},
		{enableExtendedFlags, "ENABLE_EXTENDED_FLAGS"},
		{enableVTInput, "ENABLE_VIRTUAL_TERMINAL_INPUT"},
	}
	return decodeMode(mode, bits)
}

// decodeOutputMode names the set bits of a console output mode.
func decodeOutputMode(mode uint32) []string {
	bits := []struct {
		bit  uint32
		name string
	}{
		{enableProcessedOutput, "ENABLE_PROCESSED_OUTPUT"},
		{enableWrapAtEOLOutput, "ENABLE_WRAP_AT_EOL_OUTPUT"},
		{enableVTProcessing, "ENABLE_VIRTUAL_TERMINAL_PROCESSING"},
		{disableNewlineAutoReturn, "DISABLE_NEWLINE_AUTO_RETURN"},
		{enableLVBGridWorldwide, "ENABLE_LVB_GRID_WORLDWIDE"},
	}
	return decodeMode(mode, bits)
}

// decodeMode names the set bits in the order given, then reports any bits left
// over as a hex value so an undocumented bit is never silently dropped.
func decodeMode(mode uint32, bits []struct {
	bit  uint32
	name string
}) []string {
	var names []string
	left := mode
	for _, b := range bits {
		if mode&b.bit != 0 {
			names = append(names, b.name)
			left &^= b.bit
		}
	}
	if left != 0 {
		names = append(names, fmt.Sprintf("0x%04x", left))
	}
	return names
}

// classifyTerminal names the terminal the process is most likely talking to,
// from the environment. It is a heuristic: the environment is the only thing
// that can be inspected without drawing.
func classifyTerminal(goos string, env Env) string {
	if goos != "windows" {
		if goos == "darwin" {
			return termClassMacOS
		}
		return termClassPOSIX
	}
	switch {
	case env.has("WT_SESSION"):
		return termClassWindowsTerminal
	case env.Value("TERM_PROGRAM") == "vscode":
		return termClassVSCode
	case env.has("ConEmuANSI") || env.has("ConEmuPID"):
		return termClassConEmu
	case env.Value("TERM_PROGRAM") == "mintty" || env.has("MSYSTEM"):
		return termClassMintty
	default:
		return termClassConhost
	}
}

// has reports whether name is set to any value.
func (e Env) has(name string) bool {
	_, ok := e[name]
	return ok
}

// Diagnose turns the facts into findings, most severe first. Every finding is
// a mechanism with a concrete explanation and, where one exists, a fix.
func Diagnose(f Facts) []Finding {
	var out []Finding
	windows := f.GOOS == "windows"
	flavor := classifyTerminal(f.GOOS, f.Env)

	// TERM and the entry tcell resolves.
	switch {
	case windows:
		// tcell ignores TERM on Windows entirely: its init sets defaultTerm,
		// and TERM is read only when defaultTerm is empty. The substitution is
		// surprising, so it is worth stating when TERM is unset - but setting
		// TERM would change nothing.
		if !f.TermSet {
			out = append(out, Finding{
				Severity: SeverityInfo,
				Code:     codeWindowsDefaultTerm,
				Title:    "tcell uses its Windows default terminal entry",
				Detail: fmt.Sprintf("TERM is not set. On Windows tcell ignores TERM: its init "+
					"sets defaultTerm=%q and the terminfo constructor reads TERM only when "+
					"defaultTerm is empty. The ANSI renderer is therefore used, with direct "+
					"colour, whatever TERM would have said.", f.EffectiveTerm),
			})
		}
	case !f.TermSet:
		out = append(out, Finding{
			Severity: SeverityProblem,
			Code:     codeNoTerm,
			Title:    "TERM is not set",
			Detail:   "tcell cannot resolve a terminal entry, so it cannot build a screen.",
			Fix:      "Set TERM to a built-in name, for example TERM=xterm-256color.",
		})
	case f.Entry == nil && f.InfocmpPath != "":
		out = append(out, Finding{
			Severity: SeverityNote,
			Code:     codeTermNotBuiltin,
			Title:    "TERM is not in tcell's built-in database",
			Detail: fmt.Sprintf("TERM=%q only resolves by running %v at startup. Whether that "+
				"works depends on PATH, so the renderer can differ between launches.",
				f.TermValue, f.InfocmpPath),
			Fix: "Use a built-in name such as xterm-256color, or keep infocmp on PATH.",
		})
	case f.Entry == nil:
		out = append(out, Finding{
			Severity: SeverityProblem,
			Code:     codeTermUnknown,
			Title:    "TERM cannot be resolved",
			Detail: fmt.Sprintf("TERM=%q is not built in and infocmp is not on PATH, so tcell "+
				"falls back to the Windows console renderer.", f.TermValue),
			Fix: "Set TERM=xterm-256color (or another built-in name).",
		})
	}

	// The renderer tcell selected, and whether it can reach the console.
	switch f.Backend {
	case backendTerminfo:
		if windows && f.Console != nil && (!f.Console.ConInOpens || !f.Console.ConOutOpens) {
			out = append(out, Finding{
				Severity: SeverityProblem,
				Code:     codeTerminfoNoConsole,
				Title:    "the ANSI renderer has no console to write to",
				Detail: fmt.Sprintf("tcell selected the terminfo renderer, which writes to "+
					"CONIN$/CONOUT$. Opening them failed (CONIN$: %v; CONOUT$: %v).",
					consoleError(f.Console.ConInError), consoleError(f.Console.ConOutError)),
				Fix: "Run inside a real console or a ConPTY terminal (Windows Terminal, or a " +
					"ConPTY-backed mintty).",
			})
		}
		if windows && f.Console != nil && f.Console.Attached && f.Console.BufferInfo {
			bufferMismatch := f.Console.BufferHeight != f.Console.WindowHeight ||
				f.Console.BufferWidth != f.Console.WindowWidth
			if bufferMismatch && !liveSizeMatches(f) {
				out = append(out, Finding{
					Severity: SeverityProblem,
					Code:     codeBufferSizeMismatch,
					Title:    "tcell will size itself from the console buffer, not the window",
					Detail: fmt.Sprintf("The console screen buffer is %vx%v but the visible "+
						"window is %vx%v. tcell's Windows terminfo tty reads its size from "+
						"GetConsoleScreenBufferInfo().dwSize, which is the buffer, so it will "+
						"draw a screen %v rows tall and the console will scroll to follow it.",
						f.Console.BufferWidth, f.Console.BufferHeight,
						f.Console.WindowWidth, f.Console.WindowHeight, f.Console.BufferHeight),
					Fix: "Make the buffer size equal the window size in the console " +
						"properties, or use Windows Terminal/ConPTY, where the buffer and " +
						"window are the same size.",
				})
			}
		}
		if windows && flavor == termClassConhost {
			out = append(out, Finding{
				Severity: SeverityNote,
				Code:     codeTerminfoOnConhost,
				Title:    "ANSI renderer on the classic console",
				Detail: "tcell's output is parsed by conhost's VT engine and re-emitted to the " +
					"window. Classic conhost supports a smaller set of sequences than a modern " +
					"terminal, and it quantises 24-bit SGR to its 16-colour palette - tcell's " +
					"Windows default entry is xterm-truecolor, i.e. direct colour.",
				Fix: "Use Windows Terminal (ConPTY), which forwards the ANSI stream intact.",
			})
		}

	case backendConsole:
		if windows && flavor == termClassMintty {
			out = append(out, Finding{
				Severity: SeverityProblem,
				Code:     codeConsoleOnMintty,
				Title:    "the console renderer cannot drive mintty",
				Detail: "tcell fell back to the Windows console API, which paints into a " +
					"console screen buffer. mintty displays a pty, so what the client draws is " +
					"not necessarily what you see.",
				Fix: "Set TERM to a name tcell can resolve (xterm-256color).",
			})
		}
		if windows && f.Console != nil && f.Console.Attached && f.Console.BufferInfo &&
			(f.Console.BufferHeight != f.Console.WindowHeight ||
				f.Console.BufferWidth != f.Console.WindowWidth) {
			out = append(out, Finding{
				Severity: SeverityInfo,
				Code:     codeBufferSizeMismatchConsole,
				Title:    "buffer/window size difference is harmless for this renderer",
				Detail: "The console renderer queries the window rectangle " +
					"(srWindow) rather than the buffer size, so the difference does not affect it.",
			})
		}

	case backendNone:
		out = append(out, Finding{
			Severity: SeverityProblem,
			Code:     codeScreenNotSelected,
			Title:    "tcell could not build a screen",
			Detail:   fmt.Sprintf("tcell.NewScreen() failed: %v.", consoleError(f.NewScreenError)),
			Fix:      "Set TERM to a built-in name and run in a terminal.",
		})
	}

	if windows && f.Console != nil {
		findings := diagnoseWindowsConsole(f, flavor)
		out = append(out, findings...)
	}

	if f.Screen != nil {
		out = append(out, diagnoseScreen(f, windows)...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if ri != rj {
			return ri < rj
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// diagnoseWindowsConsole reports what the console itself says about the
// process, independently of the renderer.
func diagnoseWindowsConsole(f Facts, flavor string) []Finding {
	var out []Finding
	c := f.Console

	if !c.Attached {
		out = append(out, Finding{
			Severity: SeverityProblem,
			Code:     codeNoConsole,
			Title:    "the process has no console",
			Detail: "CONIN$ and CONOUT$ did not open and none of the standard handles is a " +
				"console. tcell has no screen to draw on.",
			Fix: "Run from a real console or Windows Terminal. From mintty, make sure the " +
				"MSYS2 console bridge engages (or use winpty).",
		})
	}
	if !c.StdoutIsConsole {
		out = append(out, Finding{
			Severity: SeverityNote,
			Code:     codeStdoutNotConsole,
			Title:    "stdout is not a console",
			Detail: "The client's final restore sequence is written to os.Stdout (it opens " +
				"/dev/tty, which does not exist on Windows), so it will not reach the console " +
				"in this run. If the client is killed before tcell's Fini, the console keeps " +
				"the raw modes.",
			Fix: "Run without redirecting stdout.",
		})
	}
	if !c.StdinIsConsole {
		out = append(out, Finding{
			Severity: SeverityNote,
			Code:     codeStdinNotConsole,
			Title:    "stdin is not a console",
			Detail: "gonomadnet's hasTTY() uses term.IsTerminal(stdin). With stdin " +
				"redirected, the client falls back to daemon mode instead of starting the TUI.",
			Fix: "Run without redirecting stdin.",
		})
	}
	if mode, ok := c.OutputMode(); ok {
		// DISABLE_NEWLINE_AUTO_RETURN is set by a TUI and restored on exit;
		// finding it at startup means a previous session did not restore it.
		// ENABLE_WRAP_AT_EOL_OUTPUT is also cleared by a TUI, but ConPTY has
		// been seen to report it clear, so only treat that as evidence on a
		// classic console.
		leftover := mode&disableNewlineAutoReturn != 0 ||
			(flavor == termClassConhost && mode&enableWrapAtEOLOutput == 0)
		if leftover {
			out = append(out, Finding{
				Severity: SeverityProblem,
				Code:     codeLeftoverConsoleMode,
				Title:    "the console output mode looks like a TUI left it behind",
				Detail: fmt.Sprintf("The output mode is %#04x (%v). A TUI clears "+
					"ENABLE_WRAP_AT_EOL_OUTPUT and sets DISABLE_NEWLINE_AUTO_RETURN while it "+
					"runs, and restores both on exit. This console was inherited from a "+
					"session that did not.", mode, decodeOutputMode(mode)),
				Fix: "Close and reopen this terminal tab or window. Modes belong to the " +
					"console screen buffer, so every program in it is affected until it is " +
					"recreated.",
			})
		}
	}
	if c.Attached && c.ConsoleWindow != 0 && c.ConsoleVisibleKnown && !c.ConsoleVisible &&
		flavor == termClassMintty {
		out = append(out, Finding{
			Severity: SeverityNote,
			Code:     codeHiddenConsoleOnMintty,
			Title:    "the console is a hidden relay, not the window you see",
			Detail: "The process is attached to a console window that is not visible, and " +
				"the terminal is mintty. MSYS2/Cygwin created that console to run a native " +
				"Windows program from a pty and relays it: tcell writes ANSI into conhost's " +
				"VT engine, and the relay re-emits the resulting screen into mintty. How " +
				"faithfully it does that is outside the client's control.",
			Fix: "Use Windows Terminal (ConPTY), whose console is the terminal itself, or " +
				"winpty if you must stay in mintty.",
		})
	}
	return out
}

// diagnoseScreen reports what the live probe measured.
func diagnoseScreen(f Facts, windows bool) []Finding {
	var out []Finding
	s := f.Screen

	switch {
	case s.SkipReason != "":
		out = append(out, Finding{
			Severity: SeverityNote,
			Code:     codeProbeSkipped,
			Title:    "live screen probe skipped",
			Detail:   s.SkipReason,
		})
	case s.InitError != "":
		out = append(out, Finding{
			Severity: SeverityProblem,
			Code:     codeScreenInitFailed,
			Title:    "the screen failed to initialize",
			Detail:   fmt.Sprintf("screen.Init() failed: %v.", consoleError(s.InitError)),
			Fix:      "This is the error gonomadnet reports as \"TUI error\".",
		})
	case s.InitOK && windows && f.Console != nil && f.Console.WindowWidth > 0:
		if s.Width != f.Console.WindowWidth || s.Height != f.Console.WindowHeight {
			out = append(out, Finding{
				Severity: SeverityProblem,
				Code:     codeLiveSizeMismatch,
				Title:    "tcell's measured size differs from the console window",
				Detail: fmt.Sprintf("After Init, tcell reports %vx%v while the console "+
					"window is %vx%v: %v rows too tall. This is measured, not inferred.",
					s.Width, s.Height, f.Console.WindowWidth, f.Console.WindowHeight,
					s.Height-f.Console.WindowHeight),
				Fix: "See buffer-size-mismatch.",
			})
		} else {
			out = append(out, Finding{
				Severity: SeverityInfo,
				Code:     codeLiveSizeMatch,
				Title:    "tcell and the console agree on the size",
				Detail:   fmt.Sprintf("Both report %vx%v.", s.Width, s.Height),
			})
		}
	}
	return out
}

// liveSizeMatches reports whether the live probe measured a size that agrees
// with the console window. When it did, the inferred buffer-size mechanism has
// been disproved for this run and must not be reported.
func liveSizeMatches(f Facts) bool {
	s, c := f.Screen, f.Console
	return s != nil && s.InitOK && c != nil && c.WindowWidth > 0 &&
		s.Width == c.WindowWidth && s.Height == c.WindowHeight
}

// severityRank orders findings for display.
func severityRank(s Severity) int {
	switch s {
	case SeverityProblem:
		return 0
	case SeverityNote:
		return 1
	default:
		return 2
	}
}

// consoleError renders an error string for prose, substituting a placeholder
// when there is no error.
func consoleError(err string) string {
	if err == "" {
		return "no error recorded"
	}
	return err
}

// maxSequences caps how many distinct escape sequences are reported, so a
// pathological stream cannot flood the report.
const maxSequences = 64

// escapeSequences returns the distinct escape sequences in buf, in order of
// first appearance, with their occurrence counts. ESC and BEL are rendered as
// words so the report can be read without a terminal.
func escapeSequences(buf []byte) []SequenceCount {
	var out []SequenceCount
	seen := make(map[string]int)
	for i := 0; i < len(buf); {
		if buf[i] != 0x1b {
			i++
			continue
		}
		end, ok := sequenceEnd(buf, i)
		if !ok {
			break
		}
		text := renderSequence(normalizeSequence(buf[i:end]))
		if at, ok := seen[text]; ok {
			out[at].Count++
		} else if len(out) < maxSequences {
			seen[text] = len(out)
			out = append(out, SequenceCount{Sequence: text, Count: 1})
		}
		i = end
	}
	return out
}

// normalizeSequence collapses a cursor-position sequence (CSI row ; column H)
// to one shape, so a frame reports a single entry instead of one per line.
// Sequences with any other final byte, or any other parameter character, are
// returned unchanged.
func normalizeSequence(seq []byte) []byte {
	if len(seq) < 4 || seq[0] != 0x1b || seq[1] != '[' || seq[len(seq)-1] != 'H' {
		return seq
	}
	out := make([]byte, 0, len(seq))
	out = append(out, 0x1b, '[')
	number := false
	for _, c := range seq[2 : len(seq)-1] {
		switch {
		case c >= '0' && c <= '9':
			if !number {
				out = append(out, '*')
				number = true
			}
		case c == ';':
			out = append(out, c)
			number = false
		default:
			return seq
		}
	}
	return append(out, 'H')
}

// sequenceEnd returns the index just past the escape sequence that starts at
// start, or false when the buffer ends inside one.
func sequenceEnd(buf []byte, start int) (int, bool) {
	if start+1 >= len(buf) {
		return 0, false
	}
	switch buf[start+1] {
	case '[': // CSI: parameters and intermediates, then a final byte
		for i := start + 2; i < len(buf); i++ {
			if buf[i] >= 0x40 && buf[i] <= 0x7e {
				return i + 1, true
			}
		}
		return 0, false
	case ']', 'P', '^', '_': // OSC/DCS/PM/APC: BEL or ST terminates
		for i := start + 2; i < len(buf); i++ {
			if buf[i] == 0x07 {
				return i + 1, true
			}
			if buf[i] == 0x1b && i+1 < len(buf) && buf[i+1] == '\\' {
				return i + 2, true
			}
		}
		return 0, false
	default: // two-byte escape, e.g. ESC > or ESC =
		return start + 2, true
	}
}

// renderSequence makes a raw escape sequence readable, mapping ESC and BEL to
// words and capping long payloads (OSC clipboard and link data).
func renderSequence(seq []byte) string {
	var b strings.Builder
	for _, c := range seq {
		switch c {
		case 0x1b:
			b.WriteString("ESC")
		case 0x07:
			b.WriteString("BEL")
		default:
			b.WriteByte(c)
		}
	}
	s := b.String()
	const maxLen = 96
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
}
