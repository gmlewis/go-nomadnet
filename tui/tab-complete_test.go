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
	"reflect"
	"testing"
)

// TestTabComplete verifies TabComplete matches Python's
// RoomMessageEdit._try_tab_complete (Channels.py:458) and _candidates
// (Channels.py:439). Expected values were captured from the Python source run
// in /tmp/tab_ref2.py with deduped member names. The completion cycles through
// prefix matches, prepending "@" for mentions, appending ": " for a leading
// nick, and leaving the nick bare mid-line.
func TestTabComplete(t *testing.T) {
	t.Parallel()

	members := []string{"alice", "alicia", "bob", "carol"}

	// Leading-nick completion cycles alice <-> alicia. Note the doubled
	// space: "al hello" leaves the space after "al" in place, so the result
	// is "alice:  hello" ("alice: " + " hello").
	text := "al hello"
	pos := 2
	var state *TabState

	gotText, gotPos, gotState, ok := TabComplete(text, pos, state, members)
	if !ok || gotText != "alice:  hello" || gotPos != 7 || gotState.Idx != 0 || gotState.HasAt || gotState.TokenStart != 0 {
		t.Errorf("tab1: ok=%v text=%q pos=%d state=%+v, want \"alice:  hello\"/7/idx0", ok, gotText, gotPos, gotState)
	}
	state = gotState

	gotText, gotPos, gotState, ok = TabComplete(gotText, gotPos, state, members)
	if !ok || gotText != "alicia:  hello" || gotPos != 8 || gotState.Idx != 1 {
		t.Errorf("tab2: text=%q pos=%d idx=%d, want \"alicia:  hello\"/8/idx1", gotText, gotPos, gotState.Idx)
	}
	state = gotState

	gotText, gotPos, gotState, ok = TabComplete(gotText, gotPos, state, members)
	if !ok || gotText != "alice:  hello" || gotPos != 7 || gotState.Idx != 0 {
		t.Errorf("tab3: text=%q pos=%d idx=%d, want \"alice:  hello\"/7/idx0", gotText, gotPos, gotState.Idx)
	}
	state = gotState

	gotText, gotPos, gotState, ok = TabComplete(gotText, gotPos, state, members)
	if !ok || gotText != "alicia:  hello" || gotPos != 8 || gotState.Idx != 1 {
		t.Errorf("tab4: text=%q pos=%d idx=%d, want \"alicia:  hello\"/8/idx1", gotText, gotPos, gotState.Idx)
	}
}

func TestTabCompleteAtMention(t *testing.T) {
	t.Parallel()

	members := []string{"alice", "alicia", "bob", "carol"}

	text := "hi @al there"
	gotText, gotPos, st, ok := TabComplete(text, 6, nil, members)
	if !ok || gotText != "hi @alice there" || gotPos != 9 || !st.HasAt || st.TokenStart != 3 || st.Idx != 0 {
		t.Errorf("@tab1: text=%q pos=%d state=%+v, want \"hi @alice there\"/9/hasAt/idx0/ts3", gotText, gotPos, st)
	}

	gotText, gotPos, st, ok = TabComplete(gotText, gotPos, st, members)
	if !ok || gotText != "hi @alicia there" || gotPos != 10 || st.Idx != 1 {
		t.Errorf("@tab2: text=%q pos=%d idx=%d, want \"hi @alicia there\"/10/idx1", gotText, gotPos, st.Idx)
	}
}

func TestTabCompleteMidLine(t *testing.T) {
	t.Parallel()

	members := []string{"alice", "alicia", "bob", "carol"}

	// Mid-line nick (no @, not at start) inserts the bare nick.
	gotText, gotPos, st, ok := TabComplete("say al now", 6, nil, members)
	if !ok || gotText != "say alice now" || gotPos != 9 || st.HasAt || st.TokenStart != 4 || st.Idx != 0 {
		t.Errorf("mid: text=%q pos=%d state=%+v, want \"say alice now\"/9/ts4/idx0", gotText, gotPos, st)
	}
}

func TestTabCompleteNoTokenOrMatch(t *testing.T) {
	t.Parallel()

	members := []string{"alice", "alicia", "bob", "carol"}

	cases := []struct {
		name string
		text string
		pos  int
	}{
		{"empty", "", 0},
		{"no match", "zzz", 3},
		{"only space", "hello ", 6},
		{"hyphen token no match", "msg al-ice", 9},
		{"underscore token no match", "ping a_b", 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, _, ok := TabComplete(tc.text, tc.pos, nil, members)
			if ok {
				t.Errorf("%s: expected no completion, got ok=true", tc.name)
			}
		})
	}
}

func TestTabCompleteCursorMismatchResets(t *testing.T) {
	t.Parallel()

	members := []string{"alice", "alicia", "bob", "carol"}

	// First completion of "al".
	_, _, st, ok := TabComplete("al", 2, nil, members)
	if !ok || st.CursorAfter != 7 {
		t.Fatalf("setup: state=%+v ok=%v", st, ok)
	}

	// Cursor moved away to a position with no token under it: the stale
	// state must be ignored and completion must report nothing.
	_, _, _, ok = TabComplete("alice:  x", 8, st, members)
	if ok {
		t.Error("cursor mismatch should not complete when no token under cursor")
	}
}

func TestFilterCandidates(t *testing.T) {
	t.Parallel()

	got := filterTabCandidates([]string{"alice", "alicia", "bob", "Carol", "AL"}, "al")
	// Sorted by lowercase key: "AL"->"al" < "alice" < "alicia".
	want := []string{"AL", "alice", "alicia"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterTabCandidates = %v, want %v", got, want)
	}
}
