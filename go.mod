module github.com/gmlewis/go-nomadnet

go 1.26.4

require (
	github.com/creack/pty/v2 v2.0.1
	github.com/fxamacker/cbor/v2 v2.9.2
	github.com/gdamore/tcell/v2 v2.13.10
	github.com/gmlewis/go-reticulum v0.64.0
	github.com/mattn/go-runewidth v0.0.16
	github.com/mdp/qrterminal/v3 v3.2.1
	github.com/rivo/tview v0.42.0
	golang.design/x/clipboard v0.9.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.45.0
	golang.org/x/text v0.38.0
	rsc.io/qr v0.2.0
)

require (
	github.com/ebitengine/purego v0.10.1 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.design/x/x11 v0.2.0 // indirect
	golang.org/x/exp/shiny v0.0.0-20250606033433-dcc06ee1d476 // indirect
	golang.org/x/image v0.28.0 // indirect
	golang.org/x/mobile v0.0.0-20250606033058-a2a15c67f36f // indirect
)

// tcell fork (gmlewis/tcell): carries the inputLoop keychan/keyQ
// shutdown-deadlock guard, eventQ post quit-guard, cursor flicker
// optimization (showCursor position-unchanged shortcut + lastShowVisible
// tracking), always-on mode 2026 (synchronized output), and per-cell
// incremental rendering (forcedDirty flag — Put no longer clobbers
// lastStr, so unchanged cells are skipped by drawCell).
replace github.com/gdamore/tcell/v2 => github.com/gmlewis/tcell/v2 v2.13.11-0.20260824212538-e24b6c4fe8a2

// tview fork (gmlewis/tview): carries the fullRedraw flag (draw skips
// screen.Clear on normal redraws, relying on tcell per-cell dirty
// checking), SetFocus early-return when focus unchanged (eliminates
// redundant Blur/HideCursor cursor flicker), v0.42.0-compatible
// SetFocus/GetFocus (direct a.focus field), v0.42.0-style HasFocus
// methods on all containers, Box.Focus/Blur callback restoration,
// List.Draw adjustOffset call, and WordWrap/stripTags region-tag fix.
replace github.com/rivo/tview => github.com/gmlewis/tview v0.0.0-20260829232818-68e669800296
