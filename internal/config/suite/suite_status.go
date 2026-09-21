// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package suite

// Status represents the status of a suite: its identity and one page of its most recent execution results, as
// returned by the suite status API.
type Status struct {
	// Name of the suite, as configured. Omitted when empty.
	Name string `json:"name,omitempty"`

	// Group the suite is a part of. Used for grouping multiple suites together on the front end.
	// Omitted when the suite has no group.
	Group string `json:"group,omitempty"`

	// Key of the Suite: the unique identifier built from the sanitized group and name joined by an underscore, which
	// is what the API routes take as {key}.
	Key string `json:"key"`

	// Results is the list of suite execution results, oldest first. It is an empty array, never null, when the suite
	// has not been executed yet.
	Results []*Result `json:"results"`
}

// NewStatus creates a new Status for a given Suite, with the key of the suite and an empty, non-nil list of results.
func NewStatus(s *Suite) *Status {
	return &Status{
		Name:    s.Name,
		Group:   s.Group,
		Key:     s.Key(),
		Results: []*Result{},
	}
}
