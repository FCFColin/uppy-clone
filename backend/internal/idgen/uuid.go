// Package idgen generates opaque identifiers for users and sessions.
package idgen

import (
	"crypto/rand"
	"fmt"
)

// UUID generates a v4 UUID string using crypto/rand.
// 企业为何需要：统一 UUID 生成逻辑消除 game/room.go 与 auth/quickplay.go 的重复实现，
// 集中维护便于未来切换到标准库（如 google/uuid）。
func UUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
