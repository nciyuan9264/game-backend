package dto

import "go-game/entities"

type SplendorPlayerData struct {
	Card        map[string]int        `json:"card"`
	Gem         map[string]int        `json:"gem"`
	Score       int                   `json:"score"`
	ReserveCard []entities.NormalCard `json:"reserveCard"`
	NobleCard   []entities.NobleCard  `json:"nobleCard"`
}
