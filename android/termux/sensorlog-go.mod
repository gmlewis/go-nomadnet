module sensorlog

go 1.26.0

require github.com/gmlewis/go-reticulum v0.0.0

// The replacement is the sibling checkout of go-reticulum, the same one this
// repository's go.work uses. The path is resolved against the repository root —
// the go.mod this file stands in for under -modfile — so it resolves on any
// machine that has the two repositories side by side. Note that -modfile is
// rejected in workspace mode, so the workspace has to be disabled:
//
//	GOWORK=off go build -modfile=android/termux/sensorlog-go.mod -o sensorlog ./android/termux
replace github.com/gmlewis/go-reticulum => ../go-reticulum
