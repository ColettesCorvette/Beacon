package api

import (
	"sync"

	"orbinea.example/core2/http"
	"orbinea.example/core2/websocket"
	"orbinea.example/fluxia/beacon/backend/poshub"
	"orbinea.example/fluxia/nexus/datastore"
)

type Handler struct {
	poshub      *poshub.PosHub
	config     Config
	ds         datastore.Datastore
	wsClient   *websocket.Client
	wsClientMu sync.Mutex
}

type Config struct {
	LocalLedgerAddress     string
	LedgerApiKey           string
	LocalDatabaseAddress string
	LocalDatabaseName    string
}

func NewHandler(poshub *poshub.PosHub, ds datastore.Datastore, config Config) *Handler {
	return &Handler{poshub: poshub, config: config, ds: ds}
}

func (api *Handler) GetPing(c *http.Context) {
	c.String(http.StatusOK, "pong")
}

func (api *Handler) SetWSClient(client *websocket.Client) {
	api.wsClientMu.Lock()
	api.wsClient = client
	api.wsClientMu.Unlock()
}

// sendWS envoie une frame au client WS de supervision s'il y en a un.
// nil-safe : avant ce correctif, un run Puppet déclenché sans client WS
// connecté paniquait dans une goroutine et tuait le process.
func (api *Handler) sendWS(frame websocket.GenericWSFrame) {
	api.wsClientMu.Lock()
	client := api.wsClient
	api.wsClientMu.Unlock()
	if client == nil {
		log.Warningf("aucun client WS connecté, frame %s ignorée", frame.Channel)
		return
	}
	if err := client.SendGeneric(frame); err != nil {
		log.Errorf("envoi frame WS %s: %s", frame.Channel, err)
	}
}
