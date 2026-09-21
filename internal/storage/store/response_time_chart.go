package store

import (
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/memory"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/sql"
)

var (
	_ ResponseTimeChartReader = (*memory.Store)(nil)
	_ ResponseTimeChartReader = (*sql.Store)(nil)
)

// ResponseTimeChartReader reads the data of the response time chart of the endpoint details (fork)
type ResponseTimeChartReader interface {
	// GetResponseTimeBuckets returns the buckets of bucketSeconds of the endpoint with the given key whose start is
	// between from and to, both included, from the oldest to the most recent. An unknown key returns an empty list.
	GetResponseTimeBuckets(key string, bucketSeconds int, from, to time.Time) ([]common.ResponseTimeBucket, error)

	// GetRecentResponseTimeResults returns the latest limit results of the endpoint with the given key, without the
	// results of suites, from the oldest to the most recent timestamp. An unknown key returns an empty list.
	GetRecentResponseTimeResults(key string, limit int) ([]common.RecentResponseTimeResult, error)
}

// GetResponseTimeChartReader returns the storage provider as a ResponseTimeChartReader, if it supports it
func GetResponseTimeChartReader() (ResponseTimeChartReader, bool) {
	reader, ok := Get().(ResponseTimeChartReader)
	return reader, ok
}
