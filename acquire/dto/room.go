package dto

import "go-game/domain/domain"

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

type CreateRoomResponse struct {
	RoomID string `json:"roomID" binding:"required"`
}

type GetRoomList struct {
	Rooms []RoomInfo `json:"rooms"`
}
