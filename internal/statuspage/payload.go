// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package statuspage

import (
	"strconv"
	"strings"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
	pageconfig "github.com/jniltinho/go-uptime/v7/internal/config/statuspage"
	"github.com/jniltinho/go-uptime/v7/internal/storage/store/common"
)

// Fork: the reasons of a failed check published by a page that shows messages. They are a closed set: the text of the
// error is never published, because it carries the URL of the endpoint, the address of the DNS resolver of the
// installation and the certificate the host serves. They are constants of the public API, in English and not localized.
const (
	// ReasonCertificateError is a failure of TLS or of the certificate of the endpoint
	ReasonCertificateError = "Certificate error"

	// ReasonDNSError is a name that does not resolve
	ReasonDNSError = "DNS error"

	// ReasonTimeout is a check that ran out of time
	ReasonTimeout = "Timeout"

	// ReasonConnectionFailed is a connection refused, unreachable or closed, and a check that did not connect
	ReasonConnectionFailed = "Connection failed"

	// ReasonCheckFailed is any other failure, including a condition that failed with the service answering
	ReasonCheckFailed = "Check failed"

	// minimumHTTPStatus is the smallest HTTP status code that exists: below it the code is not from HTTP
	minimumHTTPStatus = 100
)

// failureReasons matches the errors of a result, in this order, against markers compared in lowercase
var failureReasons = []struct {
	text    string
	markers []string
}{
	{ReasonCertificateError, []string{"x509:", "tls:", "certificate is valid for", "certificate has expired", "certificate signed by", "unknown authority"}},
	{ReasonDNSError, []string{"no such host", "server misbehaving", "lookup ", "dns: "}},
	{ReasonTimeout, []string{"i/o timeout", "context deadline exceeded", "handshake timeout", "timeout awaiting"}},
	{ReasonConnectionFailed, []string{"connection refused", "no route to host", "network is unreachable", "connection reset by peer", "broken pipe", "unexpected eof", ": eof"}},
}

const (
	// StatusOperational means that every endpoint with results is up
	StatusOperational = "operational"

	// StatusDegraded means that some endpoints with results are up and others are down
	StatusDegraded = "degraded"

	// StatusDown means that every endpoint with results is down, or that the last result of an endpoint failed
	StatusDown = "down"

	// StatusUp means that the last result of an endpoint succeeded
	StatusUp = "up"

	// StatusPending means that the last result of an endpoint is pending (fork)
	StatusPending = "pending"

	// StatusUnknown means that there is no result
	StatusUnknown = "unknown"
)

// Payload is the public representation of a status page, the body of GET /api/v1/status-pages/{slug} and of the
// preview of the administration, GET /api/v1/admin/status-pages/{slug}/preview. It only has the fields that can be
// published: no key, URL, hostname, IP address, HTTP status, error, condition or event. The public route answers it
// from a cache of 30 seconds.
type Payload struct {
	// Slug is the slug of the page, the one of its public path /status/{slug}: 1 to 64 lowercase letters, digits or
	// hyphens.
	Slug string `json:"slug"`

	// Title is the title of the page, with 1 to 100 characters.
	Title string `json:"title"`

	// Description is the plain text shown below the title, with at most 1000 characters. It is an empty string when the
	// page has no description.
	Description string `json:"description"`

	// Status is the aggregated status of the endpoints of the page, ignoring the unknown ones: "operational" when all are
	// up, "down" when all are down, "degraded" otherwise (including when some are pending) and "unknown" when no
	// endpoint has a result.
	Status string `json:"status"`

	// UpdatedAt is the instant at which the payload was assembled, as a RFC 3339 timestamp in UTC. Because of the cache
	// it can be up to 30 seconds old.
	UpdatedAt time.Time `json:"updatedAt"`

	// Truncated is whether endpoints were left out because the page selects more than 200 of them. The featured
	// endpoints are kept first, then the groups in display order.
	Truncated bool `json:"truncated"`

	// GroupsCollapsed is the groups-collapsed option of the page: whether its groups start collapsed for a visitor who
	// made no choice of their own. A group whose status is not "operational" is shown expanded whatever this says.
	GroupsCollapsed bool `json:"groupsCollapsed"`

	// Summary counts the endpoints of the page by status, so that the page does not have to be counted in the browser
	// (fork)
	Summary SummaryPayload `json:"summary"`

	// Featured are the featured endpoints, in the order of the definition of the page, at most 10. They are not
	// repeated in Groups. It is an empty array, never null, when the page has none.
	Featured []FeaturedEndpointPayload `json:"featured"`

	// Groups are the sections of the page: first the groups selected by the page, in the order of its definition, then
	// the groups only reached through endpoint keys, in alphabetical order, then the endpoints without group. Groups
	// without endpoint to show are left out. It is an empty array, never null.
	Groups []GroupPayload `json:"groups"`
}

// SummaryPayload counts the endpoints of a status page by status (fork), in the summary field of Payload. On a
// truncated page it counts the endpoints that are published, which are the ones the page shows next to the notice of
// the first 200.
type SummaryPayload struct {
	// Total is the number of published endpoints, featured ones included: the sum of Up, Down, Pending and Unknown.
	Total int `json:"total"`

	// Up is the number of endpoints whose status is "up".
	Up int `json:"up"`

	// Down is the number of endpoints whose status is "down".
	Down int `json:"down"`

	// Pending is the number of endpoints whose status is "pending".
	Pending int `json:"pending"`

	// Unknown is the number of endpoints whose status is "unknown", which have no result yet.
	Unknown int `json:"unknown"`
}

// GroupPayload is a section of a public status page, an item of the groups field of Payload. Endpoints without group
// are in a group with an empty name.
type GroupPayload struct {
	// Name is the name of the group, trimmed, or an empty string for the section of the endpoints without group.
	Name string `json:"name"`

	// Status is the aggregated status of the endpoints of the group, ignoring the unknown ones: "operational" when all
	// are up, "down" when all are down, "degraded" otherwise (including when some are pending) and "unknown" when no
	// endpoint has a result.
	Status string `json:"status"`

	// Summary counts the endpoints listed in the group by status, with the rules of the summary of the page. Featured
	// endpoints are not listed in any group and are counted in none, only in the summary of the page. It is what the
	// header of a collapsed group shows.
	Summary SummaryPayload `json:"summary"`

	// Endpoints are the endpoints of the group that are not featured, ordered by name ignoring case. It is never empty.
	Endpoints []EndpointPayload `json:"endpoints"`
}

// FeaturedEndpointPayload is a featured endpoint, with the name of its group. It is an item of the featured field of
// Payload, with the fields of EndpointPayload at the same level.
type FeaturedEndpointPayload struct {
	EndpointPayload

	// Group is the name of the group of the endpoint as it is configured, or an empty string for an endpoint without
	// group.
	Group string `json:"group"`
}

// EndpointPayload is the public representation of an endpoint: an item of the endpoints field of GroupPayload, and the
// fields embedded at the top level of FeaturedEndpointPayload and of EndpointDetailsPayload. It has neither the key nor
// the URL of the endpoint.
type EndpointPayload struct {
	// Name is the display name of the endpoint, as configured. The key of the endpoint is not published here.
	Name string `json:"name"`

	// Status is the status of the endpoint, from its most recent result: "up" when it succeeded, "pending" when it is
	// pending, "down" when it failed, and "unknown" when the endpoint has no result yet.
	Status string `json:"status"`

	// Uptime is the uptime of the endpoint over the last 24 hours, 7 days and 30 days.
	Uptime UptimePayload `json:"uptime"`

	// ResponseTime is the average response time of the endpoint over the last 24 hours, 7 days and 30 days.
	ResponseTime ResponseTimePayload `json:"responseTime"`

	// Results are the latest results of the endpoint, from the oldest to the most recent: at most 50, or
	// storage.maximum-number-of-results when it is lower. It is an empty array, never null, without result.
	Results []ResultPayload `json:"results"`

	// CertificateExpiresInDays is the number of whole days until the TLS certificate of the endpoint expires, negative
	// once it expired. It is only set when the page shows the certificate expiration and a published result has a
	// certificate (fork).
	CertificateExpiresInDays *int `json:"certificateExpiresInDays,omitempty"`

	// CertificateExpiresAt is the instant of that expiration, from the same result, in UTC. The details page shows it
	// next to the number of days; the rows of the lists show only the days (fork).
	CertificateExpiresAt *time.Time `json:"certificateExpiresAt,omitempty"`
}

// UptimePayload is the uptime of an endpoint, between 0 and 1, or null without execution during the period. It is the
// uptime field of EndpointPayload, and also of the status of an endpoint answered by the dashboard API,
// GET /api/v1/endpoints/{key}/statuses.
type UptimePayload struct {
	// Last24Hours is the ratio of successful executions over the last 24 hours, from 0 to 1 (1 is 100%), or null
	// without execution during the period.
	Last24Hours *float64 `json:"24h"`

	// Last7Days is the ratio of successful executions over the last 7 days, from 0 to 1 (1 is 100%), or null without
	// execution during the period.
	Last7Days *float64 `json:"7d"`

	// Last30Days is the ratio of successful executions over the last 30 days, from 0 to 1 (1 is 100%), or null without
	// execution during the period.
	Last30Days *float64 `json:"30d"`
}

// ResponseTimePayload is the average response time of an endpoint in milliseconds, or null without execution during
// the period. It is the responseTime field of EndpointPayload, and also of the status of an endpoint answered by the
// dashboard API, GET /api/v1/endpoints/{key}/statuses.
type ResponseTimePayload struct {
	// Last24Hours is the average response time over the last 24 hours, in whole milliseconds, or null without execution
	// during the period.
	Last24Hours *int `json:"24h"`

	// Last7Days is the average response time over the last 7 days, in whole milliseconds, or null without execution
	// during the period.
	Last7Days *int `json:"7d"`

	// Last30Days is the average response time over the last 30 days, in whole milliseconds, or null without execution
	// during the period.
	Last30Days *int `json:"30d"`
}

// ResultPayload is the public representation of a result, an item of the results field of EndpointPayload. It has
// neither the errors, nor the condition results, nor the hostname, nor the IP address of the result, and the HTTP
// status is only published inside Message.
type ResultPayload struct {
	// Timestamp is the instant of the result, as a RFC 3339 timestamp in UTC: the end of the evaluation of a check, or
	// the reception of a push.
	Timestamp time.Time `json:"timestamp"`

	// Success is whether the result is healthy: every condition passed, or the push reported the status up.
	Success bool `json:"success"`

	// DurationMs is the response time of the result in whole milliseconds, rounded down. It is 0 when the request could
	// not be made and for a push that did not report a duration.
	DurationMs int64 `json:"durationMs"`

	// Pending is whether the result is pending (fork). A pending result has Success false and is not a failure. It is
	// omitted when false.
	Pending bool `json:"pending,omitempty"`

	// Message is the text of the result that can be published (fork): the message of a push or of a heartbeat (at most
	// 1024 bytes), else "HTTP " followed by the HTTP status code of the check, else, for a failed check that did not
	// answer, one of the closed set "Certificate error", "DNS error", "Timeout", "Connection failed" and "Check failed".
	// The text of the errors is never published. It is only set on the details page of an endpoint of a page that shows
	// messages, and omitted everywhere else and when there is nothing to say (a successful or pending check without
	// HTTP status).
	Message string `json:"message,omitempty"`

	// Origin is where the result comes from (fork): "push" for a result received through the push API, omitted for a
	// check made by Go Uptime. Like Message, it is only set on the details page of an endpoint of a page that shows
	// messages.
	Origin string `json:"origin,omitempty"`
}

// EndpointDetailsPayload is the public representation of an endpoint of a status page on its details page, the body
// of GET /api/v1/status-pages/{slug}/endpoints/{key}. Like Payload, it has no key, URL, hostname, IP address, HTTP
// status, error or condition; its events only have a type and a timestamp. The fields of EndpointPayload are at the
// same level as the others. It is answered from a cache of 30 seconds that a new result of the endpoint renews.
type EndpointDetailsPayload struct {
	// Page identifies the status page the endpoint is shown on.
	Page PageReferencePayload `json:"page"`

	EndpointPayload

	// Group is the name of the group of the endpoint as it is configured, or an empty string for an endpoint without
	// group.
	Group string `json:"group"`

	// UpdatedAt is the instant at which the payload was assembled, as a RFC 3339 timestamp in UTC.
	UpdatedAt time.Time `json:"updatedAt"`

	// Events are the latest events of the endpoint, at most 50, from the oldest to the most recent. Events of an unknown
	// type are left out. It is an empty array, never null, for an endpoint without event or not in the storage yet.
	Events []EventPayload `json:"events"`
}

// PageReferencePayload identifies the status page of an endpoint details page, in the page field of
// EndpointDetailsPayload
type PageReferencePayload struct {
	// Slug is the slug of the page, the one of its public path /status/{slug}.
	Slug string `json:"slug"`

	// Title is the title of the page, with 1 to 100 characters.
	Title string `json:"title"`

	// ShowMessages is whether the details page shows the messages of the results (fork)
	ShowMessages bool `json:"showMessages"`
}

// EventPayload is the public representation of an event: START, HEALTHY or UNHEALTHY. It is an item of the events
// field of EndpointDetailsPayload.
type EventPayload struct {
	// Type is the kind of the event: "START" when the endpoint started being monitored, "HEALTHY" when it became
	// healthy and "UNHEALTHY" when it became unhealthy.
	Type string `json:"type"`

	// Timestamp is the instant of the event, as a RFC 3339 timestamp in UTC.
	Timestamp time.Time `json:"timestamp"`
}

// BuildPayload builds the public representation of a page from its selection and the summaries of its endpoints. An
// endpoint without summary (not in the store yet) is unknown.
func BuildPayload(page *pageconfig.Page, selection Selection, summaries map[string]*common.EndpointSummary, now time.Time) *Payload {
	payload := &Payload{
		Slug:            page.Slug,
		Title:           page.Title,
		Description:     page.Description,
		UpdatedAt:       now.UTC(),
		Truncated:       selection.Truncated,
		GroupsCollapsed: page.GroupsCollapsed,
		Featured:        make([]FeaturedEndpointPayload, 0, len(selection.Featured)),
		Groups:          make([]GroupPayload, 0, len(selection.Sections)),
	}
	var pageStatuses []string
	for _, ref := range selection.Featured {
		endpointPayload := buildEndpointPayload(page, ref, summaries[ref.Key], now, false)
		payload.Featured = append(payload.Featured, FeaturedEndpointPayload{EndpointPayload: endpointPayload, Group: ref.Group})
		pageStatuses = append(pageStatuses, endpointPayload.Status)
	}
	for _, section := range selection.Sections {
		group := GroupPayload{Name: section.Group, Endpoints: make([]EndpointPayload, 0, len(section.Endpoints))}
		groupStatuses := make([]string, 0, len(section.Endpoints))
		for _, ref := range section.Endpoints {
			endpointPayload := buildEndpointPayload(page, ref, summaries[ref.Key], now, false)
			group.Endpoints = append(group.Endpoints, endpointPayload)
			groupStatuses = append(groupStatuses, endpointPayload.Status)
		}
		group.Status = aggregateStatus(groupStatuses)
		group.Summary = summaryOf(groupStatuses)
		payload.Groups = append(payload.Groups, group)
		pageStatuses = append(pageStatuses, groupStatuses...)
	}
	payload.Status = aggregateStatus(pageStatuses)
	payload.Summary = summaryOf(pageStatuses)
	return payload
}

// summaryOf counts the endpoints of a page, or of one of its groups, by status (fork)
func summaryOf(endpointStatuses []string) SummaryPayload {
	summary := SummaryPayload{Total: len(endpointStatuses)}
	for _, status := range endpointStatuses {
		switch status {
		case StatusUp:
			summary.Up++
		case StatusDown:
			summary.Down++
		case StatusPending:
			summary.Pending++
		default:
			summary.Unknown++
		}
	}
	return summary
}

// BuildEndpointDetailsPayload builds the public representation of an endpoint of a page for its details page, from its
// summary and its events, from the oldest to the most recent. Events of an unknown type are left out.
func BuildEndpointDetailsPayload(page *pageconfig.Page, ref EndpointRef, summary *common.EndpointSummary, events []*endpoint.Event, now time.Time) *EndpointDetailsPayload {
	payload := &EndpointDetailsPayload{
		Page:            PageReferencePayload{Slug: page.Slug, Title: page.Title, ShowMessages: page.ShowMessages},
		EndpointPayload: buildEndpointPayload(page, ref, summary, now, page.ShowMessages),
		Group:           ref.Group,
		UpdatedAt:       now.UTC(),
		Events:          make([]EventPayload, 0, len(events)),
	}
	for _, event := range events {
		switch event.Type {
		case endpoint.EventStart, endpoint.EventHealthy, endpoint.EventUnhealthy:
			payload.Events = append(payload.Events, EventPayload{Type: string(event.Type), Timestamp: event.Timestamp.UTC()})
		}
	}
	return payload
}

// buildEndpointPayload builds the public representation of an endpoint. The messages and the origins of its results are
// only set with withMessages, which is only used by the details page of a page that shows messages.
func buildEndpointPayload(page *pageconfig.Page, ref EndpointRef, summary *common.EndpointSummary, now time.Time, withMessages bool) EndpointPayload {
	endpointPayload := EndpointPayload{Name: ref.Name, Status: StatusUnknown, Results: []ResultPayload{}}
	if summary == nil {
		return endpointPayload
	}
	for _, result := range summary.Results {
		resultPayload := ResultPayload{
			Timestamp:  result.Timestamp.UTC(),
			Success:    result.Success,
			DurationMs: result.Duration.Milliseconds(),
			Pending:    result.Pending,
		}
		if withMessages {
			resultPayload.Message, resultPayload.Origin = publicMessage(result), result.Origin
		}
		endpointPayload.Results = append(endpointPayload.Results, resultPayload)
	}
	if page.ShowCertificateExpiration {
		endpointPayload.CertificateExpiresInDays, endpointPayload.CertificateExpiresAt = certificateExpiration(summary.Results, now)
	}
	if numberOfResults := len(summary.Results); numberOfResults > 0 {
		switch lastResult := summary.Results[numberOfResults-1]; {
		case lastResult.Success:
			endpointPayload.Status = StatusUp
		case lastResult.Pending:
			endpointPayload.Status = StatusPending
		default:
			endpointPayload.Status = StatusDown
		}
	}
	endpointPayload.Uptime, endpointPayload.ResponseTime = UptimePayloads(summary.Uptimes)
	return endpointPayload
}

// UptimePayloads returns the uptimes and the average response times of an endpoint, with nil values for the periods
// without execution
func UptimePayloads(uptimes common.EndpointUptimes) (UptimePayload, ResponseTimePayload) {
	return UptimePayload{Last24Hours: uptimes.Last24Hours, Last7Days: uptimes.Last7Days, Last30Days: uptimes.Last30Days},
		ResponseTimePayload{
			Last24Hours: uptimes.AverageResponseTime24Hours,
			Last7Days:   uptimes.AverageResponseTime7Days,
			Last30Days:  uptimes.AverageResponseTime30Days,
		}
}

// publicMessage returns the message of a result that can be published by a page that shows messages (fork): the message
// of a push or of the heartbeat; the message of the heartbeat found in the errors of a result stored before results had
// a message; the HTTP status of a check; or, for a check that failed without answering, the reason of the failure. The
// errors themselves are never published.
func publicMessage(result common.ResultSummary) string {
	if len(result.Message) > 0 {
		return result.Message
	}
	for _, resultError := range result.Errors {
		if strings.HasPrefix(resultError, endpoint.HeartbeatMessagePrefix) {
			return resultError
		}
	}
	// Only a real HTTP status: CheckSSHBanner answers 1 and a SSH command answers its exit code, which would be
	// published as "HTTP 1"
	if result.HTTPStatus >= minimumHTTPStatus {
		return "HTTP " + strconv.Itoa(result.HTTPStatus)
	}
	// A pushed result always has a message, and a pending result is not a failure
	if result.Success || result.Pending || len(result.Origin) > 0 {
		return ""
	}
	return failureReason(result)
}

// failureReason returns why a check failed, as one of the constants of failureReasons: the first category that matches
// any of the errors of the result, in the order of the table, and never a single character of the error itself. The
// markers are distinctive on purpose, because the errors of Go carry the URL of the endpoint: "certificate", "dns" and
// "timeout" alone would match a host like dns.example.org or a path like /certificate-status and publish a category
// chosen by the address the page hides.
func failureReason(result common.ResultSummary) string {
	if len(result.Errors) == 0 {
		// TCP, UDP, SCTP and ICMP fail without any error: only the connection tells a network failure from a condition
		// that failed with the service answering
		if result.Connected {
			return ReasonCheckFailed
		}
		return ReasonConnectionFailed
	}
	for _, reason := range failureReasons {
		for _, resultError := range result.Errors {
			lowercased := strings.ToLower(resultError)
			for _, marker := range reason.markers {
				if strings.Contains(lowercased, marker) {
					return reason.text
				}
			}
		}
	}
	return ReasonCheckFailed
}

// aggregateStatus returns the status of a group or a page from the statuses of its endpoints, ignoring unknown ones:
// operational when all are up, down when all are down, and degraded otherwise, including when some are pending (fork)
func aggregateStatus(endpointStatuses []string) string {
	var up, down, pending int
	for _, status := range endpointStatuses {
		switch status {
		case StatusUp:
			up++
		case StatusDown:
			down++
		case StatusPending:
			pending++
		}
	}
	switch {
	case up == 0 && down == 0 && pending == 0:
		return StatusUnknown
	case down == 0 && pending == 0:
		return StatusOperational
	case up == 0 && pending == 0:
		return StatusDown
	default:
		return StatusDegraded
	}
}
