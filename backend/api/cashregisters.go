package api

import "orbinea.example/core2/http"

func (api *Handler) GetCashregistersHealth(c *http.Context) {
	cashregistersHealth, err := api.poshub.GetCashregistersHealth()
	if err != nil {
		log.Errorf("Failed to get cashregisters health: %s", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, cashregistersHealth)
}
