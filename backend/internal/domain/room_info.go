package domain

// RoomInfo 房间摘要信息
type RoomInfo struct {
	Code        string
	Phase       string
	PlayerCount int
	CreatedAt   int64
}
