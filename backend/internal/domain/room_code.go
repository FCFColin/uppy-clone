package domain

import "fmt"

// RoomCode is a value object representing a 5-character room code.
// 企业为何需要：Value Object 封装校验逻辑，确保非法 RoomCode 无法被构造。
// 字符集为 [A-Z2-9]（去除易混淆的 0/1/I/O）。
//
// TODO(P3-4.3): 未来新代码应使用 RoomCode 替代裸 string 类型；
// 全量替换现有 string 类型风险过大，暂不进行。
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
