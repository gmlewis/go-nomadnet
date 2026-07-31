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

package micron

import "testing"

// Expected values captured from Python MicronParser.make_style's
// low_color/high_color/mono_color helpers (MicronParser.py:442-591),
// extracted and run in /tmp/micron_color.py against the upstream source at
// /Users/glenn/src/github.com/markqvist/nomadnet/nomadnet/ui/textui/MicronParser.py.

func TestLowColor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"default", "default"},
		{"ddd", "light gray"},
		{"222", "dark gray"},
		{"bbb", "light gray"},
		{"999", "light gray"},
		{"777", "dark gray"},
		{"111", "black"},
		{"000", "black"},
		{"aaa", "light gray"},
		{"ccc", "light gray"},
		{"F00", "light red"},
		{"0F0", "light green"},
		{"00F", "light blue"},
		{"FF0", "yellow"},
		{"0FF", "light cyan"},
		{"F0F", "light magenta"},
		{"FFF", "white"},
		{"g00", "black"},
		{"g25", "black"},
		{"g50", "black"},
		{"g75", "black"},
		{"g99", "black"},
		{"FF0000", "light red"},
		{"00FF00", "light green"},
		{"0000FF", "light blue"},
		{"FFFFFF", "white"},
		{"000000", "black"},
		{"AABBCC", "light blue"},
		{"112233", "dark blue"},
		{"FFEEDD", "light red"},
		{"808080", "dark gray"},
		{"C0C0C0", "light gray"},
		{"123456", "dark blue"},
		{"ABCDEF", "light blue"},
	}

	for _, c := range cases {
		got := lowColor(c.in)
		if got != c.want {
			t.Errorf("lowColor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHighColor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"default", "default"},
		{"ddd", "#dddddd"},
		{"222", "#222222"},
		{"bbb", "#bbbbbb"},
		{"999", "#999999"},
		{"777", "#777777"},
		{"111", "#111111"},
		{"000", "#000000"},
		{"aaa", "#aaaaaa"},
		{"ccc", "#cccccc"},
		{"F00", "#ff0000"},
		{"0F0", "#00ff00"},
		{"00F", "#0000ff"},
		{"FF0", "#ffff00"},
		{"0FF", "#00ffff"},
		{"F0F", "#ff00ff"},
		{"FFF", "#ffffff"},
		{"g00", "g00"},
		{"g25", "g25"},
		{"g50", "g50"},
		{"g75", "g75"},
		{"g99", "g99"},
		{"FF0000", "#ff0000"},
		{"00FF00", "#00ff00"},
		{"0000FF", "#0000ff"},
		{"FFFFFF", "#ffffff"},
		{"000000", "#000000"},
		{"AABBCC", "#aabbcc"},
		{"112233", "#112233"},
		{"FFEEDD", "#ffeedd"},
		{"808080", "#808080"},
		{"C0C0C0", "#c0c0c0"},
		{"123456", "#123456"},
		{"ABCDEF", "#abcdef"},
	}

	for _, c := range cases {
		got := highColor(c.in)
		if got != c.want {
			t.Errorf("highColor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMonoColor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		fg, bg, want string
	}{
		{"ddd", "default", "default"},
		{"F00", "000", "default"},
		{"default", "default", "default"},
	}
	for _, c := range cases {
		got := monoColor(c.fg, c.bg)
		if got != c.want {
			t.Errorf("monoColor(%q,%q) = %q, want %q", c.fg, c.bg, got, c.want)
		}
	}
}
