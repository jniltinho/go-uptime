// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package config

// toPtr returns a pointer to the given value
func toPtr[T any](value T) *T {
	return &value
}
