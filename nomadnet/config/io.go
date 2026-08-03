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

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load reads and parses a NomadNet INI-style config file.
// If the file doesn't exist, it returns a default config.
func Load(path string) (*Config, error) {
	c := DefaultConfig()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, fmt.Errorf("opening config: %w", err)
	}
	defer func() { _ = f.Close() }()

	c.Raw = parseINI(f)
	c.Apply()
	return c, nil
}

// Save writes the config to the specified path in INI format.
func Save(c *Config, path string) (retErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("closing config file: %w", cerr)
		}
	}()

	w := bufio.NewWriter(f)
	defer func() {
		if ferr := w.Flush(); ferr != nil && retErr == nil {
			retErr = fmt.Errorf("flushing config file: %w", ferr)
		}
	}()

	writeSection := func(name string, keys map[string]string) {
		_, _ = fmt.Fprintf(w, "[%s]\n", name)
		for k, v := range keys {
			_, _ = fmt.Fprintf(w, "%s = %s\n", k, v)
		}
		_, _ = fmt.Fprintln(w)
	}

	writeSection("logging", map[string]string{
		"loglevel":    fmt.Sprintf("%v", c.Logging.LogLevel),
		"destination": c.Logging.Destination,
	})

	clientKeys := map[string]string{
		"enable_client":                boolStr(c.Client.EnableClient),
		"user_interface":               c.Client.UserInterface,
		"downloads_path":               c.Client.DownloadsPath,
		"notify_on_new_message":        boolStr(c.Client.NotifyOnNewMessage),
		"announce_at_start":            boolStr(c.Client.AnnounceAtStart),
		"announce_interval":            fmt.Sprintf("%v", c.Client.AnnounceInterval/60),
		"try_propagation_on_send_fail": boolStr(c.Client.TryPropagationOnSendFail),
		"periodic_lxmf_sync":           boolStr(c.Client.PeriodicLXMFSync),
		"lxmf_sync_interval":           fmt.Sprintf("%v", c.Client.LXMFSyncInterval/60),
		"lxmf_sync_limit":              fmt.Sprintf("%v", c.Client.LXMFSyncLimit),
		"accept_invalid_stamps":        boolStr(c.Client.AcceptInvalidStamps),
		"max_accepted_size":            fmt.Sprintf("%.0f", c.Client.MaxAcceptedSize),
		"compact_announce_stream":      boolStr(c.Client.CompactAnnounceStream),
		"compose_in_markdown":          boolStr(c.Client.ComposeInMarkdown),
	}
	if c.Client.RequiredStampCost != nil {
		clientKeys["required_stamp_cost"] = fmt.Sprintf("%v", *c.Client.RequiredStampCost)
	} else {
		clientKeys["required_stamp_cost"] = "None"
	}
	writeSection("client", clientKeys)

	writeSection("textui", map[string]string{
		"intro_time":     fmt.Sprintf("%.1f", c.TextUI.IntroTime),
		"theme":          c.TextUI.Theme,
		"colormode":      c.TextUI.ColorMode,
		"glyphs":         c.TextUI.Glyphs,
		"mouse_enabled":  boolStr(c.TextUI.MouseEnabled),
		"editor":         c.TextUI.Editor,
		"hide_guide":     boolStr(c.TextUI.HideGuide),
		"sanitize_names": boolStr(c.TextUI.SanitizeNames),
		"clipboard_copy": boolStr(c.TextUI.ClipboardCopy),
	})

	rrcKeys := map[string]string{
		"history_per_room_cap":     fmt.Sprintf("%v", c.RRC.HistoryPerRoomCap),
		"filter_loaded_history":    boolStr(c.RRC.FilterLoadedHistory),
		"ephemeral_notices":        fmt.Sprintf("%.0f", c.RRC.EphemeralNotices),
		"color_mention_timestamps": boolStr(c.RRC.ColorMentionTimestamps),
		"render_markdown":          boolStr(c.RRC.RenderMarkdown),
		"render_micron":            boolStr(c.RRC.RenderMicron),
		"nick_colors":              boolStr(c.RRC.NickColors),
		"justify_msgs":             boolStr(c.RRC.JustifyMsgs),
		"space_msgs":               boolStr(c.RRC.SpaceMsgs),
		"show_gutters":             boolStr(c.RRC.ShowGutters),
		"enable_esoterics":         boolStr(c.RRC.EnableEsoterics),
	}
	if c.RRC.MentionColor != "" {
		rrcKeys["mention_color"] = c.RRC.MentionColor
	}
	if len(c.RRC.NickColorsTheme) > 0 {
		rrcKeys["nick_colors_theme"] = strings.Join(c.RRC.NickColorsTheme, ", ")
	}
	writeSection("rrc", rrcKeys)

	nodeKeys := map[string]string{
		"enable_node":           boolStr(c.Node.EnableNode),
		"announce_interval":     fmt.Sprintf("%v", c.Node.AnnounceInterval/60),
		"announce_at_start":     boolStr(c.Node.AnnounceAtStart),
		"disable_propagation":   boolStr(c.Node.DisablePropagation),
		"propagation_cost":      fmt.Sprintf("%v", c.Node.PropagationCost),
		"max_transfer_size":     fmt.Sprintf("%.0f", c.Node.MaxTransferSize),
		"max_sync_size":         fmt.Sprintf("%.0f", c.Node.MaxSyncSize),
		"page_refresh_interval": fmt.Sprintf("%v", c.Node.PageRefreshInterval),
		"file_refresh_interval": fmt.Sprintf("%v", c.Node.FileRefreshInterval),
		"message_storage_limit": fmt.Sprintf("%.0f", c.Node.MessageStorageLimit),
	}
	if c.Node.NodeName != "" {
		nodeKeys["node_name"] = c.Node.NodeName
	}
	if c.Node.PagesPath != "" {
		nodeKeys["pages_path"] = c.Node.PagesPath
	}
	if c.Node.FilesPath != "" {
		nodeKeys["files_path"] = c.Node.FilesPath
	}
	if len(c.Node.PrioritiseDestinations) > 0 {
		nodeKeys["prioritise_destinations"] = strings.Join(c.Node.PrioritiseDestinations, ", ")
	}
	if len(c.Node.StaticPeers) > 0 {
		nodeKeys["static_peers"] = strings.Join(c.Node.StaticPeers, ", ")
	}
	if c.Node.MaxPeers != nil {
		nodeKeys["max_peers"] = fmt.Sprintf("%v", *c.Node.MaxPeers)
	}
	writeSection("node", nodeKeys)

	printKeys := map[string]string{
		"print_messages": boolStr(c.Printing.PrintMessages),
		"print_command":  c.Printing.PrintCommand,
	}
	if c.Printing.PrintFrom != "" {
		printKeys["print_from"] = c.Printing.PrintFrom
	}
	writeSection("printing", printKeys)

	return nil
}

// boolStr converts a boolean to a "yes"/"no" string.
func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// parseINI reads an INI-style config file and returns a map of
// section → key → value pairs. Comments (lines starting with #)
// and blank lines are skipped.
func parseINI(f *os.File) map[string]map[string]string {
	result := make(map[string]map[string]string)
	var currentSection string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = line[1 : len(line)-1]
			if result[currentSection] == nil {
				result[currentSection] = make(map[string]string)
			}
			continue
		}

		// Key = Value
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			if currentSection != "" {
				result[currentSection][key] = value
			}
		}
	}

	return result
}
