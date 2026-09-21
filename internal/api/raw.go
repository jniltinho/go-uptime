package api

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"

	"github.com/labstack/echo/v5"
)

// UptimeRaw handles GET and HEAD /api/v1/endpoints/:key/uptimes/:duration: the uptime of an endpoint over the duration,
// as a bare number.
//
// Authentication: none (public group).
// Request: the path parameter key is the key of the endpoint, unescaped once with url.QueryUnescape and not
// lower-cased; the path parameter duration is one of 1h, 24h, 7d or 30d (1h reads the last two hours, because the
// uptime is stored by hour).
// Responses: 200 with the uptime as text/plain, a ratio between 0 and 1 with six decimals (e.g. 0.998500),
// Cache-Control: no-cache, no-store, must-revalidate and Expires: 0; 400 when the duration is not supported, the key
// cannot be unescaped or the time range is invalid; 404 when no endpoint has the key; 500 on an error of the storage,
// with the text of the error. Errors are text/plain.
func UptimeRaw(c *echo.Context) error {
	duration := c.Param("duration")
	var from time.Time
	switch duration {
	case "30d":
		from = time.Now().Add(-30 * 24 * time.Hour)
	case "7d":
		from = time.Now().Add(-7 * 24 * time.Hour)
	case "24h":
		from = time.Now().Add(-24 * time.Hour)
	case "1h":
		from = time.Now().Add(-2 * time.Hour) // Because uptime metrics are stored by hour, we have to cheat a little
	default:
		return httpx.SendString(c, 400, "Durations supported: 30d, 7d, 24h, 1h")
	}
	key, err := url.QueryUnescape(c.Param("key"))
	if err != nil {
		return httpx.SendString(c, 400, "invalid key encoding")
	}
	uptime, err := store.Get().GetUptimeByKey(key, from, time.Now())
	if err != nil {
		if errors.Is(err, common.ErrEndpointNotFound) {
			return httpx.SendString(c, 404, err.Error())
		} else if errors.Is(err, common.ErrInvalidTimeRange) {
			return httpx.SendString(c, 400, err.Error())
		}
		return httpx.SendString(c, 500, err.Error())
	}

	httpx.SetHeader(c, "Content-Type", "text/plain")
	httpx.SetHeader(c, "Cache-Control", "no-cache, no-store, must-revalidate")
	httpx.SetHeader(c, "Expires", "0")
	return httpx.Send(c, 200, []byte(fmt.Sprintf("%f", uptime)))
}

// ResponseTimeRaw handles GET and HEAD /api/v1/endpoints/:key/response-times/:duration: the average response time of
// an endpoint over the duration, as a bare number.
//
// Authentication: none (public group).
// Request: the path parameter key is the key of the endpoint, unescaped once with url.QueryUnescape and not
// lower-cased; the path parameter duration is one of 1h, 24h, 7d or 30d (1h reads the last two hours).
// Responses: 200 with the average response time as text/plain, an integer number of milliseconds, Cache-Control:
// no-cache, no-store, must-revalidate and Expires: 0; 400 when the duration is not supported, the key cannot be
// unescaped or the time range is invalid; 404 when no endpoint has the key; 500 on an error of the storage, with the
// text of the error. Errors are text/plain.
func ResponseTimeRaw(c *echo.Context) error {
	duration := c.Param("duration")
	var from time.Time
	switch duration {
	case "30d":
		from = time.Now().Add(-30 * 24 * time.Hour)
	case "7d":
		from = time.Now().Add(-7 * 24 * time.Hour)
	case "24h":
		from = time.Now().Add(-24 * time.Hour)
	case "1h":
		from = time.Now().Add(-2 * time.Hour) // Because uptime metrics are stored by hour, we have to cheat a little
	default:
		return httpx.SendString(c, 400, "Durations supported: 30d, 7d, 24h, 1h")
	}
	key, err := url.QueryUnescape(c.Param("key"))
	if err != nil {
		return httpx.SendString(c, 400, "invalid key encoding")
	}
	responseTime, err := store.Get().GetAverageResponseTimeByKey(key, from, time.Now())
	if err != nil {
		if errors.Is(err, common.ErrEndpointNotFound) {
			return httpx.SendString(c, 404, err.Error())
		} else if errors.Is(err, common.ErrInvalidTimeRange) {
			return httpx.SendString(c, 400, err.Error())
		}
		return httpx.SendString(c, 500, err.Error())
	}

	httpx.SetHeader(c, "Content-Type", "text/plain")
	httpx.SetHeader(c, "Cache-Control", "no-cache, no-store, must-revalidate")
	httpx.SetHeader(c, "Expires", "0")
	return httpx.Send(c, 200, []byte(fmt.Sprintf("%d", responseTime)))
}
