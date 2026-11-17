package gather

import "testing"

func TestParseCPUStat(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantIdle  uint64
		wantTotal uint64
	}{
		{
			name:      "ligne cpu standard",
			content:   "cpu  10 20 30 40 50\ncpu0 1 2 3 4 5\n",
			wantIdle:  40,
			wantTotal: 150,
		},
		{
			name:      "contenu vide (proc absent)",
			content:   "",
			wantIdle:  0,
			wantTotal: 0,
		},
		{
			name:      "lignes vides au milieu",
			content:   "\n\ncpu  1 2 3 4\n",
			wantIdle:  4,
			wantTotal: 10,
		},
	}
	for _, tt := range tests {
		idle, total := parseCPUStat(tt.content)
		if idle != tt.wantIdle || total != tt.wantTotal {
			t.Errorf("%s: parseCPUStat = %d,%d want %d,%d", tt.name, idle, total, tt.wantIdle, tt.wantTotal)
		}
	}
}
