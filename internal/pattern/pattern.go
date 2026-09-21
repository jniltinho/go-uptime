// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

// Package pattern matches strings against the glob patterns of the conditions, e.g. pat(*ok*).
package pattern

import (
	"path/filepath"
	"strings"
)

// Match checks whether a string matches a pattern
func Match(pattern, s string) bool {
	if pattern == "*" {
		return true
	}
	// Separators found in the string break filepath.Match, so we'll remove all of them.
	// This has a pretty significant impact on performance when there are separators in
	// the strings, but at least it doesn't break filepath.Match.
	s = strings.ReplaceAll(s, string(filepath.Separator), "")
	pattern = strings.ReplaceAll(pattern, string(filepath.Separator), "")
	matched, _ := filepath.Match(pattern, s)
	return matched
}
