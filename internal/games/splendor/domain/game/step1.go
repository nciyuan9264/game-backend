package game

import (
	"encoding/json"
	"log"
	"math/rand"
	"strconv"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
)

// gemColors 是可作为普通宝石拿取的五种颜色（不含 Gold 万能币）。
var gemColors = map[string]bool{
	"Red":   true,
	"Green": true,
	"White": true,
	"Blue":  true,
	"Black": true,
}

// maxTokensInHand splendor 规则：玩家手中宝石（含 Gold）上限为 10。
const maxTokensInHand = 10

// countTokens 统计玩家手中宝石总数（含 Gold）。
func countTokens(ps *domain.PlayerState) int {
	total := 0
	for _, n := range ps.Gem {
		total += n
	}
	return total
}

// HandleGetGemMessage 处理拿取宝石：
//   - 要么拿 3 个不同颜色各 1 个（不足时可少拿）；
//   - 要么拿同色 2 个（仅当该色供应 >= 4 时）；
//   - 不能拿 Gold（万能币只能通过保留卡获得）；
//   - 拿完后手中宝石不得超过 10。
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

	// 归一化 payload：丢弃非正数，校验颜色与供应量。
	requested := make(map[string]int)
	totalRequested := 0
	maxNum := 0
	for color, num := range gemPayload {
		if num <= 0 {
			continue
		}
		if color == "Gold" {
			log.Printf("❌ 不允许直接拿取 Gold room=%s player=%s", r.ID, cmd.PlayerID)
			return
		}
		if !gemColors[color] {
			log.Printf("❌ 非法宝石颜色 room=%s color=%s", r.ID, color)
			return
		}
		if r.State.Gems[color] < num {
			log.Printf("❌ 房间宝石不足 color=%s have=%d want=%d", color, r.State.Gems[color], num)
			return
		}
		requested[color] = num
		totalRequested += num
		if num > maxNum {
			maxNum = num
		}
	}

	if totalRequested == 0 {
		log.Printf("❌ 未选择任何宝石 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}
	if maxNum >= 3 {
		log.Printf("❌ 单色一次最多拿 2 个 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}
	if maxNum == 2 {
		// 拿同色 2 个：必须只选了这一种颜色，且该色供应 >= 4。
		if len(requested) != 1 {
			log.Printf("❌ 拿 2 个同色时不能同时拿其他颜色 room=%s player=%s", r.ID, cmd.PlayerID)
			return
		}
		for color := range requested {
			if r.State.Gems[color] < 4 {
				log.Printf("❌ 该色不足 4 个，不能拿 2 个 room=%s color=%s have=%d", r.ID, color, r.State.Gems[color])
				return
			}
		}
	} else {
		// 全部为 1：拿不同色，最多 3 种。
		if len(requested) > 3 {
			log.Printf("❌ 一次最多拿 3 种不同颜色 room=%s player=%s", r.ID, cmd.PlayerID)
			return
		}
	}

	if countTokens(playerState)+totalRequested > maxTokensInHand {
		log.Printf("❌ 拿取后将超过 %d 个宝石上限 room=%s player=%s", maxTokensInHand, r.ID, cmd.PlayerID)
		return
	}

	for color, num := range requested {
		r.State.Gems[color] -= num
		playerState.Gem[color] += num
	}

	payloadJSON, _ := json.Marshal(requested)
	r.State.LastData = &domain.LastAction{
		Action:   "get_gem",
		PlayerID: cmd.PlayerID,
		Payload:  payloadJSON,
	}

	SwitchToNextPlayer(r)
}

// HandleBuyCardMessage 处理购买发展卡（来自桌面明牌或玩家自己的保留卡）。
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

	playerState, ok := r.State.Players[cmd.PlayerID]
	if !ok || playerState == nil {
		log.Printf("❌ 玩家不存在 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}

	// 定位卡牌：优先从玩家自己的保留卡中找；否则必须是桌面上的明牌。
	var card *entities.NormalCard
	fromReserve := false
	reserveIdx := -1
	for i := range playerState.ReserveCard {
		if playerState.ReserveCard[i].ID == cardID {
			card = &playerState.ReserveCard[i]
			fromReserve = true
			reserveIdx = i
			break
		}
	}
	if !fromReserve {
		c, exists := r.State.NormalCards[strconv.Itoa(cardID)]
		if !exists || c == nil {
			log.Printf("❌ 卡牌不存在 room=%s card=%d", r.ID, cardID)
			return
		}
		if c.State != entities.CardStateRevealed {
			log.Printf("❌ 卡牌不可购买（非桌面明牌）room=%s card=%d state=%d", r.ID, cardID, c.State)
			return
		}
		card = c
	}

	// 计算支付：先用已有发展卡的折扣，再用宝石，最后用 Gold 补足。
	cardCount := make(map[string]int)
	for _, c := range playerState.NormalCard {
		cardCount[c.Bonus]++
	}

	paidGems := make(map[string]int)
	remainingGold := playerState.Gem["Gold"]
	canBuy := true
	for color, cost := range card.Cost {
		discounted := cost - cardCount[color]
		if discounted <= 0 {
			continue
		}
		if playerState.Gem[color] >= discounted {
			paidGems[color] += discounted
		} else {
			needGold := discounted - playerState.Gem[color]
			if remainingGold >= needGold {
				paidGems[color] += playerState.Gem[color]
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

	// 加入玩家发展卡区。
	bought := *card
	bought.State = entities.CardStateBought
	playerState.NormalCard = append(playerState.NormalCard, bought)

	if fromReserve {
		// 从保留卡中移除（保留时桌面已补过牌，无需再补）。
		playerState.ReserveCard = append(playerState.ReserveCard[:reserveIdx], playerState.ReserveCard[reserveIdx+1:]...)
	} else {
		// 桌面买牌：翻一张同 Level 的隐藏卡补位。
		revealReplacement(r, card.Level)
		card.State = entities.CardStateBought
	}

	settleNoble(r, playerState)

	payloadJSON, _ := json.Marshal(bought)
	r.State.LastData = &domain.LastAction{
		Action:   "buy_card",
		PlayerID: cmd.PlayerID,
		Payload:  payloadJSON,
	}

	// 权威重算分数。
	RecomputeDerivedState(r)

	// 终局判定：任一玩家达到 15 分则进入最后一轮，回合循环回到首位玩家时结束。
	if playerState.Score >= 15 && r.State.RoomStatus == domain.RoomStatusPlaying {
		r.State.RoomStatus = domain.RoomStatusLastTurn
	}

	SwitchToNextPlayer(r)

	if r.State.RoomStatus == domain.RoomStatusLastTurn && r.State.CurrentPlayer == r.State.FirstPlayer {
		r.State.RoomStatus = domain.RoomStatusEnd
	}
}

// revealReplacement 翻开一张指定 Level 的隐藏卡到桌面。
func revealReplacement(r *domain.Room, level int) {
	hiddenIDs := make([]string, 0)
	for k, c := range r.State.NormalCards {
		if c.State == entities.CardStateHidden && c.Level == level {
			hiddenIDs = append(hiddenIDs, k)
		}
	}
	if len(hiddenIDs) > 0 {
		pick := hiddenIDs[rand.Intn(len(hiddenIDs))]
		r.State.NormalCards[pick].State = entities.CardStateRevealed
	}
}

// settleNoble 贵族结算：每回合最多迎接一位贵族（满足条件的取 ID 最小者）。
func settleNoble(r *domain.Room, playerState *domain.PlayerState) {
	bonusCount := make(map[string]int)
	for _, c := range playerState.NormalCard {
		bonusCount[c.Bonus]++
	}

	bestID := ""
	var bestNoble *entities.NobleCard
	for nobleID, n := range r.State.NobleCards {
		if n.State != entities.CardStateRevealed {
			continue
		}
		qualified := true
		for color, req := range n.Cost {
			if bonusCount[color] < req {
				qualified = false
				break
			}
		}
		if !qualified {
			continue
		}
		if bestID == "" || nobleID < bestID {
			bestID = nobleID
			bestNoble = n
		}
	}
	if bestNoble != nil {
		bestNoble.State = entities.CardStateBought
		playerState.NobleCard = append(playerState.NobleCard, *bestNoble)
	}
}

// HandleReserveCardMessage 处理保留卡：
//   - 玩家保留区上限为 3；
//   - 若房间仍有 Gold 且玩家手中宝石未满 10，则获得 1 个 Gold；
//   - Gold 用尽或手牌已满时仍可保留，只是拿不到 Gold。
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
	if card.State != entities.CardStateRevealed {
		log.Printf("❌ 卡牌不可保留（非桌面明牌）room=%s card=%d state=%d", r.ID, cardID, card.State)
		return
	}

	playerState, ok := r.State.Players[cmd.PlayerID]
	if !ok || playerState == nil {
		log.Printf("❌ 玩家不存在 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}

	if len(playerState.ReserveCard) >= 3 {
		log.Printf("❌ 玩家保留卡牌已满 room=%s player=%s", r.ID, cmd.PlayerID)
		return
	}

	// Gold 仅在房间有库存且玩家手牌未满 10 时获得。
	if r.State.Gems["Gold"] > 0 && countTokens(playerState) < maxTokensInHand {
		r.State.Gems["Gold"]--
		playerState.Gem["Gold"]++
	}

	playerState.ReserveCard = append(playerState.ReserveCard, entities.NormalCard{
		ID:     card.ID,
		Level:  card.Level,
		Bonus:  card.Bonus,
		Points: card.Points,
		Cost:   card.Cost,
		State:  entities.CardStateBought,
	})

	// 桌面补一张同 Level 的隐藏卡，并把原卡标记为已离开桌面。
	revealReplacement(r, card.Level)
	card.State = entities.CardStateBought

	payloadJSON, _ := json.Marshal(card)
	r.State.LastData = &domain.LastAction{
		Action:   "preserve_card",
		PlayerID: cmd.PlayerID,
		Payload:  payloadJSON,
	}

	SwitchToNextPlayer(r)
}
