package api

import (
	"orbinea.example/core2/http"
	"orbinea.example/fluxia/beacon/backend/gather"
)

func (api *Handler) GetDockerContainer(c *http.Context) {
	name := c.Param("name")
	container, err := gather.ContainerStatus(name)
	if err != nil {
		log.Errorf("Failed to get container %s status: %s", name, err)
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	c.JSON(http.StatusOK, &container)
}
