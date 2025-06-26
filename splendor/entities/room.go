package entities

type CompanyInfo struct {
	Name       string `json:"name"`
	StockPrice int    `json:"stockPrice"`
	StockTotal int    `json:"stockTotal"`
	Tiles      int    `json:"tiles"`
}

type MergingSelection struct {
	MainCompany  []string `json:"mainCompany"`
	OtherCompany []string `json:"otherCompany"`
}

type RoomInfo struct {
	RoomStatus bool       `json:"roomStatus"`
	GameStatus RoomStatus `json:"gameStatus"`
	MaxPlayers int        `json:"maxPlayers"`
	UserID     string     `json:"userID"`
}

type RoomStatus string

const (
	RoomStatusWaiting  RoomStatus = "waiting"   // 等待玩家加入房间
	RoomStatusPlaying  RoomStatus = "playing"   //等待玩家放置Tile
	RoomStatusLastTurn RoomStatus = "last_turn" // 最后一个玩家回合
	RoomStatusEnd      RoomStatus = "end"
)
