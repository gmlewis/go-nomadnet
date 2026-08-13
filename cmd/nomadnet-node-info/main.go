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

// Command nomadnet-node-info looks up a previously announced node by name or
// destination address and prints everything the local Reticulum instance knows
// about it — entirely offline. It reads only the on-disk announce history under
// ~/.reticulum/storage (the known_destinations and destination_table files that
// Reticulum writes as it hears announces); it never starts interfaces or sends
// any packets.
//
// Usage:
//
//	go run ./cmd/nomadnet-node-info 'ICP Board'
//	go run ./cmd/nomadnet-node-info 1bf29468f7d10cfed65c7d0fd9717634
//	go run ./cmd/nomadnet-node-info 1bf29468            # hash prefix is enough
//	go run ./cmd/nomadnet-node-info --rnsconfig /path/to/reticulum 'ICP Board'
//	go run ./cmd/nomadnet-node-info --json 'ICP Board'
//
// The query is matched case-insensitively against every cached announce's
// display name (substring) and, when it looks like hex, against the destination
// hash and the announcing identity's hash (prefix match). All matches are
// printed: the decoded name, destination and identity hashes, last-seen time,
// packet hash, full public key, raw app data, the path-table entry (hops, next
// hop, expiry, interface hash) when present, and any other destinations
// announced by the same identity.
//
// Display names come from the announce app_data. NomadNet node announces store
// the raw UTF-8 name; LXMF peer/propagation announces use a msgpack list whose
// first element is the name. The app name itself (e.g. "nomadnetwork.node" vs
// "lxmf.delivery") is not recoverable from the hashed destination address, so
// the kind is inferred from the app_data shape.
//
// Flags:
//
//	--rnsconfig DIR   Reticulum config dir (default: ~/.reticulum)
//	--json            emit machine-readable JSON instead of text
//	-h, --help        show help and exit
package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/msgpack"
)

func main() {
	var (
		configDir string
		jsonOut   bool
		help      bool
	)
	flag.StringVar(&configDir, "rnsconfig", "", "Reticulum config dir (default: ~/.reticulum)")
	flag.BoolVar(&jsonOut, "json", false, "emit machine-readable JSON instead of text")
	flag.BoolVar(&help, "help", false, "show help and exit")
	flag.Usage = usage
	flag.Parse()

	if help || flag.NArg() == 0 {
		flag.Usage()
		if help {
			os.Exit(0)
		}
		os.Exit(2)
	}

	query := strings.Join(flag.Args(), " ")

	storage, err := resolveStorage(configDir)
	if err != nil {
		fatal("could not locate Reticulum storage: %v", err)
	}

	known, err := loadKnownDestinations(storage)
	if err != nil {
		fatal("reading known_destinations: %v", err)
	}
	paths, err := loadPathTable(storage)
	if err != nil {
		// A missing/unreadable path table is not fatal; we just have no path info.
		fmt.Fprintf(os.Stderr, "warning: reading destination_table: %v\n", err)
		paths = nil
	}

	matches := search(known, paths, query)
	if len(matches) == 0 {
		fmt.Printf("No entries matched %q.\n", query)
		os.Exit(1)
	}

	if jsonOut {
		if err := emitJSON(matches); err != nil {
			fatal("%v", err)
		}
		return
	}
	emitText(matches, query)
}

// entry holds everything known about a single announced destination.
type entry struct {
	DestHash     []byte
	IdentityHash []byte
	Name         string
	Kind         string
	AppData      []byte
	LastSeen     time.Time
	PacketHash   []byte
	PublicKey    []byte
	Path         *pathInfo
	// SameIdentity lists the other destinations announced by the same identity
	// (same public key), decoded the same way as the match.
	SameIdentity []sibling
}

type sibling struct {
	DestHash []byte
	Name     string
	Kind     string
}

// pathInfo is the subset of a destination_table row we surface.
type pathInfo struct {
	Hops      int
	NextHop   []byte
	Interface []byte
	Expires   time.Time
}

// jsonEntry is the JSON-facing projection of entry: byte slices become
// lowercase hex strings (so the "_hex"-suffixed field names are truthful),
// since encoding/json would otherwise marshal []byte as base64.
type jsonEntry struct {
	DestinationHash string        `json:"destination_hash"`
	IdentityHash    string        `json:"identity_hash"`
	Name            string        `json:"name"`
	Kind            string        `json:"kind"`
	AppDataHex      string        `json:"app_data_hex"`
	LastSeen        time.Time     `json:"last_seen"`
	PacketHash      string        `json:"packet_hash"`
	PublicKey       string        `json:"public_key"`
	Path            *jsonPath     `json:"path,omitempty"`
	SameIdentity    []jsonSibling `json:"same_identity,omitempty"`
}

type jsonSibling struct {
	DestinationHash string `json:"destination_hash"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
}

type jsonPath struct {
	Hops         int       `json:"hops"`
	NextHop      string    `json:"next_hop"`
	InterfaceHex string    `json:"interface_hash"`
	Expires      time.Time `json:"expires"`
}

func (e *entry) toJSON() jsonEntry {
	j := jsonEntry{
		DestinationHash: hex.EncodeToString(e.DestHash),
		IdentityHash:    hex.EncodeToString(e.IdentityHash),
		Name:            e.Name,
		Kind:            e.Kind,
		AppDataHex:      hex.EncodeToString(e.AppData),
		LastSeen:        e.LastSeen,
		PacketHash:      hex.EncodeToString(e.PacketHash),
		PublicKey:       hex.EncodeToString(e.PublicKey),
	}
	if e.Path != nil {
		j.Path = &jsonPath{
			Hops:         e.Path.Hops,
			NextHop:      hex.EncodeToString(e.Path.NextHop),
			InterfaceHex: hex.EncodeToString(e.Path.Interface),
			Expires:      e.Path.Expires,
		}
	}
	if len(e.SameIdentity) > 0 {
		j.SameIdentity = make([]jsonSibling, len(e.SameIdentity))
		for i, s := range e.SameIdentity {
			j.SameIdentity[i] = jsonSibling{
				DestinationHash: hex.EncodeToString(s.DestHash),
				Name:            s.Name,
				Kind:            s.Kind,
			}
		}
	}
	return j
}

// loadKnownDestinations reads <storage>/known_destinations, a msgpack map of
// destHash -> [timestamp(float s), packetHash, publicKey, appData]. Map keys are
// binary (Python/go-reticulum write bin keys); msgpack.Unpack decodes bin map
// keys as a Go string whose bytes are the raw destination hash, so we accept
// both string and []byte keys to also cover legacy str-keyed files.
func loadKnownDestinations(storage string) (map[string][]any, error) {
	data, err := os.ReadFile(filepath.Join(storage, "known_destinations"))
	if err != nil {
		return nil, err
	}
	unpacked, err := msgpack.Unpack(data)
	if err != nil {
		return nil, fmt.Errorf("unpack: %w", err)
	}
	m, ok := unpacked.(map[any]any)
	if !ok {
		return nil, errors.New("known_destinations is not a msgpack map")
	}
	out := make(map[string][]any, len(m))
	for k, v := range m {
		var destHash []byte
		switch kb := k.(type) {
		case string:
			destHash = []byte(kb)
		case []byte:
			destHash = kb
		default:
			continue
		}
		row, ok := v.([]any)
		if !ok || len(row) < 4 {
			continue
		}
		out[string(destHash)] = row
	}
	return out, nil
}

// loadPathTable reads <storage>/destination_table, a msgpack list of 8-field
// rows: [destHash, timestamp, next_hop, hops, expires, blobs, iface_hash, packet_hash].
func loadPathTable(storage string) (map[string]*pathInfo, error) {
	data, err := os.ReadFile(filepath.Join(storage, "destination_table"))
	if err != nil {
		return nil, err
	}
	unpacked, err := msgpack.Unpack(data)
	if err != nil {
		return nil, fmt.Errorf("unpack: %w", err)
	}
	list, ok := unpacked.([]any)
	if !ok {
		return nil, errors.New("destination_table is not a msgpack list")
	}
	out := make(map[string]*pathInfo, len(list))
	for _, raw := range list {
		fields, ok := raw.([]any)
		if !ok || len(fields) < 8 {
			continue
		}
		destHash, ok := fields[0].([]byte)
		if !ok {
			continue
		}
		hops, _ := toInt64(fields[3])
		nextHop, _ := fields[2].([]byte)
		ifaceHash, _ := fields[6].([]byte)
		expires, _ := toFloatSeconds(fields[4])
		out[string(destHash)] = &pathInfo{
			Hops:      int(hops),
			NextHop:   cloneBytes(nextHop),
			Interface: cloneBytes(ifaceHash),
			Expires:   floatToTime(expires),
		}
	}
	return out, nil
}

// search returns every known destination whose decoded name contains the query
// (case-insensitive) or whose destination/identity hash begins with the query
// when it parses as hex.
func search(known map[string][]any, paths map[string]*pathInfo, query string) []*entry {
	lowerQuery := strings.ToLower(query)
	hexQuery := normalizeHexQuery(query)
	isHex := hexQuery != ""

	// First pass: build all candidate entries so identity-hash prefix matching
	// and same-identity grouping can use the decoded public key.
	all := make([]*entry, 0, len(known))
	for destHashStr, row := range known {
		e := buildEntry([]byte(destHashStr), row, paths)
		all = append(all, e)
	}

	var matched []*entry
	for _, e := range all {
		if matches(e, lowerQuery, hexQuery, isHex) {
			matched = append(matched, e)
		}
	}

	// Attach same-identity siblings to each match.
	for _, e := range matched {
		if len(e.PublicKey) == 0 {
			continue
		}
		for _, other := range all {
			if string(other.DestHash) == string(e.DestHash) {
				continue
			}
			if string(other.PublicKey) == string(e.PublicKey) {
				e.SameIdentity = append(e.SameIdentity, sibling{
					DestHash: other.DestHash,
					Name:     other.Name,
					Kind:     other.Kind,
				})
			}
		}
	}
	return matched
}

// buildEntry decodes one known_destinations row into an entry, attaching its
// path-table entry if one exists.
func buildEntry(destHash []byte, row []any, paths map[string]*pathInfo) *entry {
	e := &entry{DestHash: cloneBytes(destHash)}

	if ts, ok := toFloatSeconds(row[0]); ok {
		e.LastSeen = floatToTime(ts)
	}
	if ph, ok := row[1].([]byte); ok {
		e.PacketHash = cloneBytes(ph)
	}
	if pk, ok := row[2].([]byte); ok {
		e.PublicKey = cloneBytes(pk)
		e.IdentityHash = rns.TruncatedHash(e.PublicKey)
	}
	if ad, ok := row[3].([]byte); ok {
		e.AppData = cloneBytes(ad)
		e.Name, e.Kind = decodeName(e.AppData)
	}
	if p, ok := paths[string(destHash)]; ok {
		pcopy := *p
		e.Path = &pcopy
	}
	return e
}

// matches reports whether e satisfies the name or hash query.
func matches(e *entry, lowerQuery, hexQuery string, isHex bool) bool {
	if isHex {
		if strings.HasPrefix(hex.EncodeToString(e.DestHash), hexQuery) {
			return true
		}
		if len(e.IdentityHash) > 0 && strings.HasPrefix(hex.EncodeToString(e.IdentityHash), hexQuery) {
			return true
		}
		// Fall through: a hex query might also be a name substring, but only if
		// it is not purely a hash lookup that happened to be short.
	}
	if e.Name != "" && strings.Contains(strings.ToLower(e.Name), lowerQuery) {
		return true
	}
	return false
}

// decodeName extracts the display name from announce app_data and classifies the
// app shape. NomadNet node announces carry the raw UTF-8 name; LXMF delivery
// (peer) announces carry a msgpack list whose first element is the name, and
// LXMF propagation-node announces carry a msgpack list whose first element is
// a bool/int (no name).
func decodeName(appData []byte) (name, kind string) {
	if len(appData) == 0 {
		return "", "empty"
	}
	if utf8.Valid(appData) && isPrintableText(appData) {
		return string(appData), "nomadnetwork.node"
	}
	// LXMF app data is a msgpack array. The v0.5.0+ delivery format is
	// [name, ...]; the propagation-node format is [bool, ...] with no name.
	if isMsgpackArray(appData) {
		if n, err := lxmf.DisplayNameFromAppData(appData); err == nil && n != "" {
			return n, "lxmf.delivery"
		}
		return "", "lxmf.propagation"
	}
	return "", "unknown"
}

// isMsgpackArray reports whether b begins a msgpack fixarray or array16/32 tag.
func isMsgpackArray(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	return (b[0] >= 0x90 && b[0] <= 0x9f) || b[0] == 0xdc || b[0] == 0xdd
}

// isPrintableText reports whether b is valid UTF-8 made only of printable runes
// (no C0 control bytes). Emoji and other high-codepoint glyphs are printable.
func isPrintableText(b []byte) bool {
	for _, r := range string(b) {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// normalizeHexQuery returns the lowercased hex query (without any 0x prefix) if q
// parses as even-length hex, else "".
func normalizeHexQuery(q string) string {
	q = strings.TrimSpace(q)
	q = strings.TrimPrefix(q, "0x")
	q = strings.TrimPrefix(q, "0X")
	if len(q) == 0 || len(q)%2 != 0 {
		return ""
	}
	for _, c := range q {
		if !isHexByte(c) {
			return ""
		}
	}
	return strings.ToLower(q)
}

func isHexByte(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// ---- output ---------------------------------------------------------------

func emitText(matches []*entry, query string) {
	fmt.Printf("Found %d %s for %q:\n\n", len(matches), plural(len(matches), "match", "matches"), query)
	for i, e := range matches {
		if i > 0 {
			fmt.Println()
		}
		printEntry(e)
	}
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

func printEntry(e *entry) {
	label := e.Name
	if label == "" {
		label = fmt.Sprintf("%x", e.DestHash)
	}
	fmt.Printf("── %s ──\n", label)

	fmt.Printf("  Name             %s\n", orUnknown(e.Name))
	fmt.Printf("  Inferred kind    %s\n", e.Kind)
	fmt.Printf("  Destination      %x\n", e.DestHash)
	fmt.Printf("  Identity hash    %s\n", hexOrUnknown(e.IdentityHash))
	if !e.LastSeen.IsZero() {
		fmt.Printf("  Last seen        %s (%s)\n", e.LastSeen.Format("2006-01-02 15:04:05"), ago(e.LastSeen))
	} else {
		fmt.Println("  Last seen        unknown")
	}
	if len(e.PacketHash) > 0 {
		fmt.Printf("  Packet hash      %x\n", e.PacketHash)
	}
	if len(e.PublicKey) > 0 {
		fmt.Printf("  Public key       %x\n", e.PublicKey)
	}
	if len(e.AppData) > 0 {
		fmt.Printf("  App data (text)  %s\n", quoteOrUnknown(e.Name))
		fmt.Printf("  App data (hex)   %x\n", e.AppData)
	}

	if e.Path != nil {
		fmt.Printf("  Path             %d %s away via %x\n", e.Path.Hops, plural(e.Path.Hops, "hop", "hops"), e.Path.NextHop)
		if len(e.Path.Interface) > 0 {
			fmt.Printf("  Interface hash   %x\n", e.Path.Interface)
		}
		if !e.Path.Expires.IsZero() {
			fmt.Printf("  Path expires     %s (%s)\n", e.Path.Expires.Format("2006-01-02 15:04:05"), remaining(e.Path.Expires))
		}
	} else {
		fmt.Println("  Path             no destination_table entry (not currently routed)")
	}

	if len(e.SameIdentity) > 0 {
		fmt.Println("  Same identity also announces:")
		for _, s := range e.SameIdentity {
			name := s.Name
			if name == "" {
				name = "<no name>"
			}
			fmt.Printf("    %-18s %x  (%s)\n", s.Kind, s.DestHash, name)
		}
	}
}

func emitJSON(matches []*entry) error {
	out := make([]jsonEntry, len(matches))
	for i, e := range matches {
		out[i] = e.toJSON()
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ---- helpers --------------------------------------------------------------

func resolveStorage(configDir string) (string, error) {
	dir := configDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		// Match rns.chooseConfigDir's preference order: the platform system dir
		// and ~/.config/reticulum win only when they actually contain a config
		// file; otherwise fall back to ~/.reticulum.
		for _, candidate := range []string{
			filepath.Join(home, ".config", "reticulum"),
			filepath.Join(home, ".reticulum"),
		} {
			if hasConfigFile(candidate) {
				dir = candidate
				break
			}
		}
		if dir == "" {
			dir = filepath.Join(home, ".reticulum")
		}
	}
	return filepath.Join(dir, "storage"), nil
}

func hasConfigFile(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "config"))
	return err == nil && !info.IsDir()
}

func toFloatSeconds(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	}
	return 0, false
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	}
	return 0, false
}

func floatToTime(s float64) time.Time {
	if s <= 0 {
		return time.Time{}
	}
	sec := int64(s)
	nsec := int64((s - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).Local()
}

func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func ago(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		return "in the future"
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d %s ago", int(d.Seconds()), plural(int(d.Seconds()), "second", "seconds"))
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%d %s ago", m, plural(m, "minute", "minutes"))
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%d %s ago", h, plural(h, "hour", "hours"))
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d %s ago", days, plural(days, "day", "days"))
	}
}

func remaining(t time.Time) string {
	d := time.Until(t)
	if d < 0 {
		return "expired"
	}
	switch {
	case d < time.Minute:
		s := int(d.Seconds())
		return fmt.Sprintf("in %d %s", s, plural(s, "second", "seconds"))
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("in %d %s", m, plural(m, "minute", "minutes"))
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("in %d %s", h, plural(h, "hour", "hours"))
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("in %d %s", days, plural(days, "day", "days"))
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "<unknown>"
	}
	return s
}

func hexOrUnknown(b []byte) string {
	if len(b) == 0 {
		return "<unknown>"
	}
	return fmt.Sprintf("%x", b)
}

func quoteOrUnknown(s string) string {
	if s == "" {
		return "<none>"
	}
	return strconvQuote(s)
}

// strconvQuote wraps strconv.Quote without importing strconv twice; kept inline
// to avoid pulling another import alias.
func strconvQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "nomadnet-node-info: "+format+"\n", args...)
	os.Exit(1)
}

func usage() {
	out := flag.CommandLine.Output()
	_, _ = fmt.Fprintln(out, "usage: nomadnet-node-info [--rnsconfig DIR] [--json] <name-or-address>")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Searches the local Reticulum announce history (offline) for a node by name")
	_, _ = fmt.Fprintln(out, "(substring, case-insensitive) or destination/identity hash (hex prefix) and")
	_, _ = fmt.Fprintln(out, "prints everything known about each match.")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Flags:")
	flag.PrintDefaults()
}
