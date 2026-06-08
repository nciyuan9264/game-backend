package dto

import "github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"

type RoomPlayer struct {
	PlayerID string `json:"playerID"`
	Online   bool   `json:"online"`
	AI       bool   `json:"ai"`
	Ready    bool   `json:"ready"`
}

type RoomInfo struct {
	RoomID     string            `json:"roomID"`
	OwnerID    string            `json:"ownerID"`
	MaxPlayers int               `json:"maxPlayers"`
	Status     domain.RoomStatus `json:"status"`
	RoomPlayer []RoomPlayer      `json:"roomPlayer"`
}

type DeleteRoomRequest struct {
	RoomID string `json:"roomID" binding:"required"`
}
