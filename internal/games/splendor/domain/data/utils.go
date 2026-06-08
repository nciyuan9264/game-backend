package data

import (
	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
)

// InitPlayerData 初始化指定玩家的局内数据（内存版，等价旧 ws/player.go::InitPlayerData）。
func InitPlayerData(r *domain.Room, playerID string) {
	r.State.Players[playerID] = &domain.PlayerState{
		NormalCard: []entities.NormalCard{},
		NobleCard:  []entities.NobleCard{},
		Gem: map[string]int{
			"Red":   0,
			"Green": 0,
			"White": 0,
			"Blue":  0,
			"Black": 0,
			"Gold":  0,
		},
		Score:       0,
		ReserveCard: []entities.NormalCard{},
	}
}
