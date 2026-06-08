package dto

import "github.com/nciyuan9264/game-backend/internal/games/splendor/entities"

type SplendorPlayerData struct {
	NormalCard  []entities.NormalCard `json:"normalCard"`
	Gem         map[string]int        `json:"gem"`
	Score       int                   `json:"score"`
	ReserveCard []entities.NormalCard `json:"reserveCard"`
	NobleCard   []entities.NobleCard  `json:"nobleCard"`
}
