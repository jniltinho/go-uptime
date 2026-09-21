// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"math"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

const hoursPerDay = 24

// certificateExpiration returns the number of whole days between now and the expiration of the TLS certificate of the
// most recent result with a certificate, rounded down (negative once the certificate expired), and the instant of that
// expiration in UTC. Both come from the same result, and both are nil when no result has a certificate. Pushes and
// failed connections have no certificate and are skipped, so that they do not hide the expiration read by the last
// check (fork).
func certificateExpiration(results []common.ResultSummary, now time.Time) (*int, *time.Time) {
	for i := len(results) - 1; i >= 0; i-- {
		if results[i].CertificateExpiration == 0 {
			continue
		}
		expiresAt := results[i].Timestamp.Add(results[i].CertificateExpiration).UTC()
		days := int(math.Floor(expiresAt.Sub(now).Hours() / hoursPerDay))
		return &days, &expiresAt
	}
	return nil, nil
}
