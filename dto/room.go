package dto

type RoomInfo struct {
	RoomID     string `json:"roomID"`
	MaxPlayers int    `json:"maxPlayers"`
	Status     string `json:"status"`
}

type CreateRoomRequest struct {
	MaxPlayers int `json:"maxPlayers" binding:"required"`
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
