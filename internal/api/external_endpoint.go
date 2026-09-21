package api

import (
	"errors"
	"strings"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/watchdog"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

// CreateExternalEndpointResult returns the handler of POST /api/v1/endpoints/:key/external: it records a result
// reported by an external system for an external endpoint of the configuration file, publishes its metrics and handles
// its alerts.
//
// Authentication: bearer token of the external endpoint (Authorization: Bearer <token>); the route is in the public
// group and checks the token itself.
// Request: the path parameter key is the key of the external endpoint, used as it was sent (neither unescaped nor
// lower-cased). Query parameters: success, required, true or false (when it is repeated, the last occurrence is
// validated and the first one is used); duration, optional, a Go duration such as 250ms or 1.5s; error, optional, the
// error of the result, only kept when success is false. The body is not used.
// Responses: 200 with an empty body; 400 when success is missing or invalid, or when the duration cannot be parsed; 401
// when the Authorization header is not a bearer token, the token is empty or is not the token of the endpoint; 404
// when no external endpoint has the key, in the configuration or in the storage; 500 when the result could not be
// stored, with the text of the error. Errors are text/plain. The parameters are checked in this order: success, the
// Authorization header, the key, the token, then the duration.
func CreateExternalEndpointResult(cfg *config.Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		// Check if the success query parameter is present
		// The LAST occurrence is the one that is validated and the FIRST is the one that counts, exactly as before:
		// ?success=true&success=invalid is refused
		success, exists := httpx.QueryLast(c, "success")
		if !exists || (success != "true" && success != "false") {
			return httpx.SendString(c, 400, "missing or invalid success query parameter")
		}
		// Check if the authorization bearer token header is correct
		authorizationHeader := httpx.Header(c, echo.HeaderAuthorization)
		if !strings.HasPrefix(authorizationHeader, "Bearer ") {
			return httpx.SendString(c, 401, "invalid Authorization header")
		}
		token := strings.TrimSpace(strings.TrimPrefix(authorizationHeader, "Bearer "))
		if len(token) == 0 {
			return httpx.SendString(c, 401, "bearer token must not be empty")
		}
		key := c.Param("key")
		externalEndpoint := cfg.GetExternalEndpointByKey(key)
		if externalEndpoint == nil {
			logr.Errorf("[api.CreateExternalEndpointResult] External endpoint with key=%s not found", key)
			return httpx.SendString(c, 404, "not found")
		}
		if externalEndpoint.Token != token {
			logr.Errorf("[api.CreateExternalEndpointResult] Invalid token for external endpoint with key=%s", key)
			return httpx.SendString(c, 401, "invalid token")
		}
		// Fork: a result of this API is reported by an external system, like a push: with the origin, a status page that
		// shows messages never turns the text sent in error= into a reason of its own
		result := &endpoint.Result{
			Timestamp: time.Now(),
			Success:   httpx.Query(c, "success") == "true",
			Errors:    []string{},
			Origin:    endpoint.ResultOriginPush,
		}
		if len(httpx.Query(c, "duration")) > 0 {
			parsedDuration, err := time.ParseDuration(httpx.Query(c, "duration"))
			if err != nil {
				logr.Errorf("[api.CreateExternalEndpointResult] Invalid duration from string=%s with error: %s", httpx.Query(c, "duration"), err.Error())
				return httpx.SendString(c, 400, "invalid duration: "+err.Error())
			}
			result.Duration = parsedDuration
		}
		if errorFromQuery := httpx.Query(c, "error"); !result.Success && len(errorFromQuery) > 0 {
			result.AddError(errorFromQuery)
		}
		// Fork: stores the result, publishes its metrics and handles its alerts one result at a time, like the pushes and
		// the heartbeat of the endpoint
		if err := watchdog.ProcessExternalEndpointResult(externalEndpoint, result, cfg, true); err != nil {
			if errors.Is(err, common.ErrEndpointNotFound) {
				return httpx.SendString(c, 404, err.Error())
			}
			logr.Errorf("[api.CreateExternalEndpointResult] Failed to insert result in storage: %s", err.Error())
			return httpx.SendString(c, 500, err.Error())
		}
		logr.Infof("[api.CreateExternalEndpointResult] Successfully inserted result for external endpoint with key=%s and success=%s", c.Param("key"), success)
		// Return the result
		return httpx.SendString(c, 200, "")
	}
}
