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
	"io"
	"os"
)

// NotifyMessageReceived fires the new-message notification. In text-UI mode it
// writes an ASCII bell character to stdout, mirroring the Python NomadNet
// notify_message_recieved. Tests may override the destination by setting the
// unexported notifyWriter field.
func (a *App) NotifyMessageReceived() {
	if a.UIMode != UIText {
		return
	}
	w := a.notifyWriter
	if w == nil {
		w = os.Stdout
	}
	if _, err := io.WriteString(w, "\a"); err != nil {
		_ = err
	}
}
