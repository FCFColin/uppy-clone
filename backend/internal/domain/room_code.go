package domain

import "fmt"

// RoomCode is a value object representing a 5-character room code.
// 字符集为 [A-Z2-9]（去除易混淆的 0/1/I/O）。
type RoomCode string

// NewRoomCode creates a RoomCode, returning an error if invalid.
func NewRoomCode(code string) (RoomCode, error) {
	if len(code) != 5 {
		return "", fmt.Errorf("room code must be 5 characters, got %d", len(code))
	}
	for _, c := range code {
		if (c < 'A' || c > 'Z') && (c < '2' || c > '9') {
			return "", fmt.Errorf("room code contains invalid character: %c", c)
		}
	}
	return RoomCode(code), nil
}

// String returns the string representation.
func (r RoomCode) String() string {
	return string(r)
}
