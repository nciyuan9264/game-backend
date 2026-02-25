package entities

import "go-game/domain/domain"

type CompanyInfo struct {
	Name       string `json:"name"`
	StockPrice int    `json:"stockPrice"`
	StockTotal int    `json:"stockTotal"`
	Tiles      int    `json:"tiles"`
}

type RoomInfo struct {
	RoomStatus bool              `json:"roomStatus"`
	GameStatus domain.RoomStatus `json:"gameStatus"`
	MaxPlayers int               `json:"maxPlayers"`
	OwnerID    string            `json:"ownerID"`
}
