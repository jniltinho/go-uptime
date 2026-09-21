package endpoint

import "github.com/jniltinho/go-uptime/v7/internal/config/key"

// Status contains the evaluation Results of an Endpoint
// This is essentially a DTO: it is the object returned by the endpoint status API, holding the identity of an
// endpoint (or external endpoint), one page of its most recent results and one page of its events.
type Status struct {
	// Name of the endpoint, as configured. Omitted when empty.
	Name string `json:"name,omitempty"`

	// Group the endpoint is a part of. Used for grouping multiple endpoints together on the front end.
	// Omitted when the endpoint has no group.
	Group string `json:"group,omitempty"`

	// Key of the Endpoint: the unique identifier built from the sanitized group and name joined by an underscore
	// (see key.ConvertGroupAndNameToKey), which is what the API routes take as {key}.
	Key string `json:"key"`

	// Results is the list of endpoint evaluation results, oldest first. It is an empty array, never null, when the
	// endpoint has not been evaluated yet.
	Results []*Result `json:"results"`

	// Events is a list of events (start of the monitoring and changes between healthy and unhealthy), oldest first.
	// Omitted when there is none.
	Events []*Event `json:"events,omitempty"`

	// Uptime information on the endpoint's uptime
	//
	// Used by the memory store.
	//
	// To retrieve the uptime between two time, use store.GetUptimeByKey.
	Uptime *Uptime `json:"-"`
}

// NewStatus creates a new Status for the given group and name, with its Key computed from them and with empty,
// non-nil Results, Events and Uptime.
func NewStatus(group, name string) *Status {
	return &Status{
		Name:    name,
		Group:   group,
		Key:     key.ConvertGroupAndNameToKey(group, name),
		Results: make([]*Result, 0),
		Events:  make([]*Event, 0),
		Uptime:  NewUptime(),
	}
}
