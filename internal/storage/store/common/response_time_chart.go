// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package common

import (
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

const (
	// MinuteBucketSeconds is the size of the response time buckets of the 3h, 6h and 24h periods of the chart (fork)
	MinuteBucketSeconds = 60

	// HourBucketSeconds is the size of the response time buckets of the 1w period of the chart (fork)
	HourBucketSeconds = 3600

	// MinuteBucketRetention is how long the minute buckets are kept at least
	MinuteBucketRetention = 24 * time.Hour

	// HourBucketRetention is how long the hour buckets are kept at least
	HourBucketRetention = 7 * 24 * time.Hour

	// ResponseTimeBucketCleanUpInterval is the minimum time between two clean ups of the buckets of an endpoint
	ResponseTimeBucketCleanUpInterval = time.Hour
)

// ResponseTimeBucket is the aggregate of the results of an endpoint during a minute or an hour, for the response time
// chart (fork). The minimum, the maximum and the total only include the up results with a duration of at least 1 ms.
type ResponseTimeBucket struct {
	Timestamp time.Time // Start of the bucket, in UTC
	Up        int       // Number of successful results that are not pending
	Down      int       // Number of failed results that are not pending
	Pending   int       // Number of pending results
	TimedUp   int       // Number of up results with a duration of at least 1 ms
	TotalMs   int64     // Total duration of the timed up results, in milliseconds
	MinMs     *int64    // Minimum duration of the timed up results, nil without one
	MaxMs     *int64    // Maximum duration of the timed up results, nil without one
}

// RecentResponseTimeResult is a result of an endpoint reduced to what the response time chart shows (fork)
type RecentResponseTimeResult struct {
	Timestamp time.Time
	Success   bool
	Pending   bool
	Duration  time.Duration
}

// ResponseTimeBucketTimestamp returns the unix timestamp of the bucket of bucketSeconds that contains t, in UTC
func ResponseTimeBucketTimestamp(t time.Time, bucketSeconds int) int64 {
	return t.UTC().Truncate(time.Duration(bucketSeconds) * time.Second).Unix()
}

// NewResponseTimeBucket returns the bucket of bucketSeconds with the single result given
func NewResponseTimeBucket(result *endpoint.Result, bucketSeconds int) ResponseTimeBucket {
	bucket := ResponseTimeBucket{Timestamp: time.Unix(ResponseTimeBucketTimestamp(result.Timestamp, bucketSeconds), 0).UTC()}
	switch {
	case result.Pending:
		bucket.Pending = 1
	case result.Success:
		bucket.Up = 1
		if milliseconds := result.Duration.Milliseconds(); milliseconds > 0 {
			bucket.TimedUp, bucket.TotalMs = 1, milliseconds
			bucket.MinMs, bucket.MaxMs = &milliseconds, &milliseconds
		}
	default:
		bucket.Down = 1
	}
	return bucket
}

// Add adds the counts, the total, the minimum and the maximum of other to the bucket
func (bucket *ResponseTimeBucket) Add(other ResponseTimeBucket) {
	bucket.Up += other.Up
	bucket.Down += other.Down
	bucket.Pending += other.Pending
	bucket.TimedUp += other.TimedUp
	bucket.TotalMs += other.TotalMs
	if other.MinMs != nil && (bucket.MinMs == nil || *other.MinMs < *bucket.MinMs) {
		minimum := *other.MinMs
		bucket.MinMs = &minimum
	}
	if other.MaxMs != nil && (bucket.MaxMs == nil || *other.MaxMs > *bucket.MaxMs) {
		maximum := *other.MaxMs
		bucket.MaxMs = &maximum
	}
}
