package poshub

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	gohttp "net/http"
	"net/http/httputil"
	"net/url"

	"orbinea.example/core2/http"
	central "orbinea.example/fluxia/nexus"
)

const CashregisterAPIKey = "REMPLACER_PAR_UNE_VARIABLE_ENV"

func (hub *PosHub) GetCashregistersHealth() ([]central.CashregisterHealth, error) {
	healths := []central.CashregisterHealth{}

	cashregisters, err := hub.ds.GetCashregisters()
	if err != nil {
		return healths, errors.Wrap(err, "failed to get cashregisters health")
	}

	for _, c := range cashregisters {
		health := central.CashregisterHealth{
			CashregisterID: c.ID,
			Status:         "offline",
		}
		if hub.IsOnline(c.ID) {
			connectedCashregister, _ := hub.GetConnectedCashregister(c.ID)
			err := hub.CashregisterRequestJSONResponse(gohttp.MethodGet, c.ID, "/api/health", nil, &health)
			if err != nil {
				log.Warningf("failed to request connected cashregister %s: %s", c.ID, err)
			}
			health.CashregisterID = c.ID
			health.IP = connectedCashregister.IP
		}

		healths = append(healths, health)
	}

	return healths, nil
}

func (hub *PosHub) GetCashregisterHealth(id central.UUID) (central.CashregisterHealth, error) {
	var health central.CashregisterHealth
	err := hub.CashregisterRequestJSONResponse(gohttp.MethodGet, id, "/api/health", nil, &health)
	if err != nil {
		return health, errors.Wrap(err, "failed to request cashregister")
	}
	health.CashregisterID = id

	return health, nil
}

func (hub *PosHub) InitiateCashregisterUpdate(cashregisterID central.UUID) error {
	return hub.CashregisterRequestJSONResponse(gohttp.MethodPut, cashregisterID, "/api/update", nil, nil)
}

func (hub *PosHub) InitiateCashregisterSyncIn(cashregisterID central.UUID) error {
	return hub.CashregisterRequestJSONResponse(gohttp.MethodPut, cashregisterID, "/api/sync-in", nil, nil)
}

func (hub *PosHub) CashregisterRequestJSONResponse(method string, cashregisterID central.UUID, route string, data interface{}, responseData interface{}) error {
	response, err := hub.CashregisterRequestJSON(method, cashregisterID, route, data)
	if err != nil {
		// disconnect cashregister
		hub.deleteConnectedCashregister(cashregisterID)
		return errors.Wrapf(err, "failed to request cashregister %s", cashregisterID)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New(fmt.Sprintf("cashregister return bad status code: %d", response.StatusCode))
	}

	// decode response body
	if responseData != nil {
		err = json.NewDecoder(response.Body).Decode(responseData)
		if err != nil {
			return errors.Wrap(err, "failed to decode response body")
		}
	}

	return err
}

func (hub *PosHub) CashregisterRequest(method string, cashregisterID central.UUID, route string, body []byte) (response *gohttp.Response, err error) {
	connection, connected := hub.getConnected(cashregisterID)
	if !connected {
		return response, errors.New(fmt.Sprintf("cashregister %s is not connected", cashregisterID))
	}

	// add loalhost for dev
	client := gohttp.Client{}

	url := fmt.Sprintf("http://%s%s", connection.Address, route)
	request, err := gohttp.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return response, errors.Wrap(err, "failed to create http request")
	}
	request.Header.Set("Authorization", CashregisterAPIKey)
	request.Header.Set("X-WS-Token", connection.wsToken)

	return client.Do(request)
}

func (hub *PosHub) CashregisterProxy(c *http.Context, cashregisterID central.UUID, route string) (err error) {
	connection, connected := hub.getConnected(cashregisterID)
	if !connected {
		return errors.New(fmt.Sprintf("cashregister %s is not connected", cashregisterID))
	}

	URL, err := url.Parse(fmt.Sprintf("http://%s%s", connection.Address, route))
	if err != nil {
		return errors.Wrap(err, "failed to parse remote url")
	}

	headers := c.Request.Header
	headers.Set("Authorization", CashregisterAPIKey)
	headers.Set("X-WS-Token", connection.wsToken)

	proxy := httputil.NewSingleHostReverseProxy(URL)
	proxy.Director = func(req *gohttp.Request) {
		req.Method = c.Request.Method
		req.Header = headers
		req.Body = c.Request.Body
		req.Host = URL.Host
		req.URL = URL
	}

	proxy.ServeHTTP(c.Writer, c.Request)

	return
}

func (hub *PosHub) CashregisterRequestJSON(method string, cashregisterID central.UUID, route string, data interface{}) (response *gohttp.Response, err error) {
	var marshalledData []byte
	if data != nil {
		marshalledData, err = json.Marshal(data)
		if err != nil {
			return response, errors.Wrap(err, "failed to marshall data")
		}
	}

	return hub.CashregisterRequest(method, cashregisterID, route, marshalledData)
}
