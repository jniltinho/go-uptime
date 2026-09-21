// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"fmt"
	"net/http"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/suite"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common/paging"

	"github.com/labstack/echo/v5"
)

// SuiteStatuses handles requests to retrieve all suite statuses
//
// It returns the handler of GET and HEAD /api/v1/suites/statuses: the status of every suite with a page of its results.
// While the storage has no suite status yet, an empty status is returned for every enabled suite of the configuration.
//
// Authentication: protected group (security middleware, when security is configured).
// Request: query parameters page (default 1) and pageSize (default 50, capped at 100 on page 1), see
// extractPageAndPageSizeFromRequest. They page the results of each suite, not the suites.
// Responses: 200 with a JSON array of suite.Status; 401 without a valid authentication (and 429 with security.basic
// while the client is blocked); 500 with {"error": "..."} when the storage fails.
func SuiteStatuses(cfg *config.Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		page, pageSize := extractPageAndPageSizeFromRequest(c, 100)
		params := paging.NewSuiteStatusParams().WithPagination(page, pageSize)
		suiteStatuses, err := store.Get().GetAllSuiteStatuses(params)
		if err != nil {
			return httpx.JSON(c, http.StatusInternalServerError, map[string]any{
				"error": fmt.Sprintf("Failed to retrieve suite statuses: %v", err),
			})
		}
		// If no statuses exist yet, create empty ones from config
		if len(suiteStatuses) == 0 {
			for _, s := range cfg.Suites {
				if s.IsEnabled() {
					suiteStatuses = append(suiteStatuses, suite.NewStatus(s))
				}
			}
		}
		return httpx.JSON(c, http.StatusOK, suiteStatuses)
	}
}

// SuiteStatus handles requests to retrieve a single suite's status
//
// It returns the handler of GET and HEAD /api/v1/suites/:key/statuses: the status of one suite with a page of its
// results. A suite of the configuration without status in the storage, or whose status cannot be read, gets an empty
// status.
//
// Authentication: protected group (security middleware, when security is configured).
// Request: the path parameter key is the key of the suite, used as it was sent (neither unescaped nor lower-cased);
// query parameters page (default 1) and pageSize (default 50, capped at 100 on page 1).
// Responses: 200 with suite.Status; 401 without a valid authentication (and 429 with security.basic while the client is
// blocked); 404 with {"error": "..."} when the suite is neither in the storage nor in the configuration. An error of
// the storage is never a 500: it falls back on the configuration, then on the 404.
func SuiteStatus(cfg *config.Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		page, pageSize := extractPageAndPageSizeFromRequest(c, 100)
		key := c.Param("key")
		params := paging.NewSuiteStatusParams().WithPagination(page, pageSize)
		status, err := store.Get().GetSuiteStatusByKey(key, params)
		if err != nil || status == nil {
			// Try to find the suite in config
			for _, s := range cfg.Suites {
				if s.Key() == key {
					status = suite.NewStatus(s)
					break
				}
			}
			if status == nil {
				return httpx.JSON(c, 404, map[string]any{
					"error": fmt.Sprintf("Suite with key '%s' not found", key),
				})
			}
		}
		return httpx.JSON(c, http.StatusOK, status)
	}
}
