package game

import (
	"encoding/json"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

type HandleGetCardPayload struct {
	ID string `json:"id"`
}

// ComputeInsertIndex 返回新数字牌应插入的 index：
// 按当前实际 Index 顺序，跳过王牌(num=-1)，找到第一张严格大于 newCard
// 的数字牌，返回它的 Index；若不存在则返回末尾槽位。
// 调用方随后需把所有 Index >= 返回值的牌 +1，再把 newCard.Index 设为返回值。
func ComputeInsertIndex(cards []*domain.Card, newCard *domain.Card) int {
	count := 0
	pos := -1
	for _, c := range cards {
		if c == nil || c.ID == newCard.ID {
			continue
		}
		count++
		if c.Num == domain.NumMinus1 {
			continue
		}
		if cardOrderLess(newCard.Num, newCard.Color, c.Num, c.Color) {
			if pos == -1 || c.Index < pos {
				pos = c.Index
			}
		}
	}
	if pos == -1 {
		return count
	}
	return pos
}

// cardOrderLess 数字升序、同数字黑牌在白牌前：a 是否应排在 b 之前。
func cardOrderLess(numA domain.CardNumber, colorA domain.Color, numB domain.CardNumber, colorB domain.Color) bool {
	if numA != numB {
		return numA < numB
	}
	return colorA == domain.ColorBlack && colorB == domain.ColorWhite
}

// HandleGetCardMessage 用于处理玩家获取牌的操作
func HandleGetCardMessage(r *domain.Room, cmd domain.Command) error {
	var p HandleGetCardPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		logger.Error("消息格式错误", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("error", err))
		return err
	}
	id := p.ID
	r.State.Players[cmd.PlayerID].Cards = append(r.State.Players[cmd.PlayerID].Cards, r.State.BoardCards[id])
	delete(r.State.BoardCards, id)
	r.State.RoomStatus = domain.RoomStatusGuessCard
	return nil
}

type HandleGuessCardPayload struct {
	ID  string            `json:"id"`
	Num domain.CardNumber `json:"num"`
}

func HandleGuessCardMessage(r *domain.Room, cmd domain.Command) error {
	var p HandleGuessCardPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		logger.Error("消息格式错误", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("error", err))
		return err
	}
	id := p.ID
	num := p.Num

	var target *domain.Card
	var owner string
	for pid, ps := range r.State.Players {
		for _, c := range ps.Cards {
			if c != nil && c.ID == id {
				target = c
				owner = pid
				break
			}
		}
		if target != nil {
			break
		}
	}
	if target == nil {
		logger.Error("未找到目标卡片", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("card_id", id))
		return nil
	}
	if owner == cmd.PlayerID {
		logger.Error("不能猜测自己的卡片", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("card_id", id))
		return nil
	}
	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"targetCardID":   id,
		"targetPlayerID": owner,
		"guessNum":       num,
		"correct":        target.Num == num,
	})
	r.State.LastData = &domain.LastAction{
		Action:   "guess_card",
		PlayerID: cmd.PlayerID,
		Payload:  payloadJSON,
	}
	if target.Num == num {
		target.IsRevealed = true
		allRevealed := true
		if ps, ok := r.State.Players[owner]; ok && ps != nil {
			for _, c := range ps.Cards {
				if c != nil && !c.IsRevealed {
					allRevealed = false
					break
				}
			}
		}
		if allRevealed {
			r.State.RoomStatus = domain.RoomStatusEnd
		} else if r.State.RoomStatus == domain.RoomStatusGuessCard {
			r.State.RoomStatus = domain.RoomStatusSetCard
		}
		BroadcastToRoom(r)
		return nil
	}

	ps := r.State.Players[cmd.PlayerID]
	if ps != nil {
		var newCard *domain.Card
		for _, c := range ps.Cards {
			if c != nil && c.Index == -1 {
				newCard = c
				break
			}
		}
		if newCard != nil {
			newCard.IsRevealed = true
			pos := ComputeInsertIndex(ps.Cards, newCard)
			for _, c := range ps.Cards {
				if c == nil || c.ID == newCard.ID {
					continue
				}
				if c.Index >= pos {
					c.Index++
				}
			}
			newCard.Index = pos
		}
	}
	if err := SwitchToNextPlayer(r, cmd.PlayerID); err != nil {
		logger.Error("切换玩家失败", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("error", err))
		return err
	}
	if len(r.State.BoardCards) == 0 {
		r.State.RoomStatus = domain.RoomStatusGuessCard
	} else {
		r.State.RoomStatus = domain.RoomStatusGetCard
	}
	return nil
}

type HandleSetCardPayload struct {
	ID    string `json:"id"`
	Index int    `json:"index"`
}

func HandleSetCardMessage(r *domain.Room, cmd domain.Command) error {
	var p HandleSetCardPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		logger.Error("消息格式错误", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("error", err))
		return err
	}
	id := p.ID
	index := p.Index
	ps := r.State.Players[cmd.PlayerID]
	if index < 0 {
		index = 0
	}
	var target *domain.Card
	for _, c := range ps.Cards {
		if c != nil && c.ID == id {
			target = c
			break
		}
	}
	if target == nil {
		logger.Error("未找到需要设置的位置卡片", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("card_id", id))
		return nil
	}
	for _, c := range ps.Cards {
		if c == nil || c.ID == target.ID {
			continue
		}
		if c.Index >= index {
			c.Index++
		}
	}
	target.Index = index
	if len(r.State.BoardCards) == 0 {
		r.State.RoomStatus = domain.RoomStatusGuessCard
	} else {
		r.State.RoomStatus = domain.RoomStatusGetCard
	}
	if err := SwitchToNextPlayer(r, cmd.PlayerID); err != nil {
		logger.Error("切换玩家失败", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("error", err))
		return err
	}
	return nil
}
