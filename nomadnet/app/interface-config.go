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
	"sort"
	"strings"
)

// ConfigProp is one key/value pair of an interface's [[name]] section, held in
// the order the Add/Edit form displays the field. A slice rather than a map
// because the section is written to the config file in that order: Python builds
// the section from an ordered dict and ConfigObj writes it in insertion order,
// so a map here would rewrite every interface's section in a different order on
// every save.
type ConfigProp struct {
	Key   string
	Value any
}

// configPropValue renders a property the way RNS config files spell it: a bool
// becomes True/False (ConfigObj's spelling), anything else its default form.
func configPropValue(v any) string {
	if b, ok := v.(bool); ok {
		return boolConfigWord(b)
	}
	return fmt.Sprintf("%v", v)
}

// AddInterfaceConfig appends a new [[name]] section under [interfaces] in the
// RNS config file and writes it to disk. The section's keys are written in the
// order given by props, after "type"/"interface_enabled" when those are present,
// matching Python's config write.
func (a *App) AddInterfaceConfig(name string, props []ConfigProp) error {
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
	writeConfigProps(&sb, props, "    ")

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// writeConfigProps writes an interface section's properties, in order. A value
// that is a map is written as a [[[subsection]]] with its own properties one
// level deeper, which is how a config file spells a nested section and how
// RNodeMultiInterface stores its sub-interfaces (Python hands ConfigObj a dict
// value and it writes the subsection).
func writeConfigProps(sb *strings.Builder, props []ConfigProp, indent string) {
	for _, p := range props {
		if p.Key == "name" {
			continue
		}
		if sub, ok := p.Value.(map[string]any); ok {
			fmt.Fprintf(sb, "%v[[[%v]]]\n", indent, p.Key)
			nested := make([]ConfigProp, 0, len(sub))
			for _, k := range sortedConfigKeys(sub) {
				nested = append(nested, ConfigProp{Key: k, Value: sub[k]})
			}
			writeConfigProps(sb, nested, indent+"  ")
			continue
		}
		fmt.Fprintf(sb, "%v%v = %v\n", indent, p.Key, configPropValue(p.Value))
	}
}

// sortedConfigKeys returns a map's keys in sorted order, so a nested section is
// written the same way every time.
func sortedConfigKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// GetInterfaceConfigMap reads the [[name]] section of [interfaces] as a
// property map. A [[[subsection]]] of that section is read back as a nested
// map under its own name, which is the shape a config file gives a nested
// section and the shape RNodeMultiInterface's sub-interfaces are configured in;
// reading them flat would drop them, so an edit would silently delete them.
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
	subName := "" // the [[[subsection]]] being read, "" at the section's own level
	res := make(map[string]any)

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		if strings.HasPrefix(trimmed, "[[[") && strings.HasSuffix(trimmed, "]]]") {
			if inTarget {
				subName = strings.Trim(trimmed, "[] ")
				res[subName] = map[string]any{}
			}
			continue
		}

		if strings.HasPrefix(trimmed, "[[") && strings.HasSuffix(trimmed, "]]") {
			secName := strings.Trim(trimmed, "[] ")
			subName = ""
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
			subName = ""
			continue
		}

		if inTarget {
			k, v, ok := strings.Cut(trimmed, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			if subName != "" {
				if sub, ok := res[subName].(map[string]any); ok {
					sub[k] = v
				}
				continue
			}
			res[k] = v
		}
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	if !inTarget && len(res) == 0 {
		return res, nil
	}

	return res, nil
}

// EditInterfaceConfig updates an existing interface subsection (or renames it).
// The replacement's keys are written in the order given by props.
func (a *App) EditInterfaceConfig(oldName, newName string, props []ConfigProp) error {
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
				var sb strings.Builder
				fmt.Fprintf(&sb, "  [[%v]]\n", newName)
				writeConfigProps(&sb, props, "    ")
				out = append(out, strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n")...)
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
