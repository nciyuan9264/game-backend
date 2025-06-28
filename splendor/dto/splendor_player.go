package dto

import "go-game/entities"

type SplendorPlayerData struct {
	NormalCard  []entities.NormalCard `json:"normalCard"`
	Gem         map[string]int        `json:"gem"`
	Score       int                   `json:"score"`
	ReserveCard []entities.NormalCard `json:"reserveCard"`
	NobleCard   []entities.NobleCard  `json:"nobleCard"`
}
