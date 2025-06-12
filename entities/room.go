package entities

import "go-game/dto"

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
	RoomStatus bool           `json:"roomStatus"`
	GameStatus dto.RoomStatus `json:"gameStatus"`
	MaxPlayers int            `json:"maxPlayers"`
}
