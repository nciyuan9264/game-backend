package dto

type Player struct {
	UserID    string         `json:"userId"`
	Name      string         `json:"name"`      // 玩家名称
	Cash      int            `json:"cash"`      // 初始现金，例如 6000 万
	Stocks    map[string]int `json:"stocks"`    // 股票持有情况：公司ID -> 股份数量
	Companies []string       `json:"companies"` // 持有的公司ID列表
	Status    string         `json:"status"`    // ready / playing / offline
}

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
