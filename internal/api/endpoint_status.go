// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/jniltinho/go-uptime/v7/internal/client"
	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/config/remote"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

// EndpointStatuses handles requests to retrieve all EndpointStatus
// Due to how intensive this operation can be on the storage, this function leverages a cache.
//
// It returns the handler of GET and HEAD /api/v1/endpoints/statuses: the status of every endpoint with a page of its
// results, followed by the statuses of the remote instances (remote.instances), when configured. Each page is cached
// for 10 seconds.
//
// Authentication: protected group (security middleware, when security is configured).
// Request: query parameters page (default 1; an invalid value or a value below 1 is 1) and pageSize (default 50; an
// invalid value or a value below 1 is 50; on page 1 it is capped at storage.maximum-number-of-results). They page the
// results of each endpoint, not the endpoints.
// Responses: 200 with a JSON array of endpoint.Status, without events; 401 without a valid authentication (and 429 with
// security.basic while the client is blocked), see createRouter; 500 as text/plain when the storage fails (with the
// text of the error) or the statuses cannot be encoded. A failure of the remote instances is only logged.
func EndpointStatuses(cfg *config.Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		page, pageSize := extractPageAndPageSizeFromRequest(c, cfg.Storage.MaximumNumberOfResults)
		value, exists := cache.Get(fmt.Sprintf("endpoint-status-%d-%d", page, pageSize))
		var data []byte
		if !exists {
			endpointStatuses, err := store.Get().GetAllEndpointStatuses(paging.NewEndpointStatusParams().WithResults(page, pageSize))
			if err != nil {
				logr.Errorf("[api.EndpointStatuses] Failed to retrieve endpoint statuses: %s", err.Error())
				return httpx.SendString(c, 500, err.Error())
			}
			// ALPHA: Retrieve endpoint statuses from remote instances
			if endpointStatusesFromRemote, err := getEndpointStatusesFromRemoteInstances(cfg.Remote); err != nil {
				logr.Errorf("[handler.EndpointStatuses] Silently failed to retrieve endpoint statuses from remote: %s", err.Error())
			} else if endpointStatusesFromRemote != nil {
				endpointStatuses = append(endpointStatuses, endpointStatusesFromRemote...)
			}
			// Marshal endpoint statuses to JSON
			data, err = json.Marshal(endpointStatuses)
			if err != nil {
				logr.Errorf("[api.EndpointStatuses] Unable to marshal object to JSON: %s", err.Error())
				return httpx.SendString(c, 500, "unable to marshal object to JSON")
			}
			cache.SetWithTTL(fmt.Sprintf("endpoint-status-%d-%d", page, pageSize), data, cacheTTL)
		} else {
			data = value.([]byte)
		}
		httpx.SetHeader(c, "Content-Type", "application/json")
		return httpx.Send(c, 200, data)
	}
}

// getEndpointStatusesFromRemoteInstances fetches the endpoint statuses of every remote instance and prefixes their
// names with the prefix of the instance. An instance that fails is logged and skipped; it returns nil without remote
// instances, and an error only when none of them returned a status.
func getEndpointStatusesFromRemoteInstances(remoteConfig *remote.Config) ([]*endpoint.Status, error) {
	if remoteConfig == nil || len(remoteConfig.Instances) == 0 {
		return nil, nil
	}
	var endpointStatusesFromAllRemotes []*endpoint.Status
	httpClient := client.GetHTTPClient(remoteConfig.ClientConfig)
	for _, instance := range remoteConfig.Instances {
		response, err := httpClient.Get(instance.URL)
		if err != nil {
			// Log the error but continue with other instances
			logr.Errorf("[api.getEndpointStatusesFromRemoteInstances] Failed to retrieve endpoint statuses from %s: %s", instance.URL, err.Error())
			continue
		}
		var endpointStatuses []*endpoint.Status
		if err = json.NewDecoder(response.Body).Decode(&endpointStatuses); err != nil {
			_ = response.Body.Close()
			logr.Errorf("[api.getEndpointStatusesFromRemoteInstances] Failed to decode endpoint statuses from %s: %s", instance.URL, err.Error())
			continue
		}
		_ = response.Body.Close()
		for _, endpointStatus := range endpointStatuses {
			endpointStatus.Name = instance.EndpointPrefix + endpointStatus.Name
		}
		endpointStatusesFromAllRemotes = append(endpointStatusesFromAllRemotes, endpointStatuses...)
	}
	// Only return nil, error if no remote instances were successfully processed
	if len(endpointStatusesFromAllRemotes) == 0 && remoteConfig.Instances != nil {
		return nil, fmt.Errorf("failed to retrieve endpoint statuses from all remote instances")
	}
	return endpointStatusesFromAllRemotes, nil
}

// EndpointStatus retrieves a single endpoint.Status by group and endpoint name
//
// It returns the handler of GET and HEAD /api/v1/endpoints/:key/statuses: the status of one endpoint with a page of its
// results, its events, and the fields of the details page of the dashboard.
//
// Authentication: protected group (security middleware, when security is configured).
// Request: the path parameter key is the key of the endpoint, unescaped once with url.QueryUnescape and not
// lower-cased; query parameters page and pageSize as in EndpointStatuses. The events are always the first page, of at
// most storage.maximum-number-of-events events.
// Responses: 200 with endpointStatusResponse (endpoint.Status plus push, uptime, responseTime and
// currentResponseTime); 400 when the key cannot be unescaped; 401 without a valid authentication (and 429 with
// security.basic while the client is blocked); 404 when no endpoint has the key; 500 when the storage fails (with the
// text of the error) or the status cannot be encoded. Errors are text/plain.
func EndpointStatus(cfg *config.Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		page, pageSize := extractPageAndPageSizeFromRequest(c, cfg.Storage.MaximumNumberOfResults)
		key, err := url.QueryUnescape(c.Param("key"))
		if err != nil {
			logr.Errorf("[api.EndpointStatus] Failed to decode key: %s", err.Error())
			return httpx.SendString(c, 400, "invalid key encoding")
		}
		endpointStatus, err := store.Get().GetEndpointStatusByKey(key, paging.NewEndpointStatusParams().WithResults(page, pageSize).WithEvents(1, cfg.Storage.MaximumNumberOfEvents))
		if err != nil {
			if errors.Is(err, common.ErrEndpointNotFound) {
				return httpx.SendString(c, 404, err.Error())
			}
			logr.Errorf("[api.EndpointStatus] Failed to retrieve endpoint status: %s", err.Error())
			return httpx.SendString(c, 500, err.Error())
		}
		if endpointStatus == nil { // XXX: is this check necessary?
			logr.Errorf("[api.EndpointStatus] Endpoint with key=%s not found", key)
			return httpx.SendString(c, 404, "not found")
		}
		output, err := json.Marshal(newEndpointStatusResponse(cfg, key, endpointStatus))
		if err != nil {
			logr.Errorf("[api.EndpointStatus] Unable to marshal object to JSON: %s", err.Error())
			return httpx.SendString(c, 500, "unable to marshal object to JSON")
		}
		httpx.SetHeader(c, "Content-Type", "application/json")
		return httpx.Send(c, 200, output)
	}
}
