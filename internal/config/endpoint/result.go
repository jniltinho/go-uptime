package endpoint

import (
	"slices"
	"time"
)

// Result of the evaluation of an Endpoint: the outcome of one health check made by Go Uptime, or of one result pushed
// to an external endpoint. It is serialized in the status API responses; the fields tagged json:"-" are only used
// while evaluating the conditions.
type Result struct {
	// HTTPStatus is the HTTP response status code. For SSH endpoints it carries the exit status of the command instead.
	// Zero (omitted from the JSON) when the check is not HTTP/SSH or no response was received.
	HTTPStatus int `json:"status,omitempty"`

	// DNSRCode is the response code of a DNS query in a human-readable format
	//
	// Possible values: NOERROR, FORMERR, SERVFAIL, NXDOMAIN, NOTIMP, REFUSED
	DNSRCode string `json:"-"`

	// Hostname extracted from Endpoint.URL, without scheme and port. Empty (omitted) when the URL could not be parsed,
	// when the endpoint sets ui.hide-hostname, or for pushed results.
	Hostname string `json:"hostname,omitempty"`

	// IP resolved from the Endpoint URL
	IP string `json:"-"`

	// Connected whether a connection to the host was established successfully
	Connected bool `json:"-"`

	// Duration time that the request took, serialized as an integer number of nanoseconds. Zero when the request could
	// not be made or, for pushed results, when the sender did not report a duration.
	Duration time.Duration `json:"duration"`

	// Errors encountered during the evaluation of the Endpoint's health, without duplicates. Omitted when there is
	// none or when the endpoint sets ui.hide-errors.
	Errors []string `json:"errors,omitempty"`

	// Message is the message of a pushed result (fork). It is never part of the public status pages.
	Message string `json:"message,omitempty"`

	// Origin is where the result comes from (fork): ResultOriginPush for pushed results, empty for the checks of Go Uptime
	Origin string `json:"origin,omitempty"`

	// Pending is whether the result is pending (fork): a push with status=pending, or a failure of a push endpoint
	// converted by its retries. A pending result is never successful, is left out of the alerts and of the events, and
	// counts as an execution without success in the uptime.
	Pending bool `json:"pending,omitempty"`

	// ConditionResults are the results of each of the Endpoint's Condition, in the configured order. Omitted when the
	// endpoint has no conditions (external endpoints) or sets ui.hide-conditions.
	ConditionResults []*ConditionResult `json:"conditionResults,omitempty"`

	// Success whether the result signifies a success or not: true only if there was no error and every condition
	// passed. Always false when Pending is true.
	Success bool `json:"success"`

	// Timestamp of the result, serialized in RFC 3339 format. It is set once the evaluation of the conditions is over
	// (or when the pushed result was received). In the results of a suite it is the moment the check of the endpoint
	// started, and Duration covers the whole evaluation.
	Timestamp time.Time `json:"timestamp"`

	// CertificateExpiration is the duration before the certificate expires. The fork publishes it in the protected status
	// API, so that the dashboard shows when the certificate expires. It is an integer number of nanoseconds counted from
	// the moment of the check, negative if the certificate has already expired, and zero (omitted) when the check did
	// not involve TLS.
	CertificateExpiration time.Duration `json:"certificateExpiration,omitempty"`

	// DomainExpiration is the duration before the domain expires
	DomainExpiration time.Duration `json:"-"`

	// Body is the response body
	//
	// Note that this field is not persisted in the storage.
	// It is used for health evaluation as well as debugging purposes.
	Body []byte `json:"-"`

	///////////////////////////////////////////////////////////////////////
	// Below is used only for the UI and is not persisted in the storage //
	///////////////////////////////////////////////////////////////////////
	port string `yaml:"-"` // used for endpoints[].ui.hide-port

	///////////////////////////////////
	// BELOW IS ONLY USED FOR SUITES //
	///////////////////////////////////
	// Name of the endpoint (ONLY USED FOR SUITES)
	// Group is not needed because it's inherited from the suite
	// Empty (omitted) for the results of regular endpoints.
	Name string `json:"name,omitempty"`
}

// AddError adds an error to the result's list of errors.
// It also ensures that there are no duplicates.
func (r *Result) AddError(error string) {
	if !slices.Contains(r.Errors, error) {
		r.Errors = append(r.Errors, error+"")
	}
}
