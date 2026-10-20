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

import "testing"

// TestSyncButtonOffersSyncNowAfterFailure pins that the Message Sync dialog's
// button is chosen from the router's state, not from the status wording
// (Conversations.py:1570):
//
//	self.app.get_sync_status() == "Idle" or
//	self.app.message_router.propagation_transfer_state >= LXMF.LXMRouter.PR_COMPLETE
//
// Every terminal failure is >= PR_COMPLETE (PR_NO_PATH 0xf0, PR_LINK_FAILED
// 0xf1, PR_TRANSFER_FAILED 0xf2, PR_NO_IDENTITY_RCVD 0xf3, PR_NO_ACCESS 0xf4,
// PR_FAILED 0xfe), so Python shows "Sync Now" again. Deciding from the status
// text instead would leave "Cancel Sync" — a button with nothing to cancel.
func TestSyncButtonOffersSyncNowAfterFailure(t *testing.T) {
	t.Parallel()

	// Each terminal failure the sync status can report, paired with the state
	// question the app layer answers (false == no transfer in progress).
	failures := []struct {
		status string
		active bool
	}{
		{"Link establishment failed", false},
		{"No path to node", false},
		{"Sync request failed", false},
		{"Remote got no identity", false},
		{"Node rejected request", false},
		{"Sync failed", false},
		// Controls: the states that really are in flight, and the two
		// PR_COMPLETE wordings the status-text fallback recognizes.
		{"Receiving messages", true},
		{"Path requested", true},
		{"Link established", true},
		{"Done, no new messages", false},
		{"Downloaded 3 new messages", false},
		{"Idle", false},
	}

	for _, c := range failures {
		t.Run(c.status, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			cd := NewConversationsDisplay(app, nil)
			cd.ShowSyncDialog("aabb", nil, SyncDialogHooks{
				Status: func() string { return c.status },
				Active: func() bool { return c.active },
			}, func(SyncDialogResult) {})

			cd.updateSyncProgress()

			want := "Sync Now"
			if c.active {
				want = "Cancel Sync"
			}
			if got := cd.syncSyncBtn.Label(); got != want {
				t.Errorf("button after %q = %q, want %q", c.status, got, want)
			}
		})
	}
}

// TestSyncButtonFallsBackToStatusText pins the fallback a display built without
// the state hook still gets: the two PR_COMPLETE wordings and "Idle" offer Sync
// Now, and anything else reads as in flight. The fallback cannot recognise a
// terminal failure — which is exactly why production wires Active.
func TestSyncButtonFallsBackToStatusText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status string
		want   string
	}{
		{"Idle", "Sync Now"},
		{"Done, no new messages", "Sync Now"},
		{"Downloaded 2 new messages", "Sync Now"},
		{"Receiving messages", "Cancel Sync"},
	}

	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			cd := NewConversationsDisplay(app, nil)
			cd.ShowSyncDialog("aabb", nil, SyncDialogHooks{
				Status: func() string { return c.status },
			}, func(SyncDialogResult) {})

			cd.updateSyncProgress()

			if got := cd.syncSyncBtn.Label(); got != c.want {
				t.Errorf("fallback button after %q = %q, want %q", c.status, got, c.want)
			}
		})
	}
}
