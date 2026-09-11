package micron

import "testing"

// TestDefaultIndexLinkParses verifies the wasm-demo link in the default
// index page parses as a clickable link with the correct URL (entered from
// formatting mode, matching the existing rrc:// join link's markup).
func TestDefaultIndexLinkParses(t *testing.T) {
	line := "  `F79d`_`[View the live wasm demo`c7d0e7bbd883e595f53e14fa6986188c:/page/dynamic-page.wasm]`_`f"
	nodes := Parse(line)
	for _, n := range nodes {
		if n.Type == NodeLink {
			if n.LinkURL != "c7d0e7bbd883e595f53e14fa6986188c:/page/dynamic-page.wasm" {
				t.Errorf("link URL = %q, want the hub page URL", n.LinkURL)
			}
			if n.LinkLabel != "View the live wasm demo" {
				t.Errorf("link label = %q, want the demo label", n.LinkLabel)
			}
			return
		}
	}
	t.Fatalf("no link node found in %q", line)
}
