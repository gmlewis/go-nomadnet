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

package config

import _ "embed"

// defaultConfigText is the verbatim default NomadNet configuration file, the
// same string Python ships as __default_nomadnet_config__ in NomadNetworkApp.py.
// It is embedded from testdata/default.conf (the canonical copy, also used by
// the ConfigObj parity test) so there is a single source of truth.
//
//go:embed testdata/default.conf
var defaultConfigText string

// DefaultConfigText returns the verbatim default NomadNet configuration file
// content, matching Python's __default_nomadnet_config__. It is written to disk
// on first run so the on-disk file is byte-identical to what Python produces.
func DefaultConfigText() string { return defaultConfigText }
