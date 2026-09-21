// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package store

import (
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/memory"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/sql"
)

var (
	_ EndpointSummaryBatchReader = (*memory.Store)(nil)
	_ EndpointSummaryBatchReader = (*sql.Store)(nil)
)

// EndpointSummaryBatchReader reads the latest results and the uptimes of many endpoints at once, for the public status
// pages
type EndpointSummaryBatchReader interface {
	// GetEndpointSummaries returns the latest maximumResults results, from the oldest to the most recent, and the uptimes
	// before now of the endpoints with the given keys. Keys of endpoints that do not exist in the store are absent from
	// the map; an endpoint that exists without execution in a period has a nil uptime for that period.
	GetEndpointSummaries(keys []string, maximumResults int, now time.Time) (map[string]*common.EndpointSummary, error)
}

// GetEndpointSummaryBatchReader returns the storage provider as an EndpointSummaryBatchReader, if it supports it
func GetEndpointSummaryBatchReader() (EndpointSummaryBatchReader, bool) {
	reader, ok := Get().(EndpointSummaryBatchReader)
	return reader, ok
}
