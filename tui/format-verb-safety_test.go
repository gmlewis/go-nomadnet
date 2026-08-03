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

package tui

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestNoFormatVOnByteSlice is a static guard against the one class of
// %s => %v conversion that actually changes behavior: applying %v (or any of
// its flag/width variants — %+v, %#v, %-v, %5v, …) to a []byte / []uint8
// argument. For a string argument %s and %v are identical, and for types with a
// String() method both verbs call it, so swapping %s => %v is a no-op in those
// cases. But for a []byte, %s renders the UTF-8 text while %v renders
// "[102 114 101 115 104]" — a real behavioral break.
//
// This test type-checks the whole tui package (production + test files) with
// the standard-library source importer and walks every format-printing call
// (fmt.Sprintf, fmt.Errorf, fmt.Printf, fmt.Fprintf, and the testing.T/B/F
// Errorf/Fatalf/Logf methods, plus t.Run subtest names) whose format string is
// a literal. For each %v-family verb it resolves the corresponding argument's
// type and fails if that type is a []byte / []uint8 slice. The guard passes
// today (the codebase uses %s / %x for byte slices) and will fail the moment a
// future %s => %v change targets a []byte argument — preventing the regression
// the user asked to guard against.
//
// It uses only the standard library (go/parser, go/types, go/importer) so no
// new module dependency is required.
func TestNoFormatVOnByteSlice(t *testing.T) {
	t.Parallel()

	// Locate this package's source directory regardless of the test's CWD so
	// `go test` works from any invocation directory.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed: cannot locate the package directory")
	}
	dir := filepath.Dir(thisFile)

	fset := token.NewFileSet()
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", filepath.Join(dir, "*.go"), err)
	}
	if len(names) == 0 {
		t.Fatalf("no .go files found in %s", dir)
	}

	var files []*ast.File
	for _, n := range names {
		f, perr := parser.ParseFile(fset, n, nil, parser.ParseComments)
		if perr != nil {
			t.Fatalf("parse %s: %v", n, perr)
		}
		files = append(files, f)
	}

	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}
	pkg, err := conf.Check("github.com/gmlewis/go-nomadnet/tui", fset, files, info)
	if err != nil {
		// A non-nil pkg with partial errors is acceptable: the scan simply
		// skips arguments it could not type. Only abort if nothing checked.
		t.Logf("type-check produced errors (scan proceeds on what typed ok): %v", err)
	}
	if pkg == nil {
		t.Fatalf("type-check failed entirely: %v", err)
	}

	var bad []string
	for _, f := range files {
		fileName := filepath.Base(fset.File(f.Pos()).Name())
		ast.Inspect(f, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := ce.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn := sel.Sel.Name
			if !isFormatCall(fn) {
				return true
			}
			fmtStr, fmtArgs, ok := formatStringAndArgs(fn, ce)
			if !ok {
				return true
			}
			for _, va := range formatVerbArgs(fmtStr) {
				if va.verb != 'v' {
					continue
				}
				if va.argIdx < 0 || va.argIdx >= len(fmtArgs) {
					continue
				}
				arg := fmtArgs[va.argIdx]
				tv, ok := info.Types[arg]
				if !ok {
					continue
				}
				if isByteSlice(tv.Type) {
					pos := fset.Position(arg.Pos())
					bad = append(bad, pos.String()+": %v applied to "+tv.Type.String()+
						" in "+fileName+" — use %s (UTF-8 text) or %x (hex) for byte slices")
				}
			}
			return true
		})
	}

	if len(bad) != 0 {
		t.Errorf("found %d format call(s) applying %%v to a []byte / []uint8 argument\n"+
			"(%%v renders \"[102 114 ...]\" instead of the UTF-8 text; use %%s, or %%x for a hash):\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}

// isFormatCall reports whether the selector name is a format-printing function
// or method that takes a printf-style format string.
func isFormatCall(name string) bool {
	switch name {
	case "Sprintf", "Errorf", "Printf", "Fprintf", "Fatalf", "Logf", "Runf":
		return true
	}
	return false
}

// formatStringAndArgs returns the printf format string literal and the
// remaining arguments for a format-printing call. The format string must be a
// basic string literal; otherwise the call is skipped (we cannot analyze a
// dynamically-built format). Fprintf takes a writer as its first argument, so
// its format string sits at ce.Args[1].
func formatStringAndArgs(fn string, ce *ast.CallExpr) (string, []ast.Expr, bool) {
	if fn == "Fprintf" {
		if len(ce.Args) < 2 {
			return "", nil, false
		}
		lit, ok := ce.Args[1].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", nil, false
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return "", nil, false
		}
		return s, ce.Args[2:], true
	}
	if len(ce.Args) < 1 {
		return "", nil, false
	}
	lit, ok := ce.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", nil, false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", nil, false
	}
	return s, ce.Args[1:], true
}

type verbArg struct {
	verb   byte
	argIdx int
}

// formatVerbArgs maps each consuming verb in a printf format string to the
// index (0-based, among the arguments after the format string) it consumes.
// Non-consuming verbs (%% and unrecognised) are skipped; an unmappable verb
// yields a -1 index the caller must guard against. It handles explicit
// indexing ([n]), flags, width and precision (including '*' which consumes an
// argument), following the same ordering as the fmt package.
func formatVerbArgs(format string) []verbArg {
	var out []verbArg
	argIdx := 0
	i := 0
	for i < len(format) {
		if format[i] != '%' {
			i++
			continue
		}
		i++ // consume '%'
		if i < len(format) && format[i] == '%' {
			i++
			continue
		}
		// optional explicit index [n]
		explicit := -1
		if i < len(format) && format[i] == '[' {
			j := i + 1
			num := ""
			for j < len(format) && format[j] >= '0' && format[j] <= '9' {
				num += string(format[j])
				j++
			}
			if j < len(format) && format[j] == ']' && num != "" {
				n, _ := strconv.Atoi(num)
				explicit = n - 1
				i = j + 1
			} else {
				return out // malformed; stop analyzing
			}
		}
		// flags
		for i < len(format) && strings.IndexByte("+-# 0", format[i]) >= 0 {
			i++
		}
		// width: '*' (consumes an implicit arg) or digits
		if i < len(format) && format[i] == '*' {
			if explicit < 0 {
				argIdx++
			}
			i++
		}
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
		// precision: '.' then '*' (consumes an implicit arg) or digits
		if i < len(format) && format[i] == '.' {
			i++
			if i < len(format) && format[i] == '*' {
				if explicit < 0 {
					argIdx++
				}
				i++
			}
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
		if i >= len(format) {
			return out
		}
		verb := format[i]
		i++
		switch verb {
		case 'v', 's', 'd', 'f', 'g', 'G', 'e', 'E', 't', 'T', 'b', 'c', 'o', 'O', 'q', 'x', 'X', 'U', 'p':
			ai := explicit
			if ai < 0 {
				ai = argIdx
				argIdx++
			}
			out = append(out, verbArg{verb: verb, argIdx: ai})
		default:
			// non-consuming / unknown verb: ignore
		}
	}
	return out
}

// isByteSlice reports whether t is a (unnamed) []byte / []uint8 slice — the
// type for which %v and %s differ (and for which %v is therefore wrong).
func isByteSlice(t types.Type) bool {
	s, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	b, ok := s.Elem().(*types.Basic)
	if !ok {
		return false
	}
	return b.Kind() == types.Byte || b.Kind() == types.Uint8
}
