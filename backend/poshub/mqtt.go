package poshub

import (
	"encoding/json"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"orbinea.example/core2/websocket"
	central "orbinea.example/fluxia/nexus"
)

func (hub *PosHub) InitializeMqtt() error {
	hub.mqttClient.Subscribe(central.MQChannelCashRegister, hub.handleCashregisterMqttEvent)

	return nil
}

func (hub *PosHub) handleCashregisterMqttEvent(client mqtt.Client, message mqtt.Message) {
	var cashregister central.Cashregister
	err := json.Unmarshal(message.Payload(), &cashregister)
	if err != nil {
		log.Warningf("failed to unmarshal cashregister from mqtt event")
		return
	}

	alreadyConnected := hub.IsOnline(cashregister.ID)
	if alreadyConnected {
		return
	}

	if cashregister.CurrentInstance != nil && cashregister.Status {
		connected, _ := hub.ConnectCashregister(cashregister)

		if connected {
			log.Infof("Getting the health of newly connected cashregister..")
			health, err := hub.GetCashregisterHealth(cashregister.ID)
			if err != nil {
				log.Warningf("failed to retrieve cashregister health")
				return
			}

			hub.redirect(CashregisterProxyWSFrame{
				GenericWSFrame: websocket.GenericWSFrame{
					Channel: central.GetComRessourceChannel(central.ComChannelNotify, central.ComResourceCashregisterHealth),
					Data:    health,
				},
			})
		} else {
			log.Warningf("failed to connect cashregister %s", cashregister.Name)
		}
	}
}
