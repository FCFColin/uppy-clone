package slogctx

import "strings"

type testKey struct{}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
