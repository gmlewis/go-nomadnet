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

// Command gorcons gives the gonomadnet client a terminal on Android, where the
// platform provides none.
//
// The appliance binds an abstract Unix socket and spawns this program, which
// dials that socket, starts the client on a pseudo-terminal (see
// nomadnet/console), and carries the terminal's bytes in both directions until
// the client exits or the app closes the connection. Everything it needs is on
// its command line, because the app is the only thing that runs it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/console"
)

const (
	// defaultCols and defaultRows are the terminal a session is given when the
	// app does not state one. They are the conventional size, and the app sends
	// the tablet's own as soon as the console view is laid out.
	defaultCols = 80
	defaultRows = 24
)

// usage is the documented form, which is the whole of this program's interface.
const usage = `gorcons runs the gonomadnet client on a pseudo-terminal and bridges it to the
appliance's console socket.

Usage:
  gorcons --socket NAME --command ABS [--home DIR] [--arg ARG]... [--env K=V]... [--cols N] [--rows N]

Options:
  --socket NAME   the abstract-socket name the appliance is listening on,
                  without the "@" that Go's net package spells it with
  --command ABS   the absolute path of the client to run
  --home DIR      the client's home, and the directory it runs in
  --arg ARG       an argument for the client; may be given more than once
  --env K=V       a variable for the client's environment; may be given more
                  than once. TERM, COLORTERM, LANG and HOME are the host's and
                  are set after these, so they cannot be replaced.
  --cols N        the terminal's width, 1..65535 (default 80)
  --rows N        the terminal's height, 1..65535 (default 24)
`

func main() {
	log.SetFlags(0)

	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(usage)
			return
		}
		log.Fatalf("gorcons: %v", err)
	}

	conn, err := net.Dial("unix", dialAddress(opts.socket))
	if err != nil {
		log.Fatalf("gorcons: dialing the appliance's console socket %v: %v", opts.socket, err)
	}
	defer func() { _ = conn.Close() }()

	sess, err := console.NewSession(opts.config(), conn)
	if err != nil {
		log.Fatalf("gorcons: %v", err)
	}
	if err := sess.Run(context.Background()); err != nil {
		log.Fatalf("gorcons: %v", err)
	}
}

// options is what the appliance asked the console host to do.
type options struct {
	socket  string
	command string
	home    string
	args    []string
	env     []string
	cols    uint16
	rows    uint16
}

// parseArgs reads the documented command line, and is a pure function of it so
// that every rule it keeps is a test rather than a comment.
func parseArgs(args []string) (options, error) {
	var (
		opts       options
		clientArgs stringList
		clientEnv  envList
		cols       uint
		rows       uint
	)

	fs := flag.NewFlagSet("gorcons", flag.ContinueOnError)
	// A command line that does not parse is reported by the caller, which is the
	// only place that knows whether anything is listening to its output.
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.socket, "socket", "", "the abstract-socket name the appliance is listening on")
	fs.StringVar(&opts.command, "command", "", "the absolute path of the client to run")
	fs.StringVar(&opts.home, "home", "", "the client's home, and the directory it runs in")
	fs.Var(&clientArgs, "arg", "an argument for the client; may be given more than once")
	fs.Var(&clientEnv, "env", "a variable for the client's environment; may be given more than once")
	fs.UintVar(&cols, "cols", defaultCols, "the terminal's width")
	fs.UintVar(&rows, "rows", defaultRows, "the terminal's height")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if rest := fs.Args(); len(rest) > 0 {
		return options{}, fmt.Errorf("gorcons takes no arguments of its own, and %q is not part of the documented form", rest[0])
	}
	if opts.socket == "" {
		return options{}, errors.New("gorcons needs --socket: the name the appliance is listening on")
	}
	if opts.command == "" {
		return options{}, errors.New("gorcons needs --command: the absolute path of the client to run")
	}
	// The same rule console.NewSession keeps, kept here as well so that a
	// misconfigured spawn fails before anything is executed.
	if !filepath.IsAbs(opts.command) {
		return options{}, fmt.Errorf(
			"--command %q must be an absolute path: Android's seccomp policy kills a process that resolves an unqualified name",
			opts.command,
		)
	}

	var err error
	if opts.cols, err = terminalSize(cols, "cols"); err != nil {
		return options{}, err
	}
	if opts.rows, err = terminalSize(rows, "rows"); err != nil {
		return options{}, err
	}
	opts.args = clientArgs
	opts.env = clientEnv
	return opts, nil
}

// terminalSize turns a terminal dimension into the width or height a resize
// frame can carry.
func terminalSize(value uint, flagName string) (uint16, error) {
	if value == 0 || value > math.MaxUint16 {
		return 0, fmt.Errorf("--%v must be between 1 and %v, and is %v", flagName, math.MaxUint16, value)
	}
	return uint16(value), nil
}

// config is the session the command line describes.
func (o options) config() console.Config {
	return console.Config{
		Command: o.command,
		Args:    o.args,
		Env:     o.childEnv(),
		Dir:     o.home,
		Cols:    o.cols,
		Rows:    o.rows,
	}
}

// childEnv is the client's entire environment.
//
// Nothing is inherited from this process: the terminal facts are the host's to
// state, because this is the only process that knows what terminal the client
// got, and HOME is the app's, which is the only place on a tablet the client
// may write to. Those four are appended last, so a caller cannot hand the client
// a terminal that is not the one it was given. Everything else is the appliance's
// to ask for through --env, which is how it tells a client what only the
// appliance knows — such as how far one wheel notch should move.
func (o options) childEnv() []string {
	env := append([]string(nil), o.env...)
	env = append(env, "TERM=xterm-256color", "COLORTERM=truecolor", "LANG=C.UTF-8")
	if o.home != "" {
		env = append(env, "HOME="+o.home)
	}
	return env
}

// dialAddress returns the address to dial for a session's socket name.
//
// The appliance binds an abstract socket, which Linux carries as a sun_path
// beginning with a NUL; Go spells that name with a leading "@". A name that is
// already a path is dialed as one, so the pair can also be run on a development
// machine, which has no abstract sockets to dial by name at all.
func dialAddress(name string) string {
	if strings.HasPrefix(name, "/") {
		return name
	}
	return "@" + name
}

// stringList collects a flag that may be given more than once, in order.
type stringList []string

// String renders the values for a diagnostic.
func (l *stringList) String() string { return strings.Join(*l, " ") }

// Set appends one more value.
func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

// envList collects the client's extra environment variables.
//
// A later value replaces an earlier one for the same name, so that what the
// client sees never depends on which duplicate a C library's getenv happens to
// return. The four variables the host owns are refused here rather than
// silently overridden, because a caller that asked for them has misunderstood
// who owns the terminal and should be told so.
type envList []string

// String renders the values for a diagnostic.
func (l *envList) String() string { return strings.Join(*l, " ") }

// Set adds one KEY=VALUE, replacing any earlier value of the same KEY.
func (l *envList) Set(value string) error {
	key, _, ok := strings.Cut(value, "=")
	if !ok || key == "" {
		return fmt.Errorf("%q is not KEY=VALUE", value)
	}
	if strings.ContainsAny(key, " \t") {
		return fmt.Errorf("%q is not a variable name", key)
	}
	if _, owned := hostEnv[key]; owned {
		return fmt.Errorf("%v is the console host's to set, not the caller's", key)
	}
	for i, existing := range *l {
		if k, _, _ := strings.Cut(existing, "="); k == key {
			(*l)[i] = value
			return nil
		}
	}
	*l = append(*l, value)
	return nil
}

// hostEnv names the variables the console host sets for every client, which no
// caller may replace.
var hostEnv = map[string]struct{}{
	"TERM": {}, "COLORTERM": {}, "LANG": {}, "HOME": {},
}
