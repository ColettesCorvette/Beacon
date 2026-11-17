package server

import (
	"encoding/json"
	"fmt"

	"orbinea.example/core2/http"
	"orbinea.example/core2/websocket"
	"orbinea.example/fluxia/beacon/backend/poshub"
	central "orbinea.example/fluxia/nexus"
)

func (s *Server) SetupCashregistersProxy() {
	s.httpServer.Any("/cashregisters/:id/*route", s.redirectCashregisterRequest)

	wsHub := websocket.NewHub(&websocket.Callbacks{
		OnClientRegister:   s.HandleWSConnected,
		OnClientUnregister: s.HandleWSDisconnected,
		OnMessage:          s.HandleWSMessage,
	})
	go wsHub.Run()
	ws := s.httpServer.Group("/ws")
	ws.Use(s.httpServer.HandlerFunc(websocket.BuildWebsocketMiddleware(wsHub)))
	ws.GET("")
}

func (s *Server) redirectCashregisterRequest(c *http.Context) {
	log.Infof("redirecting request to cashregister")

	cashregisterID := central.UUID(c.Param("id"))
	if !s.poshub.IsOnline(cashregisterID) {
		log.Errorf("cashregister %s is not online", cashregisterID)
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}

	err := s.poshub.CashregisterProxy(c, cashregisterID, fmt.Sprintf("/api%s", c.Param("route")))
	if err != nil {
		log.Errorf("failed to proxy cashregister request: %s", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func (s *Server) HandleWSConnected(client *websocket.Client) {
	s.poshub.AttachRedirect(func(frame poshub.CashregisterProxyWSFrame) {
		encoded, err := json.Marshal(frame)
		if err != nil {
			log.Errorf("failed to encode cashregister proxy frame")
		}

		client.Send <- encoded
	})
	s.handler.SetWSClient(client)
}

func (s *Server) HandleWSDisconnected(client *websocket.Client) {
	s.poshub.DettachListener()
}

func (s *Server) HandleWSMessage(message websocket.Message) {
	log.Info("ws client message")
}
