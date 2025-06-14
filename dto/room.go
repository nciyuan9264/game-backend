package dto

type RoomInfo struct {
	RoomID     string `json:"roomID"`
	UserID     string `json:"userID"`
	MaxPlayers int    `json:"maxPlayers"`
	Status     bool   `json:"status"`
}

type CreateRoomRequest struct {
	MaxPlayers int    `json:"maxPlayers" binding:"required"`
	UserID     string `json:"userID" binding:"required"`
}

type DeleteRoomRequest struct {
	RoomID string `json:"roomID" binding:"required"`
}

type CreateRoomResponse struct {
	Room_id string `json:"room_id" binding:"required"`
}

type GetRoomList struct {
	Rooms []RoomInfo `json:"rooms"`
}

type Tile struct {
	ID     string `json:"id"`     // "1A"
	Belong string `json:"belong"` // 公司名
}
