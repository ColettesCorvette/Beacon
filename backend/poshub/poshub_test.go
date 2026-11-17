package poshub

import (
	"testing"

	central "orbinea.example/fluxia/nexus"
)

// Doit être lancé avec -race : lecture concurrente de la map des caisses
// connectées pendant des écritures.
func TestConcurrentMapAccess(t *testing.T) {
	hub := New(nil, nil)
	id := central.UUID("test-cr")
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			hub.setConnectedCashregister(id, ConnectedCashregister{ID: id})
			hub.deleteConnectedCashregister(id)
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		hub.IsOnline(id)
		hub.getConnected(id)
		hub.getCashregisterFromWSClient(nil)
	}
	<-done
}
