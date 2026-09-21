// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package key builds the key that uniquely identifies an endpoint, an external endpoint or a suite in the storage
// and in the routes of the API.
package key

import "strings"

// ConvertGroupAndNameToKey converts a group and a name to a key of the form <group>_<name>, in which both parts are
// trimmed, converted to lowercase and have the characters "/", "_", ".", ",", " ", "#", "+" and "&" replaced by "-".
// An empty group gives a key that starts with the underscore.
func ConvertGroupAndNameToKey(groupName, name string) string {
	return sanitize(groupName) + "_" + sanitize(name)
}

func sanitize(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, ",", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "#", "-")
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "&", "-")
	return s
}
