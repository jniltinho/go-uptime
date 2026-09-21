// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package endpoint

import "unicode/utf8"

const (
	// ResultOriginPush is the origin of the results pushed through /api/push (fork)
	ResultOriginPush = "push"

	// MaximumResultMessageLength is the maximum length, in bytes, of the message of a result
	MaximumResultMessageLength = 1024

	// MaximumHeartbeatRetries is the maximum number of heartbeat retries of an external endpoint
	MaximumHeartbeatRetries = 100

	// HeartbeatMessagePrefix is the beginning of the message and of the error of a result recorded by the heartbeat of an
	// external endpoint, followed by its interval. Results stored before the message existed only have it in their errors.
	HeartbeatMessagePrefix = "heartbeat: no update received within "
)

// TruncateResultMessage returns message cut to at most MaximumResultMessageLength bytes, without splitting a UTF-8
// character
func TruncateResultMessage(message string) string {
	if len(message) <= MaximumResultMessageLength {
		return message
	}
	cut := MaximumResultMessageLength
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return message[:cut]
}
