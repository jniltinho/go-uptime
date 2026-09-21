// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package endpoint

// ConditionResult result of a Condition: the outcome of one condition of an endpoint for a given Result, as
// serialized in the status API.
type ConditionResult struct {
	// Condition that was evaluated. When it failed (or when ui.resolve-successful-conditions is set), the placeholders
	// are followed by their resolved value, e.g. "[STATUS] (502) == 200".
	Condition string `json:"condition"`

	// Success whether the condition was met (successful) or not (failed)
	Success bool `json:"success"`
}
