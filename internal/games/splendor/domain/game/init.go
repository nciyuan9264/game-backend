package game

import (
	"math/rand"
	"strconv"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/const_data"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
)

// GetRandomNobles 返回随机 nobles，前 max 张设为 Revealed。
func GetRandomNobles(max int) []entities.NobleCard {
	nobleList := make([]entities.NobleCard, len(const_data.NobleTilesList))
	copy(nobleList, const_data.NobleTilesList)
	rand.Shuffle(len(nobleList), func(i, j int) {
		nobleList[i], nobleList[j] = nobleList[j], nobleList[i]
	})
	for i := range nobleList {
		if i < max {
			nobleList[i].State = entities.CardStateRevealed
		} else {
			nobleList[i].State = entities.CardStateHidden
		}
	}
	return nobleList
}

// InitRoomData 初始化牌堆 / 宝石 / 贵族（写入 r.State，纯内存）。
func InitRoomData(r *domain.Room) {
	r.State.NormalCards = make(map[string]*entities.NormalCard)
	for _, cards := range const_data.SplendorCards {
		shuffled := rand.Perm(len(cards))
		for idx, rnd := range shuffled {
			card := cards[rnd]
			if idx < 4 {
				card.State = entities.CardStateRevealed
			} else {
				card.State = entities.CardStateHidden
			}
			c := card
			r.State.NormalCards[strconv.Itoa(c.ID)] = &c
		}
	}

	// 宝石数量随人数变化：4 人 7 个、3 人 5 个、2 人 4 个；Gold 固定 5 个。
	gemPerColor := 7
	switch r.State.MaxPlayers {
	case 2:
		gemPerColor = 4
	case 3:
		gemPerColor = 5
	}
	r.State.Gems = map[string]int{
		"Blue":  gemPerColor,
		"Green": gemPerColor,
		"Red":   gemPerColor,
		"White": gemPerColor,
		"Black": gemPerColor,
		"Gold":  5,
	}

	max := r.State.MaxPlayers + 1
	nobles := GetRandomNobles(max)
	r.State.NobleCards = make(map[string]*entities.NobleCard)
	for i := range nobles {
		n := nobles[i]
		r.State.NobleCards[n.ID] = &n
	}
}
