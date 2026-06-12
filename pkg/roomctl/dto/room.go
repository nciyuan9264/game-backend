// Package dto 定义跨游戏复用的房间相关 DTO，避免每个游戏维护一套形态相近的结构。
package dto

type RoomPlayer struct {
	PlayerID string `json:"playerID"`
	Online   bool   `json:"online"`
	AI       bool   `json:"ai"`
	Ready    bool   `json:"ready"`
}

// RoomInfo 是 /room/list 等接口的统一房间外壳。
// 游戏专属字段统一用 omitempty 承载，前端无需为每游戏维护单独类型。
type RoomInfo struct {
	RoomID     string       `json:"roomID"`
	Status     string       `json:"status"`
	OwnerID    string       `json:"ownerID"`
	RoomPlayer []RoomPlayer `json:"roomPlayer"`

	MaxPlayers     int `json:"maxPlayers,omitempty"`
	EmptyTileCount int `json:"emptyTileCount,omitempty"`
	BoardCardCount int `json:"boardCardCount,omitempty"`
	MaxScore       int `json:"maxScore,omitempty"`
}

type WsMatchSyncData struct {
	Type     string                `json:"type"`
	RoomID   string                `json:"roomID"`
	OwnerID  string                `json:"ownerID"`
	Status   string                `json:"status"`
	PlayerID string                `json:"playerID"`
	Players  map[string]RoomPlayer `json:"players"`
	Message  string                `json:"message,omitempty"`
}

type DeleteRoomRequest struct {
	RoomID string `json:"roomID" binding:"required"`
}
