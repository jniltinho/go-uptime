// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package memory

import (
	"slices"
	"sort"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// responseTimeBucketKey identifies a bucket of an endpoint by its size and its start
type responseTimeBucketKey struct {
	seconds   int
	timestamp int64
}

// addResponseTimeBuckets adds a result to its minute and hour buckets and, at most once per
// ResponseTimeBucketCleanUpInterval, removes the buckets of the endpoint older than their retention. Called with the
// lock of the store.
func (s *Store) addResponseTimeBuckets(key string, result *endpoint.Result) {
	if s.responseTimeBuckets == nil {
		s.responseTimeBuckets = make(map[string]map[responseTimeBucketKey]*common.ResponseTimeBucket)
		s.responseTimeBucketsCleanUps = make(map[string]time.Time)
	}
	buckets := s.responseTimeBuckets[key]
	if buckets == nil {
		buckets = make(map[responseTimeBucketKey]*common.ResponseTimeBucket)
		s.responseTimeBuckets[key] = buckets
	}
	for _, bucketSeconds := range []int{common.MinuteBucketSeconds, common.HourBucketSeconds} {
		added := common.NewResponseTimeBucket(result, bucketSeconds)
		bucketKey := responseTimeBucketKey{seconds: bucketSeconds, timestamp: added.Timestamp.Unix()}
		if bucket := buckets[bucketKey]; bucket != nil {
			bucket.Add(added)
		} else {
			buckets[bucketKey] = &added
		}
	}
	now := s.currentTime()
	if lastCleanUp, exists := s.responseTimeBucketsCleanUps[key]; exists && now.Sub(lastCleanUp) < common.ResponseTimeBucketCleanUpInterval && !now.Before(lastCleanUp) {
		return
	}
	minuteThreshold := now.Add(-(common.MinuteBucketRetention + time.Hour)).Unix()
	hourThreshold := now.Add(-(common.HourBucketRetention + time.Hour)).Unix()
	for bucketKey := range buckets {
		if (bucketKey.seconds == common.MinuteBucketSeconds && bucketKey.timestamp < minuteThreshold) || (bucketKey.seconds == common.HourBucketSeconds && bucketKey.timestamp < hourThreshold) {
			delete(buckets, bucketKey)
		}
	}
	s.responseTimeBucketsCleanUps[key] = now
}

// forgetResponseTimeBuckets removes the buckets of the endpoints whose key is not in keys. Called with the lock of the
// store.
func (s *Store) forgetResponseTimeBuckets(keys []string) {
	for key := range s.responseTimeBuckets {
		if !slices.Contains(keys, key) {
			delete(s.responseTimeBuckets, key)
			delete(s.responseTimeBucketsCleanUps, key)
		}
	}
}

// currentTime returns the time used by the response time buckets, which the tests can replace
func (s *Store) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// GetResponseTimeBuckets returns the buckets of bucketSeconds of the endpoint with the given key whose start is between
// from and to, from the oldest to the most recent
func (s *Store) GetResponseTimeBuckets(key string, bucketSeconds int, from, to time.Time) ([]common.ResponseTimeBucket, error) {
	s.RLock()
	defer s.RUnlock()
	buckets := make([]common.ResponseTimeBucket, 0)
	for bucketKey, bucket := range s.responseTimeBuckets[key] {
		if bucketKey.seconds != bucketSeconds || bucketKey.timestamp < from.Unix() || bucketKey.timestamp > to.Unix() {
			continue
		}
		var copied common.ResponseTimeBucket
		copied.Add(*bucket)
		copied.Timestamp = bucket.Timestamp
		buckets = append(buckets, copied)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})
	return buckets, nil
}

// GetRecentResponseTimeResults returns the latest limit results of the endpoint with the given key, from the oldest to
// the most recent timestamp
func (s *Store) GetRecentResponseTimeResults(key string, limit int) ([]common.RecentResponseTimeResult, error) {
	s.RLock()
	defer s.RUnlock()
	results := make([]common.RecentResponseTimeResult, 0)
	status, ok := s.endpointCache.GetValue(key).(*endpoint.Status)
	if !ok || limit <= 0 {
		return results, nil
	}
	latest := status.Results
	if len(latest) > limit {
		latest = latest[len(latest)-limit:]
	}
	for _, result := range latest {
		results = append(results, common.RecentResponseTimeResult{Timestamp: result.Timestamp, Success: result.Success, Pending: result.Pending, Duration: result.Duration})
	}
	slices.SortStableFunc(results, func(a, b common.RecentResponseTimeResult) int {
		return a.Timestamp.Compare(b.Timestamp)
	})
	return results, nil
}
