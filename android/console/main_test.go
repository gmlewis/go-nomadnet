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
	"strings"
	"testing"
)

// The command line is the whole of this program's interface: the appliance
// builds it, and every one of these tests is about what the appliance may and
// may not say.

const (
	socketName = "gonomadnet-console-4242-000102030405060708090a0b0c0d0e0f"
	clientPath = "/data/app/~~abc==/com.gmlewis.gonomadnet-xyz==/lib/arm64/libgonomadnetclient.so"
	homeDir    = "/data/user/0/com.gmlewis.gonomadnet/files"
)

func TestParseArgsAcceptsTheDocumentedForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "separate values",
			args: []string{
				"--socket", socketName,
				"--command", clientPath,
				"--home", homeDir,
				"--arg", "--config", "--arg", homeDir + "/config",
				"--env", "GONOMADNET_WHEEL_LINES=1",
				"--cols", "100", "--rows", "40",
			},
		},
		{
			name: "equals signs",
			args: []string{
				"--socket=" + socketName,
				"--command=" + clientPath,
				"--home=" + homeDir,
				"--arg=--config", "--arg=" + homeDir + "/config",
				"--env=GONOMADNET_WHEEL_LINES=1",
				"--cols=100", "--rows=40",
			},
		},
	}

	want := options{
		socket:  socketName,
		command: clientPath,
		home:    homeDir,
		args:    []string{"--config", homeDir + "/config"},
		env:     []string{"GONOMADNET_WHEEL_LINES=1"},
		cols:    100,
		rows:    40,
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseArgs(test.args)
			if err != nil {
				t.Fatalf("parseArgs(%v) returned an error: %v", test.args, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("parseArgs(%v) returned %+v, want %+v", test.args, got, want)
			}
		})
	}
}

func TestParseArgsRequiresASocketAndACommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "nothing at all", want: "--socket"},
		{name: "no socket", args: []string{"--command", clientPath}, want: "--socket"},
		{name: "an empty socket", args: []string{"--socket", "", "--command", clientPath}, want: "--socket"},
		{name: "no command", args: []string{"--socket", socketName}, want: "--command"},
		{name: "an empty command", args: []string{"--socket", socketName, "--command", ""}, want: "--command"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseArgs(test.args)
			if err == nil {
				t.Fatalf("parseArgs(%v) accepted a command line with no %v", test.args, test.want)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("parseArgs(%v) refused with %q, which does not name %v", test.args, err, test.want)
			}
		})
	}
}

func TestParseArgsRefusesANonAbsoluteCommand(t *testing.T) {
	t.Parallel()

	// This is the seccomp rule the whole appliance rests on: Android kills a
	// process that resolves an unqualified name, so the path has to be absolute
	// before anything is executed.
	for _, command := range []string{"libgonomadnetclient.so", "client", "./client", "bin/client", "~/client"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			_, err := parseArgs([]string{"--socket", socketName, "--command", command})
			if err == nil {
				t.Fatalf("parseArgs accepted the relative command %q", command)
			}
			if !strings.Contains(err.Error(), "absolute") {
				t.Errorf("the refusal of %q is %q, and must say the path has to be absolute", command, err)
			}
		})
	}
}

func TestParseArgsDefaultsTheSizeTo80x24(t *testing.T) {
	t.Parallel()

	got, err := parseArgs([]string{"--socket", socketName, "--command", clientPath})
	if err != nil {
		t.Fatalf("parseArgs returned an error for the shortest command line: %v", err)
	}
	if got.cols != 80 || got.rows != 24 {
		t.Errorf("an unspecified size became %vx%v, want 80x24", got.cols, got.rows)
	}
}

func TestParseArgsRefusesAnImpossibleSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no columns", args: []string{"--cols", "0"}, want: "--cols"},
		{name: "no rows", args: []string{"--rows", "0"}, want: "--rows"},
		{name: "too many columns", args: []string{"--cols", "65536"}, want: "--cols"},
		{name: "too many rows", args: []string{"--rows", "65536"}, want: "--rows"},
		// A value that is not a number at all is refused by the flag parser,
		// which names the flag the way it accepts it, with one dash.
		{name: "not a number", args: []string{"--cols", "wide"}, want: "cols"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			args := append([]string{"--socket", socketName, "--command", clientPath}, test.args...)
			_, err := parseArgs(args)
			if err == nil {
				t.Fatalf("parseArgs(%v) accepted %v", args, test.name)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("the refusal %q does not name %v", err, test.want)
			}
		})
	}
}

func TestParseArgsRefusesWhatTheDocumentedFormDoesNotHave(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "an unknown flag", args: []string{"--socket", socketName, "--command", clientPath, "--nope"}},
		{name: "a positional argument", args: []string{"--socket", socketName, "--command", clientPath, "extra"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseArgs(test.args); err == nil {
				t.Errorf("parseArgs(%v) accepted %v", test.args, test.name)
			}
		})
	}
}

func TestConfigIsTheSessionTheCommandLineDescribes(t *testing.T) {
	t.Parallel()

	got, err := parseArgs([]string{
		"--socket", socketName,
		"--command", clientPath,
		"--home", homeDir,
		"--arg", "--config", "--arg", homeDir + "/config",
		"--cols", "100", "--rows", "40",
	})
	if err != nil {
		t.Fatalf("parseArgs returned an error: %v", err)
	}

	cfg := got.config()
	if cfg.Command != clientPath {
		t.Errorf("the session runs %q, want %q", cfg.Command, clientPath)
	}
	if !reflect.DeepEqual(cfg.Args, []string{"--config", homeDir + "/config"}) {
		t.Errorf("the session's arguments are %q, want the client's own", cfg.Args)
	}
	if cfg.Dir != homeDir {
		t.Errorf("the session runs in %q, want the client's home %q", cfg.Dir, homeDir)
	}
	if cfg.Cols != 100 || cfg.Rows != 40 {
		t.Errorf("the session's terminal is %vx%v, want 100x40", cfg.Cols, cfg.Rows)
	}
}

func TestChildEnvIsTheTerminalTheApplianceNeeds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		home string
		env  []string
		want []string
	}{
		{
			name: "with a home",
			home: homeDir,
			want: []string{"TERM=xterm-256color", "COLORTERM=truecolor", "LANG=C.UTF-8", "HOME=" + homeDir},
		},
		{
			name: "with no home",
			want: []string{"TERM=xterm-256color", "COLORTERM=truecolor", "LANG=C.UTF-8"},
		},
		{
			// The terminal facts come last for this reason: a caller that asked
			// for TERM cannot be holding a terminal it does not have, and HOME
			// stays where the appliance said the client may write.
			name: "the appliance's own variables do not displace the host's",
			home: homeDir,
			env:  []string{"GONOMADNET_WHEEL_LINES=1"},
			want: []string{
				"GONOMADNET_WHEEL_LINES=1",
				"TERM=xterm-256color", "COLORTERM=truecolor", "LANG=C.UTF-8", "HOME=" + homeDir,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := options{home: test.home, env: test.env}.childEnv()
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("the child's environment is %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseArgsRefusesAnEnvironmentItCannotState(t *testing.T) {
	t.Parallel()

	// An env flag is a KEY=VALUE and nothing else, and the four the host owns
	// are refused rather than quietly overridden: a caller that asks for one has
	// misunderstood who owns the terminal.
	tests := []struct {
		name  string
		value string
	}{
		{name: "no equals sign", value: "GONOMADNET_WHEEL_LINES"},
		{name: "no name", value: "=1"},
		{name: "a name with a space", value: "GO NOMADNET=1"},
		{name: "TERM", value: "TERM=xterm"},
		{name: "COLORTERM", value: "COLORTERM=false"},
		{name: "LANG", value: "LANG=C"},
		{name: "HOME", value: "HOME=/sdcard"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseArgs([]string{
				"--socket", socketName, "--command", clientPath, "--env", test.value,
			}); err == nil {
				t.Errorf("parseArgs accepted --env %q, want a refusal", test.value)
			}
		})
	}
}

func TestParseArgsLetsTheLastValueOfANameWin(t *testing.T) {
	t.Parallel()

	// Two values for one name are one variable, and which one the client sees
	// must not depend on how a C library picks among duplicates in the
	// environment array. The last one said is the one meant.
	got, err := parseArgs([]string{
		"--socket", socketName, "--command", clientPath,
		"--env", "GONOMADNET_WHEEL_LINES=8", "--env", "GONOMADNET_WHEEL_LINES=1",
		"--env", "GONOMADNET_MOUSE_DEBUG=1",
	})
	if err != nil {
		t.Fatalf("parseArgs returned an error: %v", err)
	}

	want := []string{"GONOMADNET_WHEEL_LINES=1", "GONOMADNET_MOUSE_DEBUG=1"}
	if !reflect.DeepEqual(got.env, want) {
		t.Errorf("the client's environment is %q, want %q", got.env, want)
	}
}

func TestDialAddressIsAbstractForTheAppsName(t *testing.T) {
	t.Parallel()

	// The appliance binds an abstract socket, which Linux carries as a sun_path
	// that begins with a NUL; Go spells that with a leading "@". A name that is
	// already a path is dialed as one, which is how the pair runs on a
	// development machine that has no abstract sockets at all.
	tests := []struct {
		name string
		want string
	}{
		{name: socketName, want: "@" + socketName},
		{name: "/tmp/gonomadnet-console.sock", want: "/tmp/gonomadnet-console.sock"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := dialAddress(test.name); got != test.want {
				t.Errorf("dialAddress(%q) = %q, want %q", test.name, got, test.want)
			}
		})
	}
}
