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
	"errors"
	"testing"
)

// The editor seam's tests.
//
// A host that cannot start a program — the iOS appliance — installs a check, and the
// two screens that open an editor ask it first. What is being tested is that the
// answer is what the person sees: a refusal that is reported, and no editor mounted,
// rather than a launch that produced nothing and said nothing.

// TestTheConfigPagesEditorIsRefusedWithTheHostsReason asserts the Config page asks the
// installed check before it starts anything.
//
// It is deliberately not parallel: it installs a check for the whole package, and tests
// running beside it would be asking it too.
func TestTheConfigPagesEditorIsRefusedWithTheHostsReason(t *testing.T) {
	defer SetEditorCheck(nil)

	app := newTestApp()
	cd := NewConfigDisplay(app, "/tmp/gonomadnet-config-for-editor-check")

	refusal := errors.New("the appliance edits its configuration in the app")
	SetEditorCheck(func(editorCmd string) error { return refusal })

	cd.ShowEditor("", cd.configPath)

	if cd.editor != nil {
		t.Error("a refused editor was mounted over the explainer")
	}
	if got := app.Dialogs.Count(); got != 1 {
		t.Fatalf("the refusal was not reported: dialogs open = %v, want 1", got)
	}
}

// TestTheInterfacesPagesEditorIsRefusedWithTheHostsReason asserts the other screen
// that opens an editor asks the same check.
func TestTheInterfacesPagesEditorIsRefusedWithTheHostsReason(t *testing.T) {
	defer SetEditorCheck(nil)

	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	SetEditorCheck(func(editorCmd string) error {
		return errors.New("the appliance edits its configuration in the app")
	})

	id.ShowEditor("", "/tmp/gonomadnet-rns-config")

	if id.editor != nil {
		t.Error("a refused editor was mounted over the interface list")
	}
	if !id.pages.HasPage("main") {
		t.Error("a refused editor did not leave the interface list in place")
	}
}

// TestAnEditorCheckIsAskedTheConfiguredEditor asserts the check is given the editor
// the configuration names, which is what lets a host's answer quote it.
func TestAnEditorCheckIsAskedTheConfiguredEditor(t *testing.T) {
	defer SetEditorCheck(nil)

	var asked string
	SetEditorCheck(func(editorCmd string) error {
		asked = editorCmd
		return errors.New("refused")
	})

	app := newTestApp()
	cd := NewConfigDisplay(app, "/tmp/gonomadnet-config-for-editor-check")
	cd.ShowEditor("some-editor", cd.configPath)

	if asked != "some-editor" {
		t.Errorf("the check was given %q, want %q", asked, "some-editor")
	}
}

// TestAnEditorCheckIsNotInstalledByDefault asserts the desktop path: with no
// check installed, the client starts the editor it was configured with.
func TestAnEditorCheckIsNotInstalledByDefault(t *testing.T) {
	defer SetEditorCheck(nil)
	SetEditorCheck(nil)

	app := newTestApp()
	cd := NewConfigDisplay(app, "/tmp/gonomadnet-config-for-editor-check")

	// A command this machine cannot have, so the launch fails rather than being
	// refused by a check nobody installed.
	cd.ShowEditor("gonomadnet-no-such-editor", cd.configPath)

	if cd.editor != nil {
		t.Error("a failed launch left an editor mounted over the explainer")
	}
	if got := app.Dialogs.Count(); got != 1 {
		t.Fatalf("the failure was not reported: dialogs open = %v, want 1", got)
	}
}
