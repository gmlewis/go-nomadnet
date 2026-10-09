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

package main

import "os"

// FileMode is the mode a file the editor creates is given. It is the mode a
// configuration file is written with everywhere else in this repository.
const FileMode = 0o644

// OSFiles is the [Files] the editor uses on a real disk.
type OSFiles struct{}

// Read returns the file's contents, and reports a file that is not there as an
// empty one.
//
// A missing file is not a failure here because the paths an editor is started
// on include ones that do not exist yet: an operator editing a configuration
// that has never been written is editing an empty file, and the save creates
// it. Every other failure — a directory, a permission, an unreadable disk — is
// returned, because opening one of those as "empty" would be an editor whose
// first save destroyed what it could not read.
func (OSFiles) Read(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// Write replaces the file's contents.
//
// It goes through a staging file beside the destination and a rename, so that a
// process killed mid-write leaves the previous contents rather than half of the
// new ones: an editor is the one program whose interrupted write is somebody's
// work. It is the same rule the appliance's own writes follow.
func (OSFiles) Write(path, text string) error {
	staging := path + ".new"
	if err := os.WriteFile(staging, []byte(text), FileMode); err != nil {
		return err
	}
	if err := os.Rename(staging, path); err != nil {
		_ = os.Remove(staging)
		return err
	}
	return nil
}
