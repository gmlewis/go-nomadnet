package tui

import (
	"strings"
	"testing"
)

func TestGuideShowTopicSetsReaderText(t *testing.T) {
	app := newTestApp()
	app.Theme = 0
	gd := NewGuideDisplay(app)
	gd.showTopic(0)
	got := gd.reader.GetText(true)
	t.Logf("reader text after showTopic(0):\n%s", got)
	if strings.TrimSpace(got) == "No topic selected" {
		t.Fatalf("reader still at placeholder after showTopic(0)")
	}
	if !strings.Contains(got, "Nomad Network") {
		t.Errorf("reader text missing 'Nomad Network': %q", got)
	}
}
