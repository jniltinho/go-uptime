// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package memory

import (
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// GetUptimesByKeys returns the uptimes over the last 24 hours, 7 days and 30 days before now of the endpoints with the
// given keys. Like GetUptimeByKey, the hour in which each period starts is fully counted.
func (s *Store) GetUptimesByKeys(keys []string, now time.Time) (map[string]*common.EndpointUptimes, error) {
	uptimes := make(map[string]*common.EndpointUptimes, len(keys))
	s.RLock()
	defer s.RUnlock()
	for _, key := range keys {
		endpointStatus, ok := s.endpointCache.GetValue(key).(*endpoint.Status)
		if !ok {
			continue
		}
		if endpointUptimes := uptimesOf(endpointStatus, now); endpointUptimes != nil {
			uptimes[key] = endpointUptimes
		}
	}
	return uptimes, nil
}

// GetEndpointSummaries returns the latest maximumResults results and the uptimes of the endpoints with the given keys
func (s *Store) GetEndpointSummaries(keys []string, maximumResults int, now time.Time) (map[string]*common.EndpointSummary, error) {
	summaries := make(map[string]*common.EndpointSummary, len(keys))
	s.RLock()
	defer s.RUnlock()
	for _, key := range keys {
		if _, done := summaries[key]; done {
			continue
		}
		endpointStatus, ok := s.endpointCache.GetValue(key).(*endpoint.Status)
		if !ok {
			continue
		}
		results := endpointStatus.Results
		if maximumResults <= 0 {
			results = nil
		} else if len(results) > maximumResults {
			results = results[len(results)-maximumResults:]
		}
		summary := &common.EndpointSummary{Results: make([]common.ResultSummary, 0, len(results))}
		for _, result := range results {
			summary.Results = append(summary.Results, common.ResultSummary{
				Timestamp:             result.Timestamp,
				Success:               result.Success,
				Duration:              result.Duration,
				CertificateExpiration: result.CertificateExpiration,
				Pending:               result.Pending,
				Connected:             result.Connected,
				Message:               result.Message,
				Origin:                result.Origin,
				HTTPStatus:            result.HTTPStatus,
				Errors:                result.Errors,
			})
		}
		if endpointUptimes := uptimesOf(endpointStatus, now); endpointUptimes != nil {
			summary.Uptimes = *endpointUptimes
		}
		summaries[key] = summary
	}
	return summaries, nil
}

// uptimesOf returns the uptimes of an endpoint status, or nil if it has no execution in the last 30 days. The store
// must be locked.
func uptimesOf(endpointStatus *endpoint.Status, now time.Time) *common.EndpointUptimes {
	if endpointStatus.Uptime == nil {
		return nil
	}
	periodStarts := [3]int64{
		now.Add(-24 * time.Hour).Truncate(time.Hour).Unix(),
		now.Add(-7 * 24 * time.Hour).Truncate(time.Hour).Unix(),
		now.Add(-30 * 24 * time.Hour).Truncate(time.Hour).Unix(),
	}
	end := now.Unix()
	var totalExecutions, successfulExecutions, totalResponseTimes [3]uint64
	for hourlyUnixTimestamp, hourlyStats := range endpointStatus.Uptime.HourlyStatistics {
		if hourlyStats == nil || hourlyUnixTimestamp > end {
			continue
		}
		for i, periodStart := range periodStarts {
			if hourlyUnixTimestamp >= periodStart {
				totalExecutions[i] += hourlyStats.TotalExecutions
				successfulExecutions[i] += hourlyStats.SuccessfulExecutions
				totalResponseTimes[i] += hourlyStats.TotalExecutionsResponseTime
			}
		}
	}
	if totalExecutions[2] == 0 {
		return nil
	}
	return &common.EndpointUptimes{
		Last24Hours:                uptimeRatio(successfulExecutions[0], totalExecutions[0]),
		Last7Days:                  uptimeRatio(successfulExecutions[1], totalExecutions[1]),
		Last30Days:                 uptimeRatio(successfulExecutions[2], totalExecutions[2]),
		AverageResponseTime24Hours: averageResponseTime(totalResponseTimes[0], totalExecutions[0]),
		AverageResponseTime7Days:   averageResponseTime(totalResponseTimes[1], totalExecutions[1]),
		AverageResponseTime30Days:  averageResponseTime(totalResponseTimes[2], totalExecutions[2]),
	}
}

// averageResponseTime returns the average response time in milliseconds, rounded down, or nil without execution
func averageResponseTime(totalResponseTime, totalExecutions uint64) *int {
	if totalExecutions == 0 {
		return nil
	}
	average := int(totalResponseTime / totalExecutions)
	return &average
}

func uptimeRatio(successfulExecutions, totalExecutions uint64) *float64 {
	if totalExecutions == 0 {
		return nil
	}
	ratio := float64(successfulExecutions) / float64(totalExecutions)
	return &ratio
}
