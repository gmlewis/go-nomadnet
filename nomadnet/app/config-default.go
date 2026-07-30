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
	"os"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
)

// CreateDefaultConfig writes the default NomadNet configuration file to
// ConfigPath, ensures the config directory exists, loads it into the App, and
// applies it to all subsystems. This mirrors the Python NomadNet
// createDefaultConfig: the verbatim __default_nomadnet_config__ string is
// written so the on-disk file is byte-identical to what Python produces.
func (a *App) CreateDefaultConfig() error {
	if err := os.MkdirAll(a.ConfigDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(a.ConfigPath, []byte(config.DefaultConfigText()), 0o644); err != nil {
		return err
	}
	cfg, err := config.Load(a.ConfigPath)
	if err != nil {
		return err
	}
	a.Config = cfg
	a.applyConfig(cfg)
	a.FirstRun = true
	return nil
}
