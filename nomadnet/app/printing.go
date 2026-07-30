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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gmlewis/go-reticulum/lxmf"
)

// defaultPrintingTemplate is the default message-printing template, matching
// the Python NomadNet __printing_template_msg__.
const defaultPrintingTemplate = `
---------------------------
From: {origin}
Sent: {stime}
Rcvd: {rtime}
Title: {mtitle}

{mbody}
---------------------------
`

// defaultTimeFormat matches the Python NomadNet time_format.
const defaultTimeFormat = "2006-01-02 15:04:05"

// PrintFile runs the configured print command against the given file, mirroring
// the Python NomadNet print_file. It returns true when the command exits with
// status 0.
func (a *App) PrintFile(filename string) bool {
	parts := shlexSplit(a.PrintCommand + " " + filename)
	if len(parts) == 0 {
		return false
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if a.Logger != nil {
			a.Logger.Error("An error occurred while executing print command: %v: %v", a.PrintCommand+" "+filename, err)
		}
		return false
	}
	return true
}

// PrintMessage renders an incoming LXMF message using the printing template and
// prints the result via PrintFile, mirroring the Python NomadNet print_message.
// received is the reception time; when zero the current time is used.
func (a *App) PrintMessage(msg *lxmf.Message, received time.Time) {
	template := a.PrintingTemplateMsg
	if strings.TrimSpace(template) == "" {
		template = defaultPrintingTemplate
	}
	if received.IsZero() {
		received = time.Now()
	}
	stime := time.Unix(int64(msg.Timestamp), 0).Format(defaultTimeFormat)
	rtime := received.Format(defaultTimeFormat)

	displayName := ""
	if a.Dir != nil {
		displayName = a.Dir.SimplestDisplayStr(msg.SourceHash)
	}
	title := msg.TitleString()
	if title == "" {
		title = "None"
	}
	body := msg.ContentString()

	output := renderTemplate(template, map[string]string{
		"origin": displayName,
		"stime":  stime,
		"rtime":  rtime,
		"mtitle": title,
		"mbody":  body,
	})

	sum := sha256.Sum256([]byte(output))
	filename := filepath.Join(a.TmpFilesPath, hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(a.TmpFilesPath, 0o755); err != nil {
		if a.Logger != nil {
			a.Logger.Error("Could not create tmp dir for printing: %v", err)
		}
		return
	}
	if err := os.WriteFile(filename, []byte(output), 0o644); err != nil {
		if a.Logger != nil {
			a.Logger.Error("Could not write print file: %v", err)
		}
		return
	}
	a.PrintFile(filename)
	_ = os.Remove(filename)
}

// renderTemplate replaces {name} placeholders in the template with the provided
// values, mirroring Python str.format for the known field set.
func renderTemplate(template string, fields map[string]string) string {
	out := template
	for k, v := range fields {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}

// shlexSplit performs a minimal shell-style split on whitespace, respecting
// single and double quotes, mirroring Python's shlex.split for simple inputs.
func shlexSplit(s string) []string {
	var parts []string
	var cur strings.Builder
	var inSingle, inDouble bool
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(c)
			}
		case inDouble:
			if c == '"' {
				inDouble = false
			} else {
				cur.WriteByte(c)
			}
		default:
			switch c {
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case ' ', '\t', '\n':
				flush()
			default:
				cur.WriteByte(c)
			}
		}
	}
	flush()
	return parts
}
