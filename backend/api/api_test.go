package api

import (
	"testing"

	"orbinea.example/core2/websocket"
)

// sendWS sans client connecté ne doit ni paniquer ni bloquer.
func TestSendWSNilClient(t *testing.T) {
	h := NewHandler(nil, nil, Config{})
	h.sendWS(websocket.GenericWSFrame{Channel: "test"}) // panique avant correctif
}
