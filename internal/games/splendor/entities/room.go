package entities

type RoomInfo struct {
	RoomStatus bool       `json:"roomStatus"`
	GameStatus RoomStatus `json:"gameStatus"`
	MaxPlayers int        `json:"maxPlayers"`
	UserID     string     `json:"userID"`
}

type RoomStatus string

const (
	RoomStatusWaiting  RoomStatus = "waiting"   // 等待玩家加入房间
	RoomStatusPlaying  RoomStatus = "playing"   // 进行中
	RoomStatusLastTurn RoomStatus = "last_turn" // 最后一轮
	RoomStatusEnd      RoomStatus = "end"
)
