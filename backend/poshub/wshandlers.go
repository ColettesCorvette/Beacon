package poshub

import (
	"github.com/pkg/errors"
	"orbinea.example/core2/websocket"
	central "orbinea.example/fluxia/nexus"
)

type crWSHandler func(*ConnectedCashregister, interface{}) error

func (hub *PosHub) initWSHandlers() {
	hub.wsHandlers = map[string]crWSHandler{
		"token": hub.setWSToken,
	}
}

func (hub *PosHub) handleConnected(client *websocket.Client) {
	cashregister, err := hub.getCashregisterFromWSClient(client)
	if err != nil {
		log.Warningf("unknown cashregister connected")
	} else {
		log.Infof("Cashregister %s: websocket connected", cashregister.ID)
	}
}

func (hub *PosHub) handleDisconnected(client *websocket.Client) {
	cashregister, err := hub.getCashregisterFromWSClient(client)
	if err != nil {
		log.Warningf("unknown cashregister disconnected")
	} else {
		log.Infof("Cashregister %s: websocket disconnected", cashregister.ID)
		hub.redirect(CashregisterProxyWSFrame{
			GenericWSFrame: websocket.GenericWSFrame{
				Channel: central.GetComRessourceChannel(central.ComChannelNotify, central.ComResourceCashregisterHealth),
				Data: central.CashregisterHealth{
					CashregisterID: cashregister.ID,
					Status:         "offline",
				},
			},
		})
		hub.deleteConnectedCashregister(cashregister.ID)
	}
}

func (hub *PosHub) handleMessage(message websocket.Message) {
	// get cashregister
	cashregister, err := hub.getCashregisterFromWSClient(message.Client)
	if err != nil {
		log.Errorf("failed to get cashregister from websocket client: %s", err)
		return
	}

	// Parse message
	frame, err := message.GetGenericFrame()
	if err != nil {
		log.Errorf("failed to unmarshal WSFrame: %s", err)
		return
	}

	if frame.Channel == "" {
		log.Warningf("empty channel")
		return
	}

	// log.Infof("Received message on channel '%s' from cashregister %s with data %+v", frame.Channel, cashregister.ID, frame.Data)

	handler, ok := hub.wsHandlers[frame.Channel]
	if ok {
		log.Warningf("locally handling websocket message for channel %s", frame.Channel)
		err = handler(cashregister, frame.Data)
	} else {
		log.Infof("emit websocket message from cashregister %s on channel %s", cashregister.ID, frame.Channel)
		proxyFrame := CashregisterProxyWSFrame{
			CashregisterID: cashregister.ID,
			GenericWSFrame: frame,
		}

		hub.redirect(proxyFrame)
	}

	if err != nil {
		log.Errorf("failed to handle message: %s", err)
		return
	}
}

func (hub *PosHub) getCashregisterFromWSClient(client *websocket.Client) (*ConnectedCashregister, error) {
	hub.connectedCashregistersLock.Lock()
	defer hub.connectedCashregistersLock.Unlock()
	for _, c := range hub.connectedCashregisters {
		if c.wsClient == client {
			return c, nil
		}
	}

	return nil, errors.New("unknown websocket client")
}

func (hub *PosHub) setWSToken(c *ConnectedCashregister, data interface{}) error {
	token, ok := data.(string)
	if !ok {
		// frame malformée d'une caisse : erreur loggée, pas un panic process
		return errors.New("token frame: payload non string")
	}
	hub.connectedCashregistersLock.Lock()
	c.wsToken = token
	hub.connectedCashregistersLock.Unlock()
	return nil
}
