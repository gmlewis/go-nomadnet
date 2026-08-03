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

package browser

import (
	"reflect"
	"testing"
)

// TestExtractPartialsGolden pins ExtractPartials against golden values captured
// from the installed Python nomadnet MicronParser.parse_partial (MicronParser.py:
// 149-195) + the partial_hash computation (line 189,
// RNS.hexrep(RNS.Identity.full_hash(descriptor), delimit=False), SHA-256 → 64
// hex). The Go micron parser already pins parse_partial's field/ID/refresh/
// descriptor logic; this test pins the assembled Partial + the hash together.
// The capture script ran parse_partial on the content AFTER the leading "`{"
// (i.e. "<rest>}"), matching how both Python and Go invoke it (line[2:]).
func TestExtractPartialsGolden(t *testing.T) {
	t.Parallel()
	// Each case is one "`{...}" line in the markup.
	cases := []struct {
		name   string
		markup string
		want   []Partial
	}{
		{
			"url only",
			"`{url_path}",
			[]Partial{{URL: "url_path", Fields: []string{""}, Refresh: 0, Descriptor: "url_path", Raw: "`{url_path}",
				Hash: "c409790e2b7a9a2df61c51877174dbf9b107cb53b9e52fb1c70926651aa6a2da"}},
		},
		{
			"url refresh 5",
			"`{url_path`5}",
			[]Partial{{URL: "url_path", Fields: []string{""}, Refresh: 5, Descriptor: "url_path|5", Raw: "`{url_path`5}",
				Hash: "832d68ce9c4cafd48e6b9f8a1df78a0edb7c853a16abf59b7de1d4fb5c78a358"}},
		},
		{
			"url refresh fields",
			"`{url_path`5`f1|f2}",
			[]Partial{{URL: "url_path", Fields: []string{"f1", "f2"}, Refresh: 5, Descriptor: "url_path|5|f1|f2", Raw: "`{url_path`5`f1|f2}",
				Hash: "fe275df23c5fc269c9b457b665bbb88d5a9a571ac735d5be7cdec1c90ce13615"}},
		},
		{
			"refresh 0 treated as none",
			"`{url_path`0}",
			[]Partial{{URL: "url_path", Fields: []string{""}, Refresh: 0, Descriptor: "url_path|0", Raw: "`{url_path`0}",
				Hash: "458604ddb740effcda91974403d2e6dd64ba4a0e71660ecbbb4473007188842b"}},
		},
		{
			"pid field",
			"`{url_path`2`pid=myid}",
			[]Partial{{URL: "url_path", Fields: []string{"pid=myid"}, ID: "myid", Refresh: 2, Descriptor: "url_path|2|pid=myid", Raw: "`{url_path`2`pid=myid}",
				Hash: "637c3cd50dea606ea083c0e39e1dbbc50b9cc20c6d5886729e3ad70695fac086"}},
		},
		{
			"relative url with fields",
			"`{a:b`10`x=1|y=2|pid=p3}",
			[]Partial{{URL: "a:b", Fields: []string{"x=1", "y=2", "pid=p3"}, ID: "p3", Refresh: 10, Descriptor: "a:b|10|x=1|y=2|pid=p3", Raw: "`{a:b`10`x=1|y=2|pid=p3}",
				Hash: "51064faa49f6f84035a23fb681f0393f83e89053b24151c9ef739bf6d0b20868"}},
		},
		{
			"four components yields no partial",
			"`{url_path`5`pid=abc`f1}",
			nil,
		},
		{
			"empty url yields no partial",
			"`{`5`f1|f2}",
			nil,
		},
		{
			"no partials in markup",
			">Heading\nSome text\n",
			nil,
		},
		{
			"two partials in one page",
			">Title\n`{a`1}\n`{b`2`pid=zz}\n",
			nil, // asserted structurally in the dedicated subtest below
		},
	}

	// The "two partials" case's hashes are computed by the implementation; assert
	// structurally rather than hardcoding (the single-partial cases pin the
	// hash golden values).
	for _, c := range cases {
		if c.name == "two partials in one page" {
			continue
		}
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := ExtractPartials(c.markup)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ExtractPartials(%q) = %+v, want %+v", c.markup, got, c.want)
			}
		})
	}

	t.Run("two partials in one page", func(t *testing.T) {
		got := ExtractPartials("`{a`1}\n`{b`2`pid=zz}\n")
		if len(got) != 2 {
			t.Fatalf("got %v partials, want 2: %+v", len(got), got)
		}
		if got[0].URL != "a" || got[0].Refresh != 1 || got[0].Descriptor != "a|1" {
			t.Errorf("first partial = %+v, want URL=a Refresh=1 Descriptor=a|1", got[0])
		}
		if got[1].URL != "b" || got[1].ID != "zz" || got[1].Refresh != 2 || got[1].Descriptor != "b|2|pid=zz" {
			t.Errorf("second partial = %+v, want URL=b ID=zz Refresh=2", got[1])
		}
		// Hashes must be the SHA-256 of the respective descriptors.
		if got[0].Hash != PartialHash("a|1") {
			t.Errorf("first hash = %q, want %q", got[0].Hash, PartialHash("a|1"))
		}
		if got[1].Hash != PartialHash("b|2|pid=zz") {
			t.Errorf("second hash = %q, want %q", got[1].Hash, PartialHash("b|2|pid=zz"))
		}
	})
}

// TestPartialHash pins the SHA-256 → 64-hex mapping against the Python golden
// values (RNS.hexrep(RNS.Identity.full_hash(descriptor))).
func TestPartialHash(t *testing.T) {
	t.Parallel()
	cases := []struct {
		descriptor string
		want       string
	}{
		{"url_path", "c409790e2b7a9a2df61c51877174dbf9b107cb53b9e52fb1c70926651aa6a2da"},
		{"url_path|5", "832d68ce9c4cafd48e6b9f8a1df78a0edb7c853a16abf59b7de1d4fb5c78a358"},
		{"a:b|10|x=1|y=2|pid=p3", "51064faa49f6f84035a23fb681f0393f83e89053b24151c9ef739bf6d0b20868"},
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.descriptor, func(t *testing.T) {
			if got := PartialHash(c.descriptor); got != c.want {
				t.Errorf("PartialHash(%q) = %q, want %q", c.descriptor, got, c.want)
			}
		})
	}
}

// TestPartialRequestData pins the var_* mapping + linkFields extraction against
// the pure-logic part of Python Browser.__get_partial_request_data
// (Browser.py:763-777). The form-widget field_* collection is TUI glue and is
// NOT covered here (documented in PartialRequestData).
func TestPartialRequestData(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		fields    []string
		wantRD    map[string]string
		wantLinks []string
	}{
		{"empty fields", []string{""}, map[string]string{}, []string{""}},
		{"no fields", nil, map[string]string{}, nil},
		{"var entries", []string{"x=1", "y=2"}, map[string]string{"var_x": "1", "var_y": "2"}, nil},
		{"link fields only", []string{"name", "addr"}, map[string]string{}, []string{"name", "addr"}},
		{"mixed", []string{"x=1", "name", "y=2", "pid=p3"},
			map[string]string{"var_x": "1", "var_pid": "p3", "var_y": "2"}, []string{"name"}},
		{"bad equals dropped", []string{"a=1=2", "noeq", "b=3"},
			map[string]string{"var_b": "3"}, []string{"noeq"}},
		{"pid entry is a var too", []string{"pid=abc"}, map[string]string{"var_pid": "abc"}, nil},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rd, links := PartialRequestData(c.fields)
			if !reflect.DeepEqual(rd, c.wantRD) {
				t.Errorf("requestData = %v, want %v", rd, c.wantRD)
			}
			if !reflect.DeepEqual(links, c.wantLinks) {
				t.Errorf("linkFields = %v, want %v", links, c.wantLinks)
			}
		})
	}
}
