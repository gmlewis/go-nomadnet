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
	"bufio"
	"fmt"
	"os"
	"strings"
)

// AddInterfaceConfig appends a new [[name]] section under [interfaces] in the
// RNS config file and writes it to disk. Matches Python's config write.
func (a *App) AddInterfaceConfig(name string, props map[string]any) error {
	path := a.RNSConfigPath()
	if path == "" {
		return fmt.Errorf("RNS config path unavailable")
	}

	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")
	hasInterfaces := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "[interfaces]" {
			hasInterfaces = true
			break
		}
	}

	var sb strings.Builder
	if !hasInterfaces {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			sb.WriteString(string(content))
			sb.WriteString("\n")
		} else {
			sb.WriteString(string(content))
		}
		sb.WriteString("[interfaces]\n")
	} else {
		sb.WriteString(string(content))
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			sb.WriteString("\n")
		}
	}

	fmt.Fprintf(&sb, "  [[%v]]\n", name)
	for k, v := range props {
		if k == "name" {
			continue
		}
		fmt.Fprintf(&sb, "    %v = %v\n", k, v)
	}

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// GetInterfaceConfigMap reads the [[name]] section key-value pairs from [interfaces].
func (a *App) GetInterfaceConfigMap(name string) (map[string]any, error) {
	path := a.RNSConfigPath()
	if path == "" {
		return nil, fmt.Errorf("RNS config path unavailable")
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	inInterfaces := false
	inTarget := false
	res := make(map[string]any)

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		if strings.HasPrefix(trimmed, "[[[") && strings.HasSuffix(trimmed, "]]]") {
			inTarget = false
			continue
		}

		if strings.HasPrefix(trimmed, "[[") && strings.HasSuffix(trimmed, "]]") {
			secName := strings.Trim(trimmed, "[] ")
			if inInterfaces && secName == name {
				inTarget = true
			} else {
				inTarget = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			secName := strings.Trim(trimmed, "[] ")
			inInterfaces = (secName == "interfaces")
			inTarget = false
			continue
		}

		if inTarget {
			k, v, ok := strings.Cut(trimmed, "=")
			if ok {
				res[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	if !inTarget && len(res) == 0 {
		// Section not found or empty
		return res, nil
	}

	return res, nil
}

// EditInterfaceConfig updates an existing interface subsection (or renames it).
func (a *App) EditInterfaceConfig(oldName, newName string, props map[string]any) error {
	path := a.RNSConfigPath()
	if path == "" {
		return fmt.Errorf("RNS config path unavailable")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var out []string
	inInterfaces := false
	inTarget := false
	replaced := false

	for i := range len(lines) {
		l := lines[i]
		trimmed := strings.TrimSpace(l)

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") && !strings.HasPrefix(trimmed, "[[") {
			secName := strings.Trim(trimmed, "[] ")
			inInterfaces = (secName == "interfaces")
			inTarget = false
			out = append(out, l)
			continue
		}

		if inInterfaces && strings.HasPrefix(trimmed, "[[") && strings.HasSuffix(trimmed, "]]") && !strings.HasPrefix(trimmed, "[[[") {
			secName := strings.Trim(trimmed, "[] ")
			if secName == oldName {
				inTarget = true
				replaced = true
				out = append(out, fmt.Sprintf("  [[%v]]", newName))
				for k, v := range props {
					if k == "name" {
						continue
					}
					out = append(out, fmt.Sprintf("    %v = %v", k, v))
				}
				continue
			} else {
				inTarget = false
			}
		}

		if inTarget {
			// Skip lines of old target section until next section or sub-section
			if strings.HasPrefix(trimmed, "[") {
				inTarget = false
				out = append(out, l)
			}
			continue
		}

		out = append(out, l)
	}

	if !replaced {
		return a.AddInterfaceConfig(newName, props)
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}

// RemoveInterfaceConfig removes a [[name]] section under [interfaces].
func (a *App) RemoveInterfaceConfig(name string) error {
	path := a.RNSConfigPath()
	if path == "" {
		return fmt.Errorf("RNS config path unavailable")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var out []string
	inInterfaces := false
	inTarget := false

	for i := range len(lines) {
		l := lines[i]
		trimmed := strings.TrimSpace(l)

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") && !strings.HasPrefix(trimmed, "[[") {
			secName := strings.Trim(trimmed, "[] ")
			inInterfaces = (secName == "interfaces")
			inTarget = false
			out = append(out, l)
			continue
		}

		if inInterfaces && strings.HasPrefix(trimmed, "[[") && strings.HasSuffix(trimmed, "]]") && !strings.HasPrefix(trimmed, "[[[") {
			secName := strings.Trim(trimmed, "[] ")
			if secName == name {
				inTarget = true
				continue
			} else {
				inTarget = false
			}
		}

		if inTarget {
			if strings.HasPrefix(trimmed, "[") {
				inTarget = false
				out = append(out, l)
			}
			continue
		}

		out = append(out, l)
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}
