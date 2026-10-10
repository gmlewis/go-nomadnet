// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseArgsReadsTheDocumentedCommandLine(t *testing.T) {
	t.Parallel()

	opts, err := parseArgs([]string{"--socket", "gonomadnet-rnode-7-ab", "--tty", "/files/etc/rnode/tty"})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if opts.socket != "gonomadnet-rnode-7-ab" {
		t.Errorf("socket is %q", opts.socket)
	}
	if opts.tty != "/files/etc/rnode/tty" {
		t.Errorf("tty is %q", opts.tty)
	}
	// With no --uid the bridge serves this process's own uid, which is the
	// appliance's: the two are the same application.
	if opts.uid != os.Getuid() {
		t.Errorf("uid is %v, want %v", opts.uid, os.Getuid())
	}
}

func TestParseArgsStatesItsOwnUid(t *testing.T) {
	t.Parallel()

	opts, err := parseArgs([]string{"--socket", "s", "--tty", "/t", "--uid", "10123"})
	if err != nil {
		t.Fatalf("parseArgs failed: %v", err)
	}
	if opts.uid != 10123 {
		t.Errorf("uid is %v, want 10123", opts.uid)
	}
}

func TestParseArgsRefusesACommandLineThatIsNotTheDocumentedOne(t *testing.T) {
	t.Parallel()

	// Both flags name something this program cannot invent: a socket nobody
	// dials is a radio that never connects, and a pty whose path nobody is told
	// is a transport configured with a path that does not exist.
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no socket", []string{"--tty", "/t"}, "--socket"},
		{"no tty", []string{"--socket", "s"}, "--tty"},
		{"empty socket", []string{"--socket", "", "--tty", "/t"}, "--socket"},
		{"a bare argument", []string{"--socket", "s", "--tty", "/t", "extra"}, "extra"},
		{"an unknown flag", []string{"--socket", "s", "--tty", "/t", "--wat"}, "wat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseArgs(tc.args)
			if err == nil {
				t.Fatalf("parseArgs(%v) reported no error", tc.args)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error %q does not name %q", err.Error(), tc.want)
			}
		})
	}
}
