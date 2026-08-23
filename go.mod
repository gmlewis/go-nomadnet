module github.com/gmlewis/go-nomadnet

go 1.26.4

require (
	github.com/creack/pty/v2 v2.0.1
	github.com/fxamacker/cbor/v2 v2.9.2
	github.com/gdamore/tcell/v2 v2.13.10
	github.com/gmlewis/go-reticulum v0.39.0
	github.com/mattn/go-runewidth v0.0.16
	github.com/mdp/qrterminal/v3 v3.2.1
	github.com/rivo/tview v0.42.0
	golang.org/x/term v0.37.0
	golang.org/x/text v0.38.0
	rsc.io/qr v0.2.0
)

require (
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/sys v0.42.0 // indirect
)

// tcell fork (gmlewis/tcell): carries the inputLoop keychan/keyQ
// shutdown-deadlock guard, eventQ post quit-guard, cursor flicker
// optimization (showCursor position-unchanged shortcut + lastShowVisible
// tracking), always-on mode 2026 (synchronized output), and per-cell
// incremental rendering (forcedDirty flag — Put no longer clobbers
// lastStr, so unchanged cells are skipped by drawCell).
replace github.com/gdamore/tcell/v2 => github.com/gmlewis/tcell/v2 v2.13.11-0.20260823113451-e5d19bcf6c9b

// tview fork (gmlewis/tview): carries the fullRedraw flag (draw skips
// screen.Clear on normal redraws, relying on tcell per-cell dirty
// checking), SetFocus early-return when focus unchanged (eliminates
// redundant Blur/HideCursor cursor flicker), v0.42.0-compatible
// SetFocus/GetFocus (direct a.focus field), and v0.42.0-style HasFocus
// methods on all containers.
replace github.com/rivo/tview => github.com/gmlewis/tview v0.0.0-20260823113522-3ec2ab956f0b
