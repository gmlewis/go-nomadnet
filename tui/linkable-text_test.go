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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestLinkableTextCreation(t *testing.T) {
	t.Parallel()

	lt := NewLinkableText(nil)
	if lt == nil {
		t.Fatal("NewLinkableText returned nil")
	}
}

func TestLinkableTextAddLink(t *testing.T) {
	t.Parallel()

	lt := NewLinkableText(nil)
	lt.AddLink("Click here", "https://example.com")

	links := lt.Links()
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Target != "https://example.com" {
		t.Errorf("target = %q, want %q", links[0].Target, "https://example.com")
	}
	if links[0].Label != "Click here" {
		t.Errorf("label = %q, want %q", links[0].Label, "Click here")
	}
}

func TestLinkableTextSetText(t *testing.T) {
	t.Parallel()

	lt := NewLinkableText(nil)
	lt.SetText("Hello world")
	if lt.PlainText() != "Hello world" {
		t.Errorf("PlainText = %q, want %q", lt.PlainText(), "Hello world")
	}
}

func TestLinkableTextHandleLinkClick(t *testing.T) {
	t.Parallel()

	var clickedTarget string
	var clickedFields string
	lt := NewLinkableText(func(target, fields string) {
		clickedTarget = target
		clickedFields = fields
	})

	lt.AddLink("Go to page", "abc123")
	lt.SetText("[yellow][\"0\"]Go to page[\"\"][-]")

	lt.HandleLinkByIndex(0)

	if clickedTarget != "abc123" {
		t.Errorf("clicked target = %q, want %q", clickedTarget, "abc123")
	}
	if clickedFields != "" {
		t.Errorf("clicked fields = %q, want empty", clickedFields)
	}
}

func TestLinkableTextMultipleLinks(t *testing.T) {
	t.Parallel()

	lt := NewLinkableText(nil)
	lt.AddLink("Link 1", "target1")
	lt.AddLink("Link 2", "target2")
	lt.AddLink("Link 3", "target3")

	links := lt.Links()
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d", len(links))
	}

	wantTargets := []string{"target1", "target2", "target3"}
	for i, want := range wantTargets {
		if links[i].Target != want {
			t.Errorf("links[%d].Target = %q, want %q", i, links[i].Target, want)
		}
	}
}

func TestLinkableTextLinkWithFields(t *testing.T) {
	t.Parallel()

	var clickedFields string
	lt := NewLinkableText(func(_, fields string) {
		clickedFields = fields
	})

	lt.AddLinkWithFields("Submit", "submit_action", "name=value")
	lt.HandleLinkByIndex(0)

	if clickedFields != "name=value" {
		t.Errorf("fields = %q, want %q", clickedFields, "name=value")
	}
}

func TestLinkableTextRenderWithLinks(t *testing.T) {
	t.Parallel()

	lt := NewLinkableText(nil)
	lt.AddLink("Click me", "page1")
	lt.AddLink("Also click", "page2")
	lt.SetText("Before [yellow][\"0\"]Click me[\"\"][-] middle [yellow][\"1\"]Also click[\"\"][-] after")

	rendered := lt.RenderedText()
	if rendered == "" {
		t.Error("RenderedText should not be empty")
	}
}

func TestLinkableTextClear(t *testing.T) {
	t.Parallel()

	lt := NewLinkableText(nil)
	lt.AddLink("Link 1", "target1")
	lt.SetText("Some text")

	lt.Clear()

	if len(lt.Links()) != 0 {
		t.Errorf("after Clear, links = %d, want 0", len(lt.Links()))
	}
	if lt.PlainText() != "" {
		t.Errorf("after Clear, PlainText = %q, want empty", lt.PlainText())
	}
}

func TestLinkableTextMouseClick(t *testing.T) {
	t.Parallel()

	var clickedTarget string
	lt := NewLinkableText(func(target, _ string) {
		clickedTarget = target
	})

	lt.AddLink("First", "link1")
	lt.AddLink("Second", "link2")
	lt.SetText("[\"0\"]First[\"\"] [\"1\"]Second[\"\"]")
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("failed to init screen: %v", err)
	}
	screen.SetSize(40, 10)
	lt.SetRect(0, 0, 40, 10)
	lt.Draw(screen)

	// Dispatch mouse left click at (7, 0) which is on "Second" (col 6..12)
	event := tcell.NewEventMouse(7, 0, tcell.Button1, 0)
	lt.MouseHandler()(tview.MouseLeftClick, event, func(p tview.Primitive) {})

	if clickedTarget != "link2" {
		t.Errorf("clickedTarget = %q, want %q", clickedTarget, "link2")
	}
}
