package game

import (
	"encoding/json"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

type HandleGetCardPayload struct {
	ID string `json:"id"`
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
			pos := 0
			for _, c := range ps.Cards {
				if c == nil || c.ID == newCard.ID {
					continue
				}
				if c.Num < newCard.Num {
					pos++
					continue
				}
				if c.Num == newCard.Num {
					oi := 0
					if c.Color == domain.ColorWhite {
						oi = 1
					}
					ti := 0
					if newCard.Color == domain.ColorWhite {
						ti = 1
					}
					if oi < ti {
						pos++
					}
				}
			}
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
