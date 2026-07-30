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
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
)

func TestCreateDefaultConfig(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if err := a.CreateDefaultConfig(); err != nil {
		t.Fatal(err)
	}
	if a.Config == nil {
		t.Fatal("Config should be set")
	}
	if _, err := os.Stat(a.ConfigPath); os.IsNotExist(err) {
		t.Fatal("config file should exist on disk")
	}
	// reload and verify defaults applied
	cfg, err := config.Load(a.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Client.EnableClient {
		t.Error("EnableClient should be true")
	}
	// applyConfig should have populated App fields
	if !a.EnableClient {
		t.Error("applyConfig should set EnableClient")
	}
}
