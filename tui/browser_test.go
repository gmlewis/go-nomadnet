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
	"testing"
)

func TestHandleLinkAnchor(t *testing.T) {
	t.Parallel()
	dest, hash, err := HandleLink("#section1")
	if err != nil || dest != "anchor" || hash != "section1" {
		t.Errorf("HandleLink(#section1) = %q,%q,%v", dest, hash, err)
	}
}

func TestHandleLinkLXMF(t *testing.T) {
	t.Parallel()
	dest, hash, err := HandleLink("lxmf@aabb11223344")
	if err != nil || dest != "lxmf" || hash != "aabb11223344" {
		t.Errorf("HandleLink(lxmf@...) = %q,%q,%v", dest, hash, err)
	}
}

func TestHandleLinkRRC(t *testing.T) {
	t.Parallel()
	dest, hash, err := HandleLink("rrc://hub123")
	if err != nil || dest != "rrc" || hash != "hub123" {
		t.Errorf("HandleLink(rrc://...) = %q,%q,%v", dest, hash, err)
	}
}

func TestHandleLinkPageHash(t *testing.T) {
	t.Parallel()
	hash := "aabb11223344556677889900aabb11223344556677889900aabb112233445566"
	dest, gotHash, err := HandleLink(hash)
	if err != nil || dest != "page" || gotHash != hash {
		t.Errorf("HandleLink(64hex) = %q,%q,%v", dest, gotHash, err)
	}
}

func TestHandleLinkEmpty(t *testing.T) {
	t.Parallel()
	_, _, err := HandleLink("")
	if err == nil {
		t.Error("HandleLink(empty) should return error")
	}
}

func TestHandleLinkUnknown(t *testing.T) {
	t.Parallel()
	_, _, err := HandleLink("ftp://example.com")
	if err == nil {
		t.Error("HandleLink(ftp://) should return error")
	}
}

func TestDetectPartials(t *testing.T) {
	t.Parallel()

	markup := "Hello world\n>>header\nSome text\n>>footer\nEnd"
	partials := DetectPartials(markup)
	if len(partials) != 2 {
		t.Fatalf("DetectPartials got %d, want 2", len(partials))
	}
	if partials[0] != "header" || partials[1] != "footer" {
		t.Errorf("DetectPartials = %v, want [header footer]", partials)
	}
}

func TestDetectPartialsNone(t *testing.T) {
	t.Parallel()
	partials := DetectPartials("No partials here")
	if len(partials) != 0 {
		t.Errorf("DetectPartials got %d, want 0", len(partials))
	}
}

func TestParseMicronColorsBgFg(t *testing.T) {
	t.Parallel()
	bg, fg := ParseMicronColors("#!bg=fff\n#!fg=000\nHello")
	if bg != "fff" || fg != "000" {
		t.Errorf("ParseMicronColors = bg=%q fg=%q, want bg=fff fg=000", bg, fg)
	}
}

func TestParseMicronColorsNone(t *testing.T) {
	t.Parallel()
	bg, fg := ParseMicronColors("No colors here")
	if bg != "" || fg != "" {
		t.Errorf("ParseMicronColors = bg=%q fg=%q, want empty", bg, fg)
	}
}

func TestParseMicronColorsHex6(t *testing.T) {
	t.Parallel()
	bg, fg := ParseMicronColors("#!bg=aabbcc\n#!fg=112233")
	if bg != "aabbcc" || fg != "112233" {
		t.Errorf("ParseMicronColors = bg=%q fg=%q, want bg=aabbcc fg=112233", bg, fg)
	}
}

func TestBrowserDisplayCurrentURL(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	if bd.CurrentURL() != "" {
		t.Error("CurrentURL() should be empty initially")
	}

	bd.LoadURL("test://page1")
	if bd.CurrentURL() != "test://page1" {
		t.Errorf("CurrentURL() = %q, want test://page1", bd.CurrentURL())
	}
}

func TestBrowserDisplayBackForward(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	bd.LoadURL("page1")
	bd.LoadURL("page2")
	bd.LoadURL("page3")

	bd.GoBack()
	if bd.CurrentURL() != "page2" {
		t.Errorf("GoBack() CurrentURL = %q, want page2", bd.CurrentURL())
	}

	bd.GoBack()
	if bd.CurrentURL() != "page1" {
		t.Errorf("GoBack() CurrentURL = %q, want page1", bd.CurrentURL())
	}

	bd.GoForward()
	if bd.CurrentURL() != "page2" {
		t.Errorf("GoForward() CurrentURL = %q, want page2", bd.CurrentURL())
	}

	bd.LoadURL("page4")
	bd.GoForward()
	if bd.CurrentURL() != "page4" {
		t.Errorf("GoForward after LoadURL should not move, got %q", bd.CurrentURL())
	}
}

func TestBrowserDisplayReload(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	bd.LoadURL("test://page")
	bd.Reload()
	if bd.CurrentURL() != "test://page" {
		t.Errorf("Reload() CurrentURL = %q, want test://page", bd.CurrentURL())
	}
}
