package game

import (
	"encoding/json"
	"log"
	"math/rand"
	"strconv"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
)

func HandleGetGemMessage(r *domain.Room, cmd domain.Command) {
	if cmd.PlayerID != r.State.CurrentPlayer {
		log.Printf("❌ 非当前玩家回合 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}
	var gemPayload map[string]int
	if err := json.Unmarshal(cmd.Payload, &gemPayload); err != nil {
		log.Printf("❌ get_gem payload 解析失败 room=%s err=%v", r.ID, err)
		return
	}

	playerState, ok := r.State.Players[cmd.PlayerID]
	if !ok || playerState == nil {
		log.Printf("❌ 玩家不存在 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}

	for color, num := range gemPayload {
		if r.State.Gems[color] < num {
			log.Printf("❌ 房间宝石不足 color=%s have=%d want=%d", color, r.State.Gems[color], num)
			return
		}
	}
	for color, num := range gemPayload {
		r.State.Gems[color] -= num
		playerState.Gem[color] += num
	}

	payloadJSON, _ := json.Marshal(gemPayload)
	r.State.LastData = &domain.LastAction{
		Action:   "get_gem",
		PlayerID: cmd.PlayerID,
		Payload:  payloadJSON,
	}

	SwitchToNextPlayer(r)
}

func HandleBuyCardMessage(r *domain.Room, cmd domain.Command) {
	if cmd.PlayerID != r.State.CurrentPlayer {
		log.Printf("❌ 非当前玩家回合 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}
	var cardID int
	if err := json.Unmarshal(cmd.Payload, &cardID); err != nil {
		log.Printf("❌ buy_card payload 解析失败 room=%s err=%v", r.ID, err)
		return
	}

	card, ok := r.State.NormalCards[strconv.Itoa(cardID)]
	if !ok || card == nil {
		log.Printf("❌ 卡牌不存在 room=%s card=%d", r.ID, cardID)
		return
	}

	playerState, ok := r.State.Players[cmd.PlayerID]
	if !ok || playerState == nil {
		log.Printf("❌ 玩家不存在 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}

	cardCount := make(map[string]int)
	for _, c := range playerState.NormalCard {
		cardCount[c.Bonus]++
	}

	paidGems := make(map[string]int)
	remainingGold := playerState.Gem["Gold"]
	canBuy := true
	for color, cost := range card.Cost {
		owned := playerState.Gem[color] + cardCount[color]
		if owned >= cost {
			if cardCount[color] < cost {
				paidGems[color] = cost - cardCount[color]
			}
		} else {
			needGold := cost - owned
			if remainingGold >= needGold {
				paidGems[color] = playerState.Gem[color]
				paidGems["Gold"] += needGold
				remainingGold -= needGold
			} else {
				canBuy = false
				break
			}
		}
	}
	if !canBuy {
		log.Printf("❌ 玩家宝石不足 room=%s player=%s card=%d", r.ID, cmd.PlayerID, cardID)
		return
	}
	for color, amt := range paidGems {
		playerState.Gem[color] -= amt
		r.State.Gems[color] += amt
	}

	playerState.NormalCard = append(playerState.NormalCard, *card)

	if card.State == entities.CardStateBought {
		// 来自玩家保留卡 — 从 ReserveCard 中移除该 ID
		for i, c := range playerState.ReserveCard {
			if c.ID == card.ID {
				playerState.ReserveCard = append(playerState.ReserveCard[:i], playerState.ReserveCard[i+1:]...)
				break
			}
		}
	} else {
		// 翻一张同 Level 的隐藏卡到 Revealed
		hiddenIDs := make([]string, 0)
		for k, c := range r.State.NormalCards {
			if c.State == entities.CardStateHidden && c.Level == card.Level {
				hiddenIDs = append(hiddenIDs, k)
			}
		}
		if len(hiddenIDs) > 0 {
			pick := hiddenIDs[rand.Intn(len(hiddenIDs))]
			r.State.NormalCards[pick].State = entities.CardStateRevealed
		}
		card.State = entities.CardStateBought
	}

	// noble 结算
	bonusCount := make(map[string]int)
	for _, c := range playerState.NormalCard {
		bonusCount[c.Bonus]++
	}
	for nobleID, n := range r.State.NobleCards {
		if n.State != entities.CardStateRevealed {
			continue
		}
		ok := true
		for color, req := range n.Cost {
			if bonusCount[color] < req {
				ok = false
				break
			}
		}
		if ok {
			n.State = entities.CardStateBought
			playerState.NobleCard = append(playerState.NobleCard, *n)
			_ = nobleID
		}
	}

	payloadJSON, _ := json.Marshal(card)
	r.State.LastData = &domain.LastAction{
		Action:   "buy_card",
		PlayerID: cmd.PlayerID,
		Payload:  payloadJSON,
	}

	// 将原 BroadcastToRoom 中的 score 计算 + 状态切换搬到这里
	score := 0
	for _, c := range playerState.NormalCard {
		score += c.Points
	}
	for _, n := range playerState.NobleCard {
		score += n.Points
	}
	playerState.Score = score

	if score >= 15 && r.State.RoomStatus == domain.RoomStatusPlaying {
		if r.State.CurrentPlayer != r.State.FirstPlayer {
			r.State.RoomStatus = domain.RoomStatusLastTurn
		} else {
			r.State.RoomStatus = domain.RoomStatusEnd
		}
	}

	SwitchToNextPlayer(r)

	if r.State.CurrentPlayer == r.State.FirstPlayer && r.State.RoomStatus == domain.RoomStatusLastTurn {
		r.State.RoomStatus = domain.RoomStatusEnd
	}
}

func HandleReserveCardMessage(r *domain.Room, cmd domain.Command) {
	if cmd.PlayerID != r.State.CurrentPlayer {
		log.Printf("❌ 非当前玩家回合 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}
	var cardID int
	if err := json.Unmarshal(cmd.Payload, &cardID); err != nil {
		log.Printf("❌ preserve_card payload 解析失败 room=%s err=%v", r.ID, err)
		return
	}

	card, ok := r.State.NormalCards[strconv.Itoa(cardID)]
	if !ok || card == nil {
		log.Printf("❌ 卡牌不存在 room=%s card=%d", r.ID, cardID)
		return
	}

	playerState, ok := r.State.Players[cmd.PlayerID]
	if !ok || playerState == nil {
		log.Printf("❌ 玩家不存在 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}

	if r.State.Gems["Gold"] <= 0 {
		log.Printf("❌ 房间宝石不足 room=%s", r.ID)
		return
	}
	if len(playerState.ReserveCard) >= 3 {
		log.Printf("❌ 玩家保留卡牌已满 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}

	r.State.Gems["Gold"]--
	playerState.Gem["Gold"]++

	playerState.ReserveCard = append(playerState.ReserveCard, entities.NormalCard{
		ID:     card.ID,
		Level:  card.Level,
		Bonus:  card.Bonus,
		Points: card.Points,
		Cost:   card.Cost,
		State:  entities.CardStateBought,
	})

	hiddenIDs := make([]string, 0)
	for k, c := range r.State.NormalCards {
		if c.State == entities.CardStateHidden && c.Level == card.Level {
			hiddenIDs = append(hiddenIDs, k)
		}
	}
	if len(hiddenIDs) > 0 {
		pick := hiddenIDs[rand.Intn(len(hiddenIDs))]
		r.State.NormalCards[pick].State = entities.CardStateRevealed
	}
	card.State = entities.CardStateBought

	payloadJSON, _ := json.Marshal(card)
	r.State.LastData = &domain.LastAction{
		Action:   "preserve_card",
		PlayerID: cmd.PlayerID,
		Payload:  payloadJSON,
	}

	SwitchToNextPlayer(r)
}
