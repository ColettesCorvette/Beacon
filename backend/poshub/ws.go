package poshub

import (
	"fmt"
	"io"
	"net/http"
	gohttp "net/http"

	"github.com/pkg/errors"
	"orbinea.example/core2/websocket"
	central "orbinea.example/fluxia/nexus"
)

type CashregisterProxyWSFrame struct {
	websocket.GenericWSFrame
	CashregisterID central.UUID
}

// Initialize try to establish connection to all cashregisters
func (hub *PosHub) InitializeWS() error {
	cashregisters, err := hub.ds.GetCashregisters()
	if err != nil {
		return errors.Wrap(err, "failed to get cashregisters")
	}

	hub.wsHub = websocket.NewHub(&websocket.Callbacks{
		OnClientRegister:   hub.handleConnected,
		OnClientUnregister: hub.handleDisconnected,
		OnMessage:          hub.handleMessage,
	}, websocket.WithMessageMaxSize(1000000))

	go hub.wsHub.Run()

	for _, c := range cashregisters {
		if !c.Status || c.CurrentInstance == nil || c.CurrentInstance.Port == 0 || len(c.IPAddresses) == 0 {
			continue
		}

		go hub.ConnectCashregister(c)
	}

	return nil
}

func (hub *PosHub) ConnectCashregister(c central.Cashregister) (bool, ConnectedCashregister) {
	connected := false

	cashregisterConnection := ConnectedCashregister{
		ID: c.ID,
	}

	if c.CurrentInstance == nil {
		return connected, cashregisterConnection
	}

	// add localhost for dev
	c.IPAddresses = append([]string{"localhost"}, c.IPAddresses...)

	// try to connect to all ip
	for _, IP := range c.IPAddresses {
		address := fmt.Sprintf("%s:%d", IP, c.CurrentInstance.Port)

		if !pingCashregister(address, c.ID) {
			continue
		}

		url := fmt.Sprintf("ws://%s%s", address, "/api/ws")
		header := http.Header{}
		header.Set("Authorization", CashregisterAPIKey)
		wsClient, err := websocket.NewClient(url, hub.wsHub, header)
		if err != nil {
			continue
		}

		cashregisterConnection.IP = IP
		cashregisterConnection.Address = address
		cashregisterConnection.wsClient = wsClient
		hub.setConnectedCashregister(c.ID, cashregisterConnection)
		connected = true
		break
	}

	if connected {
		log.Infof("Cashregister %s connected at %s", c.ID, cashregisterConnection.Address)
	} else {
		log.Infof("Cashregister %s not connected", c.ID)
	}

	return connected, cashregisterConnection
}

// pingCashregister vérifie que address répond bien au ping ET s'identifie
// avec l'ID attendu. Avant ce correctif, un ping en échec n'empêchait pas
// la connexion WS (validation contournée).
func pingCashregister(address string, expectedID central.UUID) bool {
	request, err := gohttp.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/api/ping", address), nil)
	if err != nil {
		return false
	}
	request.Header.Set("Authorization", CashregisterAPIKey)

	response, err := gohttp.DefaultClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != gohttp.StatusOK {
		return false
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Debugf("ping caisse %s: lecture réponse impossible: %s", address, err)
		return false
	}
	if central.UUID(body) != expectedID {
		log.Debugf("caisse à %s: ID %q inattendu (attendu %s)", address, string(body), expectedID)
		return false
	}
	return true
}

func (hub *PosHub) setConnectedCashregister(id central.UUID, cashregister ConnectedCashregister) {
	hub.connectedCashregistersLock.Lock()
	hub.connectedCashregisters[id] = &cashregister
	hub.connectedCashregistersLock.Unlock()
}

func (hub *PosHub) deleteConnectedCashregister(id central.UUID) {
	hub.connectedCashregistersLock.Lock()
	delete(hub.connectedCashregisters, id)
	hub.connectedCashregistersLock.Unlock()
}

func (hub *PosHub) SendWSMessageToCashregister(id central.UUID, frame websocket.GenericWSFrame) error {
	connectedCashregister, ok := hub.getConnected(id)
	if !ok {
		return errors.New("cashregister not connected")
	}

	return connectedCashregister.wsClient.SendGeneric(frame)
}

func (hub *PosHub) redirect(frame CashregisterProxyWSFrame) {
	if hub.redirectWS != nil {
		hub.redirectWS(frame)
	}
}
