package gather

import (
	"strings"
	"testing"
)

func TestParsePuppetLastRun(t *testing.T) {
	summary := `---
version:
  config: 1717427100
  puppet: 7.26.0
resources:
  changed: 0
  total: 142
time:
  last_run: 1717427112
  total: 12.3
`
	if got := parsePuppetLastRun(strings.NewReader(summary)); got != 1717427112 {
		t.Fatalf("expected 1717427112, got %v", got)
	}

	if got := parsePuppetLastRun(strings.NewReader("---\nresources:\n  total: 1\n")); got != 0 {
		t.Fatalf("expected 0 when last_run is absent, got %v", got)
	}
}
