package api

import "testing"

func TestUpdateFactsContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		key     string
		value   string
		want    string
	}{
		{"remplace la valeur", "role=server\nsite=villefranche\n", "role", "device", "role=device\nsite=villefranche\n"},
		{"ajoute si absente", "role=server\n", "site", "villefranche", "role=server\nsite=villefranche\n"},
		{"pas de faux préfixe", "roleplay=x\n", "role", "server", "roleplay=x\nrole=server\n"},
		{"fichier vide", "", "role", "server", "role=server\n"},
	}
	for _, tt := range tests {
		got := updateFactsContent(tt.content, tt.key, tt.value)
		if got != tt.want {
			t.Errorf("%s: updateFactsContent = %q want %q", tt.name, got, tt.want)
		}
	}
}
