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
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The examples tree is embedded so a fresh install can materialise it without
// depending on a source checkout. Upstream copies its packaged examples
// directory on first run (NomadNetworkApp.py:215-223); this is the equivalent
// here, with the example content adapted for the Go port: upstream's
// messageboard page is executable Python, which this port does not run.
//
//go:embed examples
var examplesFS embed.FS

// copyExamplesIfAbsent materialises the embedded examples tree at ExamplesPath
// when that directory does not exist. An existing directory is left untouched,
// so a returning user's edits are never overwritten — matching Python's
// `if not os.path.isdir(self.examplespath): shutil.copytree(...)`.
func (a *App) copyExamplesIfAbsent() error {
	if _, err := os.Stat(a.ExamplesPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	const root = "examples"
	return fs.WalkDir(examplesFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		target := filepath.Join(a.ExamplesPath, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := examplesFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded example %v: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
