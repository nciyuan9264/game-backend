package data

import (
	"fmt"
	"sort"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/arrayutil"

	"golang.org/x/exp/rand"
)

// GenerateAvailableCards 生成count个当前房间中未被任何玩家使用的 card
func GenerateAvailableCards(r *domain.Room, count int) ([]*domain.Card, error) {
	available := make([]*domain.Card, 0, len(r.State.BoardCards))
	for _, card := range r.State.BoardCards {
		available = append(available, card)
	}

	rand.Shuffle(len(available), func(i, j int) { available[i], available[j] = available[j], available[i] })
	cards := arrayutil.SafeSlice(available, count)

	return cards, nil
}

// 初始化玩家数据
func InitPlayerData(r *domain.Room, playerID string) error {
	if r.State.Players == nil {
		r.State.Players = make(map[string]*domain.PlayerState)
	}
	if r.State.Players[playerID] == nil {
		r.State.Players[playerID] = &domain.PlayerState{
			Cards: []*domain.Card{},
		}
	}
	cards, err := GenerateAvailableCards(r, 5)
	if err != nil {
		return fmt.Errorf("生成可用 cards 失败: %w", err)
	}

	// 非王牌按数字升序、同数字黑色在前排序；王牌(-1)可置于任意位置，随机插入。
	normal := make([]*domain.Card, 0, len(cards))
	jokers := make([]*domain.Card, 0)
	for _, card := range cards {
		if card.Num == domain.NumMinus1 {
			jokers = append(jokers, card)
		} else {
			normal = append(normal, card)
		}
	}

	sort.Slice(normal, func(i, j int) bool {
		if normal[i].Num != normal[j].Num {
			return normal[i].Num < normal[j].Num
		}
		ri := 0
		if normal[i].Color == domain.ColorWhite {
			ri = 1
		}
		rj := 0
		if normal[j].Color == domain.ColorWhite {
			rj = 1
		}
		return ri < rj
	})

	for _, joker := range jokers {
		pos := rand.Intn(len(normal) + 1)
		normal = append(normal, nil)
		copy(normal[pos+1:], normal[pos:])
		normal[pos] = joker
	}
	cards = normal

	for idx, card := range cards {
		card.Index = idx
		r.State.Players[playerID].Cards = append(r.State.Players[playerID].Cards, card)
		delete(r.State.BoardCards, card.ID)
	}

	return nil
}
