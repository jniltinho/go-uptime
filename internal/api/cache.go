// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package api

import (
	"time"

	"github.com/TwiN/gocache/v2"
)

const (
	// cacheTTL is how long a page of endpoint statuses stays in the cache
	cacheTTL = 10 * time.Second
)

var (
	// cache holds the encoded pages of GET /api/v1/endpoints/statuses, under the keys endpoint-status-<page>-<pageSize>.
	// The administration drops them when an endpoint is updated, deleted or restored.
	cache = gocache.NewCache().WithMaxSize(100).WithEvictionPolicy(gocache.FirstInFirstOut)
)
