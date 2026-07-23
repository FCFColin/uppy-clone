package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// RoomInfo is a summary of a room, used for cache values and API responses.
type RoomInfo struct {
	Code        string `json:"code"`
	Phase       string `json:"phase"`
	PlayerCount int    `json:"player_count"`
	CreatedAt   int64  `json:"created_at"`
}

// RoomRegistryInfo is room ownership metadata stored in Redis (ADR-005).
type RoomRegistryInfo struct {
	Code      string `json:"code"`
	Instance  string `json:"instance"`
	Address   string `json:"address"`
	CreatedAt int64  `json:"created_at"`
}

// UnmarshalRoomRegistryInfo decodes JSON-encoded room registry metadata.
func UnmarshalRoomRegistryInfo(data []byte) (*RoomRegistryInfo, error) {
	var info RoomRegistryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

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

// LobbyState stores serialized game state for a lobby room.
type LobbyState struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	State     string `json:"state"`
	UpdatedAt int64  `json:"updated_at"`
	CreatedAt int64  `json:"created_at"`
}

// LobbyListResult contains paginated lobby results with metadata.
type LobbyListResult struct {
	Lobbies    []LobbyState
	Total      int
	HasMore    bool
	NextCursor string // format: "updated_at|code"
}

// LeaderboardEntry is a single row on the public leaderboard.
type LeaderboardEntry struct {
	Rank    int    `json:"rank"`
	Score   int    `json:"score"`
	Name    string `json:"name"`
	EndedAt int64  `json:"endedAt"`
}

// Nickname is a value object representing a sanitized player nickname.
type Nickname string

// NewNickname creates a Nickname from raw input using the provided validator.
func NewNickname(name string, v NicknameValidator) (Nickname, error) {
	sanitized := v(name)
	runes := []rune(sanitized)
	if len(runes) > MaxNicknameLen {
		runes = runes[:MaxNicknameLen]
	}
	if len(runes) == 0 {
		return "", fmt.Errorf("nickname cannot be empty")
	}
	return Nickname(string(runes)), nil
}

// String returns the string representation.
func (n Nickname) String() string {
	return string(n)
}

// NicknameValidator validates nickname strings.
type NicknameValidator func(string) string

// DefaultValidator is a ready-to-use NicknameValidator.
var DefaultValidator NicknameValidator = SanitizeNickname

var (
	controlCharsRegex   = regexp.MustCompile(`[\x00-\x1F\x7F-\x9F]`)
	invisibleCharsRegex = regexp.MustCompile(`[\x{200B}-\x{200F}\x{FEFF}\x{2028}-\x{202F}\x{2060}-\x{206F}]`)
	htmlCharsRegex      = regexp.MustCompile(`[<>"'\x60&]`)
	whitespaceRegex     = regexp.MustCompile(`\s+`)
)

// NicknameInputRejected reports whether raw input contains characters that must not
// be accepted as a client-provided nickname (control/HTML chars). Matches legacy game
// dangerousCharsRegex behavior for GenerateUniqueNickname.
func NicknameInputRejected(raw string) bool {
	return nicknameInputRejectedRegex.MatchString(raw)
}

var nicknameInputRejectedRegex = regexp.MustCompile(`[\x00-\x1f\x7f-\x9f<>"'&]`)

// SanitizeNickname sanitizes a player nickname.
// Removes control characters, zero-width chars, HTML special chars,
// trims whitespace, limits length to MaxNicknameLen runes, and collapses whitespace.
func SanitizeNickname(raw string) string {
	if raw == "" {
		return ""
	}
	raw = controlCharsRegex.ReplaceAllString(raw, "")
	raw = invisibleCharsRegex.ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)
	raw = htmlCharsRegex.ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)
	raw = whitespaceRegex.ReplaceAllString(raw, " ")
	runeSlice := []rune(raw)
	if len(runeSlice) > MaxNicknameLen {
		raw = string(runeSlice[:MaxNicknameLen])
	}
	return raw
}
