package api

import (
	"orbinea.example/core2/http"
	"orbinea.example/fluxia/beacon/backend/gather"
)

func (api *Handler) GetBrokerPersistenceSize(c *http.Context) {
	size := gather.GetPersistenceDatabaseSize()
	c.String(http.StatusOK, "%d", size)
}

func (api *Handler) GetBrokerPersistenceMetadata(c *http.Context) {
	meta, err := gather.GetRemoteMQTTPersistenceDatabaseMetadata()
	if err != nil {
		log.Errorf("failed to get metadata: %s", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, &meta)
}

func (api *Handler) PostBrokerPersistenceUpdate(c *http.Context) {
	err := gather.ExecInContainer("mosquitto", []string{"kill", "-USR1", "1"})
	if err != nil {
		log.Errorf("Failed to exec: %s", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, nil)
}
