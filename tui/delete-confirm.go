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

// ConfirmState represents the state of a confirmation dialog.
type ConfirmState int

const (
	// ConfirmPending means the dialog is awaiting user input.
	ConfirmPending ConfirmState = iota
	// ConfirmAccepted means the user confirmed the action.
	ConfirmAccepted
	// ConfirmRejected means the user cancelled the action.
	ConfirmRejected
)

// DeleteConfirm manages the state of a delete confirmation dialog.
// This is a reusable state machine for the various confirmation
// dialogs in the TUI (delete node, delete hub, delete conversation, etc.).
type DeleteConfirm struct {
	message string
	target  string
	state   ConfirmState
}

// NewDeleteConfirm creates a new pending confirmation dialog.
func NewDeleteConfirm(message, target string) *DeleteConfirm {
	return &DeleteConfirm{
		message: message,
		target:  target,
		state:   ConfirmPending,
	}
}

// Message returns the confirmation message.
func (dc *DeleteConfirm) Message() string {
	return dc.message
}

// Target returns the identifier of the item being deleted.
func (dc *DeleteConfirm) Target() string {
	return dc.target
}

// State returns the current confirmation state.
func (dc *DeleteConfirm) State() ConfirmState {
	return dc.state
}

// Accept marks the confirmation as accepted.
func (dc *DeleteConfirm) Accept() {
	dc.state = ConfirmAccepted
}

// Reject marks the confirmation as rejected.
func (dc *DeleteConfirm) Reject() {
	dc.state = ConfirmRejected
}

// Reset returns the dialog to pending state.
func (dc *DeleteConfirm) Reset() {
	dc.state = ConfirmPending
}

// IsResolved returns true if the dialog has been accepted or rejected.
func (dc *DeleteConfirm) IsResolved() bool {
	return dc.state != ConfirmPending
}

// IsAccepted returns true if the dialog was accepted.
func (dc *DeleteConfirm) IsAccepted() bool {
	return dc.state == ConfirmAccepted
}
