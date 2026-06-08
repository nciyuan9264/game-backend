package dto

import "github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"

type RoomPlayer struct {
	PlayerID string `json:"playerID"`
	Online   bool   `json:"online"`
	AI       bool   `json:"ai"`
	Ready    bool   `json:"ready"`
}
type RoomInfo struct {
	RoomID         string            `json:"roomID"`
	Status         domain.RoomStatus `json:"status"`
	OwnerID        string            `json:"ownerID"`
	RoomPlayer     []RoomPlayer      `json:"roomPlayer"`
	EmptyTileCount int               `json:"emptyTileCount"`
}

type CreateRoomRequest struct {
	MaxPlayers int    `json:"maxPlayers" binding:"required"`
	AiCount    int    `json:"aiCount"`
	UserID     string `json:"userID" binding:"required"`
}

type WsMatchSyncData struct {
	Type     string                `json:"type"`
	RoomID   string                `json:"roomID"`
	OwnerID  string                `json:"ownerID"`
	Status   domain.RoomStatus     `json:"status"`
	PlayerID string                `json:"playerID"`
	Players  map[string]RoomPlayer `json:"players"`
	Message  string                `json:"message"`
}
