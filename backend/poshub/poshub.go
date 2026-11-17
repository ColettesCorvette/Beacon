package poshub

import (
	"sync"

	"github.com/pkg/errors"
	"orbinea.example/core2/websocket"
	central "orbinea.example/fluxia/nexus"
	"orbinea.example/fluxia/nexus/datastore"
)

type RedirectWS func(CashregisterProxyWSFrame)

type ConnectedCashregister struct {
	ID       central.UUID
	wsClient *websocket.Client
	wsToken  string
	Address  string
	IP       string
}

type PosHub struct {
	ds                         datastore.Datastore
	connectedCashregisters     map[central.UUID]*ConnectedCashregister
	connectedCashregistersLock sync.Mutex
	wsHub                      *websocket.Hub
	wsHandlers                 map[string]crWSHandler
	mqttClient                 *central.MQClient
	redirectWS                 RedirectWS
}

func New(ds datastore.Datastore, mqttClient *central.MQClient) *PosHub {
	hub := &PosHub{ds: ds, connectedCashregisters: map[central.UUID]*ConnectedCashregister{}, mqttClient: mqttClient, connectedCashregistersLock: sync.Mutex{}}
	hub.initWSHandlers()
	return hub
}

// getConnected retourne une copie de la caisse connectée, sous lock.
// Retourner une copie évite toute lecture non synchronisée des champs
// (wsToken notamment) après relâchement du lock.
func (hub *PosHub) getConnected(id central.UUID) (ConnectedCashregister, bool) {
	hub.connectedCashregistersLock.Lock()
	defer hub.connectedCashregistersLock.Unlock()
	c, ok := hub.connectedCashregisters[id]
	if !ok {
		return ConnectedCashregister{}, false
	}
	return *c, true
}

func (hub *PosHub) IsOnline(id central.UUID) bool {
	_, ok := hub.getConnected(id)
	return ok
}

// Initialize try to establish connection to all cashregisters
func (hub *PosHub) Initialize() error {
	err := hub.InitializeWS()
	if err != nil {
		return errors.Wrap(err, "failed to initialize websocket")
	}

	err = hub.InitializeMqtt()
	return err
}

func (hub *PosHub) AttachRedirect(redirectWS RedirectWS) {
	hub.redirectWS = redirectWS
}

func (hub *PosHub) DettachListener() {
	hub.redirectWS = nil
}

func (hub *PosHub) GetConnectedCashregister(id central.UUID) (ConnectedCashregister, error) {
	c, ok := hub.getConnected(id)
	if !ok {
		return c, errors.New("not connected")
	}
	return c, nil
}
