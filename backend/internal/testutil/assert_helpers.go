package testutil

import "strings"

// Contains reports whether substr is in s.
func Contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
