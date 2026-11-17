package gather

import "testing"

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		ref       string
		wantName  string
		wantTag   string
	}{
		{"mongo:4.4", "mongo", "4.4"},
		{"alpha.srv.orbinea:5000/ledger:2026.20.5", "alpha.srv.orbinea:5000/ledger", "2026.20.5"},
		{"eclipse-mosquitto", "eclipse-mosquitto", "latest"},
		{"alpha.srv.orbinea:5000/gatectl", "alpha.srv.orbinea:5000/gatectl", "latest"},
	}
	for _, tt := range tests {
		name, tag := parseImageRef(tt.ref)
		if name != tt.wantName || tag != tt.wantTag {
			t.Errorf("parseImageRef(%q) = %q,%q want %q,%q", tt.ref, name, tag, tt.wantName, tt.wantTag)
		}
	}
}
