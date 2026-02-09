package dto

type RoomPlayer struct {
	PlayerID string `json:"playerID"`
	Online   bool   `json:"online"`
	AI       bool   `json:"ai"`
	Ready    bool   `json:"ready"`
}
type RoomInfo struct {
	RoomID         string       `json:"roomID"`
	Status         RoomStatus   `json:"status"`
	OwnerID        string       `json:"ownerID"`
	RoomPlayer     []RoomPlayer `json:"roomPlayer"`
	EmptyTileCount int          `json:"emptyTileCount"`
}

type PlayerInfo struct {
	Money int `json:"money"`
}

type CreateRoomRequest struct {
	MaxPlayers int    `json:"maxPlayers" binding:"required"`
	AiCount    int    `json:"aiCount"`
	UserID     string `json:"userID" binding:"required"`
}

type DeleteRoomRequest struct {
	RoomID string `json:"roomID" binding:"required"`
}

type CreateRoomResponse struct {
	RoomID string `json:"roomID" binding:"required"`
}

type GetRoomList struct {
	Rooms        []RoomInfo `json:"rooms"`
	OnlinePlayer int        `json:"onlinePlayer"`
}

type Tile struct {
	ID     string `json:"id"`     // "1A"
	Belong string `json:"belong"` // 公司名
}

type PlayerConn struct {
	PlayerID string        `json:"playerID"`
	Conn     ConnInterface `json:"-"`
	Online   bool          `json:"online"`
	Ready    bool          `json:"ready"`
	AI       bool          `json:"ai"`
}
