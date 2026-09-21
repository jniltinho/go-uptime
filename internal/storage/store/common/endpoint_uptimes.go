// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package common

import "time"

// EndpointUptimes is the uptime of an endpoint over the last 24 hours, 7 days and 30 days, between 0 and 1.
// A nil value means that there was no execution during the period.
type EndpointUptimes struct {
	Last24Hours *float64
	Last7Days   *float64
	Last30Days  *float64

	// AverageResponseTime24Hours, AverageResponseTime7Days and AverageResponseTime30Days are the average response times
	// in milliseconds over the same periods. A nil value means that there was no execution during the period.
	AverageResponseTime24Hours *int
	AverageResponseTime7Days   *int
	AverageResponseTime30Days  *int
}

// ResultSummary is the part of an endpoint result that can be shown on a public status page
type ResultSummary struct {
	Timestamp time.Time
	Success   bool
	Duration  time.Duration

	// CertificateExpiration is the duration between the result and the expiration of the TLS certificate, or zero without
	// certificate. Status pages only publish the days derived from it, when they are configured to (fork).
	CertificateExpiration time.Duration

	// Pending is whether the result is pending (fork)
	Pending bool

	// Connected is whether the connection could be established. Status pages never publish it: it only tells a network
	// failure, which registers no error for TCP, UDP, SCTP and ICMP, from a condition that failed with the service
	// answering (fork).
	Connected bool

	// Message, Origin, HTTPStatus and Errors are only used by the details page of a status page that shows messages, which
	// publishes the message, the origin and the HTTP status but never the errors: the errors are only read to recognize
	// the message of the heartbeat of results stored before it had one (fork)
	Message    string
	Origin     string
	HTTPStatus int
	Errors     []string
}

// EndpointSummary is the latest results and the uptimes of an endpoint, for the public status pages
type EndpointSummary struct {
	// Results are the latest results, from the oldest to the most recent
	Results []ResultSummary

	Uptimes EndpointUptimes
}
