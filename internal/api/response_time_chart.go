// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/httpx"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"

	"github.com/TwiN/logr"
	"github.com/labstack/echo/v5"
)

const (
	// maximumRecentChartResults is the maximum number of results of the recent period of the protected chart (fork), like
	// the heartbeats of the chart of the Uptime Kuma
	maximumRecentChartResults = 100

	// responseTimeChartInvalidPeriodBody is the body of the 400 of a period that is not recent, 3h, 6h, 24h or 1w
	responseTimeChartInvalidPeriodBody = `{"error":"invalid period"}`
)

// responseTimeChartPeriod is a period of the response time chart made of buckets
type responseTimeChartPeriod struct {
	bucketSeconds int
	buckets       int
}

// responseTimeChartPeriods are the periods of buckets of the chart, the recent period being the latest results
var responseTimeChartPeriods = map[string]responseTimeChartPeriod{
	"3h":  {bucketSeconds: common.MinuteBucketSeconds, buckets: 180},
	"6h":  {bucketSeconds: common.MinuteBucketSeconds, buckets: 360},
	"24h": {bucketSeconds: common.MinuteBucketSeconds, buckets: 1440},
	"1w":  {bucketSeconds: common.HourBucketSeconds, buckets: 168},
}

// errResponseTimeChartNotSupported is the error of a storage that cannot read the data of the chart, answered with 500
// by the dashboard and with 503 by the status pages.
var errResponseTimeChartNotSupported = errors.New("the storage does not support the response time chart")

// responseTimeChartPayload is the body of GET /api/v1/endpoints/:key/response-time-chart and of GET
// /api/v1/status-pages/:slug/endpoints/:key/response-time-chart. Period is the period requested (recent, 3h, 6h, 24h or
// 1w). IntervalSeconds is the interval of the endpoint (or of its heartbeat) in seconds, used to break the line over
// long gaps, and null when it is zero or unknown. BucketSeconds is the size of a bucket in seconds (60 for 3h, 6h and
// 24h, 3600 for 1w), absent for recent. From and To are the limits of the data in UTC (RFC 3339): for recent, the
// timestamps of the first and of the last result, or both the time of the request without results; otherwise the first
// bucket of the period and the time of the request. Results is only present for recent and Buckets for the other
// periods.
type responseTimeChartPayload struct {
	Period          string    `json:"period"`
	IntervalSeconds *int64    `json:"intervalSeconds"`
	BucketSeconds   int       `json:"bucketSeconds,omitempty"`
	From            time.Time `json:"from"`
	To              time.Time `json:"to"`
	// The list of the period is always present, even when empty, and the other one is absent
	Results *[]responseTimeChartResult `json:"results,omitempty"`
	Buckets *[]responseTimeChartBucket `json:"buckets,omitempty"`
}

// responseTimeChartResult is a result of the recent period of responseTimeChartPayload, from the oldest to the newest.
// Timestamp is in UTC (RFC 3339), Status is up, down or pending, and DurationMs is the response time in milliseconds.
type responseTimeChartResult struct {
	Timestamp  time.Time `json:"timestamp"`
	Status     string    `json:"status"`
	DurationMs int64     `json:"durationMs"`
}

// responseTimeChartBucket is a bucket of a period of responseTimeChartPayload; only the buckets the storage has are
// sent. Timestamp is the start of the bucket in UTC (RFC 3339). Up, Down and Pending count the results of the bucket by
// status. AvgMs, MinMs and MaxMs are the average, the minimum and the maximum response time, in milliseconds, of the up
// results of the bucket that lasted at least 1 ms, and null without any.
type responseTimeChartBucket struct {
	Timestamp time.Time `json:"timestamp"`
	Up        int       `json:"up"`
	Down      int       `json:"down"`
	Pending   int       `json:"pending"`
	AvgMs     *int64    `json:"avgMs"`
	MinMs     *int64    `json:"minMs"`
	MaxMs     *int64    `json:"maxMs"`
}

// isResponseTimeChartPeriod returns whether period is recent or a period of buckets
func isResponseTimeChartPeriod(period string) bool {
	_, isBucketPeriod := responseTimeChartPeriods[period]
	return period == "recent" || isBucketPeriod
}

// endpointIntervalSeconds returns the interval of the endpoint with the given key, used by the chart to break its line
// over long gaps: the interval of the endpoint of the configuration file, of the heartbeat of the external endpoint of
// the configuration file or of the managed endpoint, in this order, and nil when it is zero or unknown
func endpointIntervalSeconds(cfg *config.Config, key string) *int64 {
	var interval time.Duration
	if ep := cfg.GetEndpointByKey(key); ep != nil {
		interval = ep.Interval
	} else if externalEndpoint := cfg.GetExternalEndpointByKey(key); externalEndpoint != nil {
		interval = externalEndpoint.Heartbeat.Interval
	} else if state := managedendpoint.Get(key); state != nil {
		if state.Endpoint != nil {
			interval = state.Endpoint.Interval
		} else if state.Push != nil {
			interval = state.Push.Heartbeat.Interval
		}
	}
	seconds := int64(interval / time.Second)
	if seconds <= 0 {
		return nil
	}
	return &seconds
}

// buildResponseTimeChart encodes the payload of the chart of the endpoint for a valid period, with at most
// maximumResults recent results
func buildResponseTimeChart(cfg *config.Config, key, period string, maximumResults int, now time.Time) ([]byte, error) {
	reader, ok := store.GetResponseTimeChartReader()
	if !ok {
		return nil, errResponseTimeChartNotSupported
	}
	payload := responseTimeChartPayload{Period: period, IntervalSeconds: endpointIntervalSeconds(cfg, key), To: now.UTC()}
	if period == "recent" {
		results, err := reader.GetRecentResponseTimeResults(key, maximumResults)
		if err != nil {
			return nil, err
		}
		payload.From = payload.To
		encodedResults := make([]responseTimeChartResult, 0, len(results))
		for _, result := range results {
			status := "down"
			if result.Pending {
				status = "pending"
			} else if result.Success {
				status = "up"
			}
			encodedResults = append(encodedResults, responseTimeChartResult{Timestamp: result.Timestamp.UTC(), Status: status, DurationMs: result.Duration.Milliseconds()})
		}
		if len(encodedResults) > 0 {
			payload.From, payload.To = encodedResults[0].Timestamp, encodedResults[len(encodedResults)-1].Timestamp
		}
		payload.Results = &encodedResults
		return json.Marshal(payload)
	}
	chartPeriod := responseTimeChartPeriods[period]
	bucketDuration := time.Duration(chartPeriod.bucketSeconds) * time.Second
	lastBucket := now.UTC().Truncate(bucketDuration)
	payload.BucketSeconds = chartPeriod.bucketSeconds
	payload.From = lastBucket.Add(-time.Duration(chartPeriod.buckets-1) * bucketDuration)
	buckets, err := reader.GetResponseTimeBuckets(key, chartPeriod.bucketSeconds, payload.From, lastBucket)
	if err != nil {
		return nil, err
	}
	encodedBuckets := make([]responseTimeChartBucket, 0, len(buckets))
	for _, bucket := range buckets {
		encoded := responseTimeChartBucket{Timestamp: bucket.Timestamp.UTC(), Up: bucket.Up, Down: bucket.Down, Pending: bucket.Pending, MinMs: bucket.MinMs, MaxMs: bucket.MaxMs}
		if bucket.TimedUp > 0 {
			average := bucket.TotalMs / int64(bucket.TimedUp)
			encoded.AvgMs = &average
		}
		encodedBuckets = append(encodedBuckets, encoded)
	}
	payload.Buckets = &encodedBuckets
	return json.Marshal(payload)
}

// endpointResponseTimeChartHandler serves the response time chart of an endpoint of the dashboard. The endpoint must be
// known in memory, like for the event streams, so that an unknown key does not read the storage.
//
// It returns the handler of GET and HEAD /api/v1/endpoints/:key/response-time-chart.
//
// Authentication: protected group (security middleware, when security is configured).
// Request: the path parameter key is the key of the endpoint, unescaped once with url.QueryUnescape and not
// lower-cased; the query parameter period is required and is one of recent (the latest results, at most 100 and at
// most storage.maximum-number-of-results), 3h, 6h, 24h (buckets of one minute) or 1w (buckets of one hour). There is no
// default.
// Responses: 200 with responseTimeChartPayload; 400 with {"error": "invalid period"} when the period is missing or
// unknown; 401 without a valid authentication (and 429 with security.basic while the client is blocked); 404 with
// {"error": "endpoint not found"} when the key cannot be unescaped or is not a known endpoint (checked before the
// period); 500 with {"error": "failed to load the response time chart"} when the storage fails or does not support the
// chart. Every response has Cache-Control: no-store.
func endpointResponseTimeChartHandler(cfg *config.Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		httpx.SetHeader(c, echo.HeaderCacheControl, "no-store")
		key, err := url.QueryUnescape(c.Param("key"))
		if err != nil || (cfg.GetEndpointByKey(key) == nil && cfg.GetExternalEndpointByKey(key) == nil && managedendpoint.Get(key) == nil) {
			return httpx.JSON(c, http.StatusNotFound, map[string]any{"error": "endpoint not found"})
		}
		period := httpx.Query(c, "period")
		if !isResponseTimeChartPeriod(period) {
			httpx.SetHeader(c, echo.HeaderContentType, echo.MIMEApplicationJSON)
			return httpx.SendString(c, http.StatusBadRequest, responseTimeChartInvalidPeriodBody)
		}
		maximumResults := maximumRecentChartResults
		if cfg.Storage != nil && cfg.Storage.MaximumNumberOfResults > 0 && cfg.Storage.MaximumNumberOfResults < maximumResults {
			maximumResults = cfg.Storage.MaximumNumberOfResults
		}
		body, err := buildResponseTimeChart(cfg, key, period, maximumResults, time.Now())
		if err != nil {
			logr.Errorf("[api.endpointResponseTimeChartHandler] Failed to build the response time chart of endpoint with key=%s: %s", key, err.Error())
			return httpx.JSON(c, http.StatusInternalServerError, map[string]any{"error": "failed to load the response time chart"})
		}
		httpx.SetHeader(c, echo.HeaderContentType, echo.MIMEApplicationJSON)
		return httpx.Send(c, http.StatusOK, body)
	}
}

// statusPageResponseTimeChartHandler serves the response time chart of an endpoint of a published status page. The
// identical 404 is answered before the period is validated, and the payload is cached, see
// statuspage.PublicResponseTimeChart.
//
// It returns the handler of GET and HEAD /api/v1/status-pages/:slug/endpoints/:key/response-time-chart. Only registered
// when status-pages.enabled is true.
//
// Authentication: none, or HTTP Basic with the login of the page when the page requires one (statusPageAuth).
// Request: path parameters slug and key (the key is unescaped once with url.QueryUnescape and not lower-cased); the
// query parameter period is required and is one of recent, 3h, 6h, 24h or 1w, as in endpointResponseTimeChartHandler.
// Responses: 200 with responseTimeChartPayload, Cache-Control: no-cache (private, no-store and Vary: Authorization for
// a page with a login) and the headers of the public status pages; 400 with {"error": "invalid period"}; 401 with
// WWW-Authenticate: Basic when the page requires a login and the credential is missing or wrong; 404 with
// {"error": "status page not found"} when the page is not published or does not show the endpoint; 429 with Retry-After
// when the client exceeded the rate limit of the 404s or failed the login of the page too many times; 503 with
// {"error": "status page temporarily unavailable"} when the payload cannot be built. Errors have Cache-Control:
// no-store.
func statusPageResponseTimeChartHandler(cfg *config.Config, notFound echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		published, captured := publishedStatusPage(c)
		if !captured {
			return notFound(c)
		}
		key, err := url.QueryUnescape(c.Param("key"))
		if err != nil || !statuspage.IsEndpointShownOf(published, key) {
			return notFound(c)
		}
		period := httpx.Query(c, "period")
		if !isResponseTimeChartPeriod(period) {
			return sendStatusPageError(c, http.StatusBadRequest, responseTimeChartInvalidPeriodBody)
		}
		now := time.Now()
		body, err := statuspage.PublicResponseTimeChart(published, key, period, now, func(maximumResults int) ([]byte, error) {
			return buildResponseTimeChart(cfg, key, period, maximumResults, now)
		})
		switch {
		case err == nil:
			setPublicAPIHeaders(c)
			setProtectedPageCacheControl(c, published.Page.RequiresLogin(), "no-cache")
			httpx.SetHeader(c, echo.HeaderContentType, echo.MIMEApplicationJSON)
			return httpx.Send(c, http.StatusOK, body)
		case errors.Is(err, statuspage.ErrPageNotFound):
			return notFound(c)
		default:
			return sendStatusPageError(c, http.StatusServiceUnavailable, statusPageUnavailableBody)
		}
	}
}
