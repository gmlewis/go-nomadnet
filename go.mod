module github.com/gmlewis/go-nomadnet

go 1.26.4

require (
	github.com/fxamacker/cbor/v2 v2.9.2
	github.com/gdamore/tcell/v2 v2.13.10
	github.com/gmlewis/go-reticulum v0.35.0
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
	golang.org/x/sys v0.38.0 // indirect
)

// tcell: carry the inputLoop keychan/keyQ shutdown-deadlock guard (upstream
// PR https://github.com/gdamore/tcell/pull/673 was closed without merge; revived
// as https://github.com/gdamore/tcell/pull/1155). The fork is gmlewis/tcell at
// tag v2.13.10-quitguard.1 (v2.13.10 + the 5-line guard on t.keychan <-). See
// memory/tcell-inputloop-keychan-send-deadlock.md for the full analysis.
replace github.com/gdamore/tcell/v2 => github.com/gmlewis/tcell/v2 v2.13.10-quitguard.1
