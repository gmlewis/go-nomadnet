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
	"reflect"
	"testing"

	"github.com/gmlewis/tcell/v2/terminfo"
)

// console builds a ConsoleFacts for a Windows process attached to a console
// whose screen buffer is bufferW x bufferH and whose visible window is
// windowW x windowH. The modes are the ones a console has before any TUI has
// touched it.
func console(bufferW, bufferH, windowW, windowH int) *ConsoleFacts {
	normalOut := uint32(enableProcessedOutput | enableWrapAtEOLOutput)
	normalIn := uint32(enableProcessedInput | enableLineInput | enableEchoInput)
	return &ConsoleFacts{
		Attached:        true,
		StdinIsConsole:  true,
		StdoutIsConsole: true,
		StderrIsConsole: true,
		StdinMode:       normalIn,
		StdoutMode:      normalOut,
		StderrMode:      normalOut,
		ConInOpens:      true,
		ConOutOpens:     true,
		ConInMode:       normalIn,
		ConInModeKnown:  true,
		ConOutMode:      normalOut,
		ConOutModeKnown: true,
		BufferInfo:      true,
		BufferWidth:     bufferW,
		BufferHeight:    bufferH,
		WindowWidth:     windowW,
		WindowHeight:    windowH,
		MaxWindowWidth:  windowW,
		MaxWindowHeight: windowH,
	}
}

// entry returns a built-in terminfo entry as tcell would resolve it.
func entry(name string) *terminfo.Terminfo {
	return &terminfo.Terminfo{Name: name, Columns: 80, Lines: 24, Colors: 256}
}

// findings reduces findings to their codes, in the order Diagnose returns them.
func codes(findings []Finding) []string {
	var out []string
	for _, f := range findings {
		out = append(out, f.Code)
	}
	return out
}

// only returns the findings of one severity, as codes.
func only(findings []Finding, severity Severity) []string {
	var out []string
	for _, f := range findings {
		if f.Severity == severity {
			out = append(out, f.Code)
		}
	}
	return out
}

func TestDecodeInputMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode uint32
		want []string
	}{
		{"zero", 0, nil},
		{"cooked", enableProcessedInput | enableLineInput | enableEchoInput,
			[]string{"ENABLE_PROCESSED_INPUT", "ENABLE_LINE_INPUT", "ENABLE_ECHO_INPUT"}},
		{"tcell raw", enableWindowInput | enableExtendedFlags | enableVTInput,
			[]string{"ENABLE_WINDOW_INPUT", "ENABLE_EXTENDED_FLAGS", "ENABLE_VIRTUAL_TERMINAL_INPUT"}},
		{"unknown bit ignored", 0x4000,
			[]string{"0x4000"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := decodeInputMode(tc.mode); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("decodeInputMode(%#x) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

func TestDecodeOutputMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode uint32
		want []string
	}{
		{"zero", 0, nil},
		{"normal", enableProcessedOutput | enableWrapAtEOLOutput,
			[]string{"ENABLE_PROCESSED_OUTPUT", "ENABLE_WRAP_AT_EOL_OUTPUT"}},
		{"tcell", enableProcessedOutput | enableVTProcessing | disableNewlineAutoReturn | enableLVBGridWorldwide,
			[]string{
				"ENABLE_PROCESSED_OUTPUT",
				"ENABLE_VIRTUAL_TERMINAL_PROCESSING",
				"DISABLE_NEWLINE_AUTO_RETURN",
				"ENABLE_LVB_GRID_WORLDWIDE",
			}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := decodeOutputMode(tc.mode); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("decodeOutputMode(%#x) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

func TestClassifyTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		goos string
		env  Env
		want string
	}{
		{"windows terminal", "windows", Env{"WT_SESSION": "abc"}, termClassWindowsTerminal},
		{"vs code", "windows", Env{"TERM_PROGRAM": "vscode"}, termClassVSCode},
		{"conemu", "windows", Env{"ConEmuANSI": "ON"}, termClassConEmu},
		{"mintty by program", "windows", Env{"TERM_PROGRAM": "mintty"}, termClassMintty},
		{"mintty by msystem", "windows", Env{"MSYSTEM": "MINGW64"}, termClassMintty},
		{"bare windows is conhost", "windows", Env{"TERM": "xterm"}, termClassConhost},
		{"macos", "darwin", Env{"TERM": "xterm-256color"}, termClassMacOS},
		{"linux", "linux", Env{"TERM": "xterm-256color"}, termClassPOSIX},
		{"windows terminal wins over msystem", "windows", Env{"WT_SESSION": "abc", "MSYSTEM": "MINGW64"}, termClassWindowsTerminal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyTerminal(tc.goos, tc.env); got != tc.want {
				t.Errorf("classifyTerminal(%v, %v) = %q, want %q", tc.goos, tc.env, got, tc.want)
			}
		})
	}
}

func TestConsoleOutputMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		console *ConsoleFacts
		want    uint32
		wantOK  bool
	}{
		{"nil", nil, 0, false},
		{"stdout wins", &ConsoleFacts{StdoutIsConsole: true, StdoutMode: 1, ConOutModeKnown: true, ConOutMode: 2}, 1, true},
		{"falls back to CONOUT$", &ConsoleFacts{ConOutModeKnown: true, ConOutMode: 2}, 2, true},
		{"nothing known", &ConsoleFacts{}, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tc.console.OutputMode()
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("OutputMode() = (%#x, %v), want (%#x, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestEffectiveTerm(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		goos string
		term string
		want string
	}{
		{"windows ignores TERM", "windows", "xterm-kitty", windowsDefaultTerm},
		{"windows with no TERM", "windows", "", windowsDefaultTerm},
		{"unix keeps TERM", "darwin", "xterm-ghostty", "xterm-ghostty"},
		{"unix with no TERM", "linux", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := effectiveTerm(tc.goos, tc.term); got != tc.want {
				t.Errorf("effectiveTerm(%v, %q) = %q, want %q", tc.goos, tc.term, got, tc.want)
			}
		})
	}
}

func TestEscapeSequences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want []SequenceCount
	}{
		{"empty", "", nil},
		{"plain text", "hello, world", nil},
		{"single csi", "\x1b[?1049h", []SequenceCount{{Sequence: "ESC[?1049h", Count: 1}}},
		{"repeated and ordered", "\x1b[?1049h\x1b[?1049h\x1b[0m", []SequenceCount{
			{Sequence: "ESC[?1049h", Count: 2},
			{Sequence: "ESC[0m", Count: 1},
		}},
		{"truecolor sgr", "\x1b[38;2;1;2;3mX", []SequenceCount{{Sequence: "ESC[38;2;1;2;3m", Count: 1}}},
		{"osc with bel", "\x1b]52;c;YWJj\x07", []SequenceCount{{Sequence: "ESC]52;c;YWJjBEL", Count: 1}}},
		{"osc with st", "\x1b]8;;http://x\x1b\\", []SequenceCount{{Sequence: "ESC]8;;http://xESC\\", Count: 1}}},
		{"two byte escape", "\x1b>", []SequenceCount{{Sequence: "ESC>", Count: 1}}},
		{"cursor positions collapse", "\x1b[2;1H\x1b[3;1H\x1b[10;20H",
			[]SequenceCount{{Sequence: "ESC[*;*H", Count: 3}}},
		{"single row cursor position", "\x1b[5H",
			[]SequenceCount{{Sequence: "ESC[*H", Count: 1}}},
		{"truncated csi ignored", "\x1b[38;2", nil},
		{"bare esc ignored", "\x1b", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := escapeSequences([]byte(tc.in)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("escapeSequences(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDiagnose(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		f    Facts
		want []string
	}{
		{
			// The classic configuration: the ANSI renderer sized from a
			// 9001-line scrollback buffer while the window is 30 rows tall.
			name: "buffer taller than window on classic conhost",
			f: Facts{
				GOOS:      "windows",
				TermSet:   true,
				TermValue: "xterm-256color",
				Entry:     entry("xterm-256color"),
				Backend:   backendTerminfo,
				Env:       Env{},
				Console:   console(120, 9001, 120, 30),
			},
			want: []string{codeBufferSizeMismatch, codeTerminfoOnConhost},
		},
		{
			// ConPTY reports the buffer as the window, and the terminal is
			// Windows Terminal, so nothing is wrong.
			name: "conpty with matching size",
			f: Facts{
				GOOS:      "windows",
				TermSet:   true,
				TermValue: "xterm-256color",
				Entry:     entry("xterm-256color"),
				Backend:   backendTerminfo,
				Env:       Env{"WT_SESSION": "abc"},
				Console:   console(120, 30, 120, 30),
			},
			want: nil,
		},
		{
			name: "console renderer on mintty",
			f: Facts{
				GOOS:      "windows",
				TermSet:   true,
				TermValue: "msys",
				Backend:   backendConsole,
				Env:       Env{"MSYSTEM": "MINGW64"},
				Console:   console(120, 9001, 120, 30),
			},
			want: []string{
				codeConsoleOnMintty,
				codeBufferSizeMismatchConsole,
			},
		},
		{
			// The console mintty created for a native program is hidden; that
			// is the relay the client's output has to survive.
			name: "hidden console on mintty",
			f: Facts{
				GOOS:      "windows",
				TermSet:   true,
				TermValue: "xterm-256color",
				Backend:   backendTerminfo,
				Entry:     entry("xterm-direct"),
				Env:       Env{"MSYSTEM": "MINGW64"},
				Console: func() *ConsoleFacts {
					c := console(120, 30, 120, 30)
					c.ConsoleWindow = 0x1234
					c.ConsoleVisible = false
					c.ConsoleVisibleKnown = true
					return c
				}(),
			},
			want: []string{codeHiddenConsoleOnMintty},
		},
		{
			// IsWindowVisible can fail to resolve, and a zero ConsoleVisible
			// then means nothing. Unknown visibility must not be reported as a
			// hidden console.
			name: "unknown console visibility on mintty",
			f: Facts{
				GOOS:      "windows",
				TermSet:   true,
				TermValue: "xterm-256color",
				Backend:   backendTerminfo,
				Entry:     entry("xterm-direct"),
				Env:       Env{"MSYSTEM": "MINGW64"},
				Console: func() *ConsoleFacts {
					c := console(120, 30, 120, 30)
					c.ConsoleWindow = 0x1234
					return c
				}(),
			},
			want: nil,
		},
		{
			name: "unix without TERM has no screen at all",
			f: Facts{
				GOOS:           "darwin",
				Backend:        backendNone,
				NewScreenError: "terminal entry not found",
				Env:            Env{},
			},
			want: []string{codeNoTerm, codeScreenNotSelected},
		},
		{
			name: "unix TERM cannot be resolved",
			f: Facts{
				GOOS:           "linux",
				TermSet:        true,
				TermValue:      "xterm-kitty",
				Backend:        backendNone,
				NewScreenError: "terminal entry not found",
				Env:            Env{},
			},
			want: []string{codeScreenNotSelected, codeTermUnknown},
		},
		{
			name: "unix TERM resolves only by running infocmp",
			f: Facts{
				GOOS:        "linux",
				TermSet:     true,
				TermValue:   "xterm-kitty",
				InfocmpPath: "/usr/bin/infocmp",
				Backend:     backendTerminfo,
				Env:         Env{},
			},
			want: []string{codeTermNotBuiltin},
		},
		{
			name: "redirected stdout and a console left by a dead TUI",
			f: Facts{
				GOOS:    "windows",
				Backend: backendTerminfo,
				Entry:   entry("xterm-direct"),
				Env:     Env{"WT_SESSION": "abc"},
				Console: &ConsoleFacts{
					Attached:        true,
					StdinIsConsole:  true,
					StdoutIsConsole: false,
					ConInOpens:      true,
					ConOutOpens:     true,
					ConOutMode:      enableVTProcessing | disableNewlineAutoReturn,
					ConOutModeKnown: true,
					BufferInfo:      true,
					BufferWidth:     120,
					BufferHeight:    30,
					WindowWidth:     120,
					WindowHeight:    30,
				},
			},
			want: []string{
				codeLeftoverConsoleMode,
				codeStdoutNotConsole,
				codeWindowsDefaultTerm,
			},
		},
		{
			// The live probe measured the window size, so the inherited
			// buffer-size inference is suppressed.
			name: "live probe matches the window",
			f: Facts{
				GOOS:      "windows",
				TermSet:   true,
				TermValue: "xterm-256color",
				Entry:     entry("xterm-256color"),
				Backend:   backendTerminfo,
				Env:       Env{"WT_SESSION": "abc"},
				Console:   console(120, 9001, 120, 30),
				Screen:    &ScreenFacts{InitOK: true, Width: 120, Height: 30},
			},
			want: []string{codeLiveSizeMatch},
		},
		{
			name: "live probe reports the buffer height",
			f: Facts{
				GOOS:      "windows",
				TermSet:   true,
				TermValue: "xterm-256color",
				Entry:     entry("xterm-256color"),
				Backend:   backendTerminfo,
				Env:       Env{"WT_SESSION": "abc"},
				Console:   console(120, 9001, 120, 30),
				Screen:    &ScreenFacts{InitOK: true, Width: 120, Height: 9001},
			},
			want: []string{codeBufferSizeMismatch, codeLiveSizeMismatch},
		},
		{
			name: "screen probe skipped",
			f: Facts{
				GOOS:    "windows",
				Backend: backendTerminfo,
				Entry:   entry("xterm-direct"),
				Env:     Env{"WT_SESSION": "abc"},
				Console: console(120, 30, 120, 30),
				Screen:  &ScreenFacts{SkipReason: "stdin is not a terminal"},
			},
			want: []string{codeProbeSkipped, codeWindowsDefaultTerm},
		},
		{
			name: "screen init failed",
			f: Facts{
				GOOS:      "windows",
				TermSet:   true,
				TermValue: "xterm-256color",
				Entry:     entry("xterm-256color"),
				Backend:   backendTerminfo,
				Env:       Env{"WT_SESSION": "abc"},
				Console:   console(120, 30, 120, 30),
				Screen:    &ScreenFacts{InitError: "open CONOUT$: the system cannot find the file specified"},
			},
			want: []string{codeScreenInitFailed},
		},
		{
			name: "terminfo renderer with no console to draw on",
			f: Facts{
				GOOS:    "windows",
				Backend: backendTerminfo,
				Entry:   entry("xterm-direct"),
				Env:     Env{"MSYSTEM": "MINGW64"},
				Console: &ConsoleFacts{
					Attached:    false,
					ConInError:  "open CONIN$: the system cannot find the file specified",
					ConOutError: "open CONOUT$: the system cannot find the file specified",
				},
			},
			want: []string{
				codeNoConsole, codeTerminfoNoConsole,
				codeStdinNotConsole, codeStdoutNotConsole,
				codeWindowsDefaultTerm,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := codes(Diagnose(tc.f)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Diagnose() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiagnoseSeverityOrder(t *testing.T) {
	t.Parallel()
	f := Facts{
		GOOS:    "windows",
		Backend: backendTerminfo,
		Entry:   entry("xterm-direct"),
		Env:     Env{},
		Console: &ConsoleFacts{
			Attached:        true,
			StdoutIsConsole: true,
			StdoutMode:      enableVTProcessing | disableNewlineAutoReturn,
			ConOutOpens:     true,
			ConInOpens:      true,
			BufferInfo:      true,
			BufferWidth:     120,
			BufferHeight:    9001,
			WindowWidth:     120,
			WindowHeight:    30,
		},
	}
	got := Diagnose(f)
	if len(got) < 3 {
		t.Fatalf("Diagnose() returned %v findings, want at least 3", len(got))
	}
	if problems, notes := only(got, SeverityProblem), only(got, SeverityNote); len(problems) == 0 || len(notes) == 0 {
		t.Fatalf("want both problems and notes, got %v", got)
	}
	if got[0].Severity != SeverityProblem {
		t.Errorf("first finding is %v (%v), want a problem first", got[0].Severity, got[0].Code)
	}
	if last := got[len(got)-1].Severity; last == SeverityProblem {
		t.Errorf("last finding is a problem; findings are not sorted by severity: %v", got)
	}
}
