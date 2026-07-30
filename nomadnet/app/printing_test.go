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

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
)

func TestRenderTemplate(t *testing.T) {
	t.Parallel()
	got := renderTemplate("From: {origin} | {mtitle} | {mbody}", map[string]string{
		"origin": "Alice",
		"mtitle": "Hi",
		"mbody":  "Body",
	})
	want := "From: Alice | Hi | Body"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintFile(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	dir := a.TmpFilesPath
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	a.PrintCommand = "true"
	if !a.PrintFile(f) {
		t.Fatal("true command should succeed")
	}
	a.PrintCommand = "false"
	if a.PrintFile(f) {
		t.Fatal("false command should fail")
	}
	a.PrintCommand = "/no/such/command/xyz"
	if a.PrintFile(f) {
		t.Fatal("missing command should fail")
	}
}

func TestPrintMessage(t *testing.T) {
	t.Parallel()
	outFile := filepath.Join(tempDir(t), "printmsg_out")
	script := filepath.Join(tempDir(t), "copyprint.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncp \"$1\" "+outFile+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if err := os.MkdirAll(a.TmpFilesPath, 0o755); err != nil {
		t.Fatal(err)
	}
	a.Dir = directory.New()
	a.PrintCommand = script

	msg := &lxmf.Message{
		SourceHash: []byte{0xaa, 0xbb, 0xcc, 0xdd},
		Title:      []byte("Greeting"),
		Content:    []byte("Hello there"),
		Timestamp:  1700000000,
	}
	a.PrintMessage(msg, time.Unix(1700000100, 0))

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output not written: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "From: <aabbccdd>") {
		t.Errorf("output missing display name: %q", s)
	}
	if !strings.Contains(s, "Title: Greeting") {
		t.Errorf("output missing title: %q", s)
	}
	if !strings.Contains(s, "Hello there") {
		t.Errorf("output missing body: %q", s)
	}
	if !strings.Contains(s, "Sent: ") {
		t.Errorf("output missing sent time: %q", s)
	}
	if !strings.Contains(s, "Rcvd: ") {
		t.Errorf("output missing received time: %q", s)
	}
}
