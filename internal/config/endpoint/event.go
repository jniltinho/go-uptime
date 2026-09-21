// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package endpoint

import (
	"time"
)

// Event is something that happens at a specific time: the start of the monitoring of an endpoint or a change of its
// health. It is serialized as part of Status.
type Event struct {
	// Type is the kind of event: START, HEALTHY or UNHEALTHY
	Type EventType `json:"type"`

	// Timestamp is the moment at which the event happened, serialized in RFC 3339 format. For HEALTHY and UNHEALTHY
	// events it is the timestamp of the result that changed the health of the endpoint.
	Timestamp time.Time `json:"timestamp"`
}

// EventType is the kind of an Event, serialized as an upper case string (one of EventStart, EventHealthy and
// EventUnhealthy).
type EventType string

var (
	// EventStart is a type of event that represents when an endpoint starts being monitored
	EventStart EventType = "START"

	// EventHealthy is a type of event that represents an endpoint passing all of its conditions
	EventHealthy EventType = "HEALTHY"

	// EventUnhealthy is a type of event that represents an endpoint failing one or more of its conditions
	EventUnhealthy EventType = "UNHEALTHY"
)

// NewEventFromResult creates an Event from a Result, with the timestamp of the result and the type EventHealthy if
// the result is successful, EventUnhealthy otherwise.
func NewEventFromResult(result *Result) *Event {
	event := &Event{Timestamp: result.Timestamp}
	if result.Success {
		event.Type = EventHealthy
	} else {
		event.Type = EventUnhealthy
	}
	return event
}
