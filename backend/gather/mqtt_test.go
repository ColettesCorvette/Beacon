package gather

import (
	"strings"
	"testing"
)

func TestParsePersistenceDumpTopicAvecDeuxPoints(t *testing.T) {
	dump := "Mosquitto DB dump\n" +
		"DB_CHUNK_MSG_STORE:\n" +
		"\tStore ID: 42\n" +
		"\tTopic: site/a: b/status\n" +
		"\tPayload Length: 7\n"
	res := parsePersistenceDump(strings.NewReader(dump))
	if len(res.Msg) != 1 {
		t.Fatalf("attendu 1 message, obtenu %d", len(res.Msg))
	}
	if res.Msg[0].Topic != "site/a: b/status" {
		t.Errorf("topic tronqué: %q", res.Msg[0].Topic)
	}
	if res.Msg[0].Sz != 7 {
		t.Errorf("taille: %d want 7", res.Msg[0].Sz)
	}
}
