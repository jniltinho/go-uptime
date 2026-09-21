// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package suite

import (
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

// Result represents the result of a suite execution: one run of all the endpoints of a suite, as serialized in the
// suite status API.
type Result struct {
	// Name of the suite. Omitted when empty.
	Name string `json:"name,omitempty"`

	// Group of the suite. Omitted when the suite has no group.
	Group string `json:"group,omitempty"`

	// Success indicates whether all required endpoints succeeded and there was no suite-level error
	Success bool `json:"success"`

	// Timestamp is when the suite execution started, serialized in RFC 3339 format
	Timestamp time.Time `json:"timestamp"`

	// Duration is how long the entire suite execution took, serialized as an integer number of nanoseconds
	Duration time.Duration `json:"duration"`

	// EndpointResults contains the results of each endpoint execution, in execution order, each one carrying the name
	// of its endpoint. Endpoints skipped after a failure have no entry.
	EndpointResults []*endpoint.Result `json:"endpointResults"`

	// Context is the final state of the context after all endpoints executed
	Context map[string]interface{} `json:"-"`

	// Errors contains any suite-level errors, such as the timeout of the suite. Omitted when there is none.
	Errors []string `json:"errors,omitempty"`
}

// AddError adds an error to the suite result
func (r *Result) AddError(err string) {
	r.Errors = append(r.Errors, err)
}

// CalculateSuccess determines if the suite execution was successful and sets Success accordingly: true only if
// every endpoint result is successful and there is no suite-level error.
func (r *Result) CalculateSuccess() {
	r.Success = true
	// Check if any endpoints failed (all endpoints are required)
	for _, epResult := range r.EndpointResults {
		if !epResult.Success {
			r.Success = false
			break
		}
	}
	// Also check for suite-level errors
	if len(r.Errors) > 0 {
		r.Success = false
	}
}
