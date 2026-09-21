package api

import (
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/managedendpoint"
	"github.com/jniltinho/go-uptime/v7/internal/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"

	"github.com/TwiN/logr"
)

// endpointStatusResponse is the status of an endpoint with the fields of the fork used by the details page of the
// dashboard: whether the endpoint is a push endpoint, its uptimes and average response times, and the response time of
// its last result, whatever the page of results requested
//
// It is the body of GET /api/v1/endpoints/:key/statuses. The fields of endpoint.Status are inlined. Push is whether the
// endpoint receives its results by push (a managed push endpoint or an external endpoint of the configuration file).
// Uptime (a ratio from 0 to 1) and ResponseTime (an average in milliseconds) cover the last 24 hours, 7 days and 30
// days; a period is null without execution during it, and all of them are null when the storage cannot summarize
// endpoints. CurrentResponseTime is the response time of the latest result in milliseconds, and null when there is no
// result or when it is not positive.
type endpointStatusResponse struct {
	*endpoint.Status
	Push                bool                           `json:"push"`
	Uptime              statuspage.UptimePayload       `json:"uptime"`
	ResponseTime        statuspage.ResponseTimePayload `json:"responseTime"`
	CurrentResponseTime *int64                         `json:"currentResponseTime"`
}

// endpointSummaryReader is implemented by the storages that can summarize endpoints (latest results and uptimes), the
// same reader the public status pages use.
type endpointSummaryReader interface {
	GetEndpointSummaries(keys []string, maximumResults int, now time.Time) (map[string]*common.EndpointSummary, error)
}

// newEndpointStatusResponse wraps a status with the fields of the details page. The uptimes and the response times are
// left empty when the storage cannot summarize endpoints or when the summary fails, which is only logged.
func newEndpointStatusResponse(cfg *config.Config, key string, status *endpoint.Status) *endpointStatusResponse {
	response := &endpointStatusResponse{Status: status, Push: isPushEndpoint(cfg, key)}
	reader, ok := store.Get().(endpointSummaryReader)
	if !ok {
		return response
	}
	summaries, err := reader.GetEndpointSummaries([]string{key}, 1, time.Now())
	if err != nil {
		logr.Errorf("[api.EndpointStatus] Failed to retrieve the summary of endpoint with key=%s: %s", key, err.Error())
		return response
	}
	summary := summaries[key]
	if summary == nil {
		return response
	}
	response.Uptime, response.ResponseTime = statuspage.UptimePayloads(summary.Uptimes)
	if numberOfResults := len(summary.Results); numberOfResults > 0 {
		if milliseconds := summary.Results[numberOfResults-1].Duration.Milliseconds(); milliseconds > 0 {
			response.CurrentResponseTime = &milliseconds
		}
	}
	return response
}

// isPushEndpoint returns whether the key is the key of a push endpoint managed through the administration, in any state,
// or of an external endpoint of the configuration file, enabled or not
func isPushEndpoint(cfg *config.Config, key string) bool {
	if state := managedendpoint.Get(key); state != nil && state.Push != nil {
		return true
	}
	for _, externalEndpoint := range cfg.ExternalEndpoints {
		if externalEndpoint.Key() == key {
			return true
		}
	}
	return false
}
