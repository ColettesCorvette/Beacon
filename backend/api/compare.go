package api

import (
	"fmt"

	"github.com/pkg/errors"
	"orbinea.example/core2/http"
	"orbinea.example/core2/websocket"
	"orbinea.example/fluxia/beacon/backend/poshub"
	central "orbinea.example/fluxia/nexus"
	"orbinea.example/fluxia/nexus/datastore"
	"orbinea.example/fluxia/nexus/helpers"
)

func (api *Handler) Compare(c *http.Context) {
	var err error

	var comparison central.DataComparison

	err = c.ParseJSON(&comparison)
	if err != nil {
		log.Errorf("failed to unmarshal request body: %s", err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	ds1, err := api.GetComparableDatastore(comparison.Source1)
	if err != nil {
		log.Errorf("failed to get comparable %s (%s) datastore: %s", comparison.Source1.Type, comparison.Source1.ID, err)
		c.JSON(http.StatusInternalServerError, central.APIError{
			Code:    "comparison_failed",
			Message: fmt.Sprintf("can't create datastore for %s, id %s: %s", comparison.Source1.Type, comparison.Source1.ID, err),
		})
		return
	}

	ds2, err := api.GetComparableDatastore(comparison.Source2)
	if err != nil {
		log.Errorf("failed to get comparable %s (%s) datastore: %s", comparison.Source2.Type, comparison.Source2.ID, err)
		c.JSON(http.StatusInternalServerError, central.APIError{
			Code:    "comparison_failed",
			Message: fmt.Sprintf("can't create datastore for %s, id %s: %s", comparison.Source2.Type, comparison.Source2.ID, err),
		})
		return
	}

	if len(comparison.Collections) == 0 {
		comparison.Collections = helpers.CashregisterComparableCollections
	}

	result, err := helpers.Compare(comparison.Collections, ds1, ds2, comparison.Since, func(gp central.GenericProgress) {
		api.sendWS(websocket.GenericWSFrame{
			Channel: "comparison-progress",
			Data:    gp,
		})
	})
	if err != nil {
		log.Errorf("failed to compare: %s", err)
		c.JSON(http.StatusInternalServerError, central.APIError{
			Code:    "comparison_failed",
			Message: fmt.Sprintf("failed to compare: %s", err.Error()),
		})

		return
	}

	c.JSON(http.StatusOK, result)
}

func (api *Handler) GetComparableDatastore(comparable central.DataComparable) (ds datastore.ComparableDatastore, err error) {
	if comparable.Type == central.DataComparableTypeCashregister {
		crID := central.UUID(comparable.ID)
		if !api.poshub.IsOnline(crID) {
			return ds, errors.New("cashregister is offline")
		}
		connectedCR, err := api.poshub.GetConnectedCashregister(crID)
		if err != nil {
			return ds, errors.Wrapf(err, "failed to get connected cashregister")
		}

		ds = datastore.NewCashregisterDatastore(fmt.Sprintf("http://%s", connectedCR.Address), poshub.CashregisterAPIKey)
	} else if comparable.Type == central.DataComparableTypeLedger {
		if comparable.ID != "local" {
			return ds, fmt.Errorf("comparable not supported: %s (%s)", comparable.Type, comparable.ID)
		}

		ds = api.ds
	} else {
		return ds, fmt.Errorf("unknown comparable type: %s", comparable.Type)
	}

	return
}
