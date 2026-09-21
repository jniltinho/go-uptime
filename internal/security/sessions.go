// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package security

import "github.com/TwiN/gocache/v2"

var sessions = gocache.NewCache().WithEvictionPolicy(gocache.LeastRecentlyUsed) // TODO: Move this to storage
