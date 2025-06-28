package ws

import (
	"go-game/entities"
	"go-game/repository"
	"log"
	"strconv"

	"github.com/go-redis/redis/v8"
)

// 处理玩家放置 tile 消息
func handleBuyCardMessage(conn ReadWriteConn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	currentPlayer, err := GetCurrentPlayer(rdb, repository.Ctx, roomID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return
	}
	if currentPlayer != playerID {
		log.Println("❌ 不是当前玩家的回合")
		return
	}

	// 1. 获取卡牌 ID
	cardIDFloat, ok := msgMap["payload"].(float64)
	if !ok {
		log.Println("❌ 消息格式错误: payload 不是数字")
		return
	}
	cardID := int(cardIDFloat)

	// 2. 获取卡牌信息
	card, err := GetNormalCardByID(roomID, strconv.Itoa(cardID))
	if err != nil {
		log.Println("❌ 获取卡牌失败:", err)
		return
	}

	// 3. 获取玩家宝石
	playerGems, err := GetPlayerGem(roomID, playerID)
	if err != nil {
		log.Println("❌ 获取玩家宝石失败:", err)
		return
	}
	playerCard, err := GetPlayerNormalCard(roomID, playerID)
	if err != nil {
		log.Println("❌ 获取玩家卡牌失败:", err)
		return
	}

	cardCount := make(map[string]int)
	for _, c := range playerCard {
		cardCount[c.Bonus]++
	}

	// 4. 检查是否能支付
	required := card.Cost
	paidGems := make(map[string]int) // 实际支付宝石数
	remainingGold := playerGems["Gold"]

	canBuy := true
	for color, cost := range required {
		owned := playerGems[color] + cardCount[color]
		if owned >= cost {
			paidGems[color] = cost - cardCount[color]
		} else {
			needGold := cost - owned - cardCount[color]
			if remainingGold >= needGold {
				paidGems[color] = owned - cardCount[color]
				paidGems["Gold"] += needGold
				remainingGold -= needGold
			} else {
				canBuy = false
				break
			}
		}
	}

	if !canBuy {
		log.Println("❌ 玩家宝石不足，无法购买该卡牌")
		return
	}

	// 5. 扣除玩家宝石
	for color, amount := range paidGems {
		playerGems[color] -= amount
	}

	// 6. 更新玩家宝石
	if err := SetPlayerGem(roomID, playerID, playerGems); err != nil {
		log.Println("❌ 更新玩家宝石失败:", err)
		return
	}

	gemCount, err := GetGemCounts(roomID)
	if err != nil {
		log.Println("❌ 获取宝石数量失败:", err)
		return
	}

	for color, amount := range paidGems {
		gemCount[color] += amount
	}
	if err := SetGemCounts(roomID, gemCount); err != nil {
		log.Println("❌ 设置宝石数量失败:", err)
		return
	}

	// 7. 玩家获得该颜色卡牌加 1（假设 SetPlayerCard 函数存在）
	playerCards, err := GetPlayerNormalCard(roomID, playerID)
	if err != nil {
		log.Println("❌ 获取玩家卡牌失败:", err)
		return
	}

	playerCards = append(playerCards, *card)

	if err := SetPlayerNormalCard(roomID, playerID, playerCards); err != nil {
		log.Println("❌ 设置玩家卡牌失败:", err)
		return
	}

	if card.State == entities.CardStateBought {
		playerReserveCards, err := GetPlayerReserveCards(roomID, playerID)
		if err != nil {
			log.Println("❌ 获取玩家保留卡牌失败:", err)
			return
		}
		for i, c := range playerReserveCards {
			if c.ID == card.ID {
				playerReserveCards = append(playerReserveCards[:i], playerReserveCards[i+1:]...)
				break
			}
		}
		if err := SetPlayerReserveCards(roomID, playerID, playerReserveCards); err != nil {
			log.Println("❌ 设置玩家保留卡牌失败:", err)
			return
		}
	} else {
		allCards, err := GetAllNormalCards(roomID)
		if err != nil {
			return
		}
		for _, c := range allCards {
			if c.State == entities.CardStateHidden && c.Level == card.Level {
				c.State = entities.CardStateRevealed
				if err := SetNormalCardByID(roomID, &c); err != nil {
					log.Println("❌ 翻开卡牌失败:", err)
				}
				break
			}
		}
		// 8. 设置该卡牌为已购买
		card.State = entities.CardStateBought
		if err := SetNormalCardByID(roomID, card); err != nil {
			log.Println("❌ 更新卡牌状态失败:", err)
			return
		}
	}

	err = SetLastData(roomID, playerID, "buy_card", card)
	if err != nil {
		log.Println("❌ 设置最后购买的卡牌失败:", err)
		return
	}

	allNobles, _ := GetAllNobleCards(roomID)
	revealedNobles := make([]entities.NobleCard, 0)
	for _, noble := range allNobles {
		if noble.State == entities.CardStateRevealed {
			revealedNobles = append(revealedNobles, noble)
		}
	}

	// 1. 获取玩家已有的折扣卡数量
	playerNobleCards, err := GetPlayerNobleCard(roomID, playerID)
	if err != nil {
		log.Printf("获取玩家贵族卡失败: %v", err)
		return
	}

	// 2. 获取玩家已有的卡牌数量
	playerCards, err = GetPlayerNormalCard(roomID, playerID)
	if err != nil {
		log.Printf("获取玩家卡牌失败: %v", err)
		return
	}

	cardCount = make(map[string]int)
	for _, card := range playerCards {
		cardCount[card.Bonus]++
	}

	// 3. 遍历所有 revealed 的贵族卡，判断是否满足条件
	for _, noble := range revealedNobles {
		satisfy := true
		for color, required := range noble.Cost {
			if cardCount[color] < required {
				satisfy = false
				break
			}
		}

		if satisfy {
			// 4. 满足条件，分配 noble
			noble.State = entities.CardStateBought

			// 写回 noble 状态（假设 noble.ID 是 string，cardID 用字符串即可）
			if err := SetNobleCardByID(roomID, &noble); err != nil {
				log.Printf("⚠️ 设置贵族卡 %s 状态失败: %v", noble.ID, err)
				continue
			}

			// 添加到玩家的 noble 列表中
			playerNobleCards = append(playerNobleCards, noble)
		}
	}

	// 5. 更新玩家的 noble 列表到 Redis
	if err := SetPlayerNobleCard(roomID, playerID, playerNobleCards); err != nil {
		log.Printf("⚠️ 更新玩家贵族卡列表失败: %v", err)
		return
	}

	SwitchToNextPlayer(rdb, repository.Ctx, roomID, currentPlayer)
}

func handleGetGemMessage(conn ReadWriteConn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	currentPlayer, err := GetCurrentPlayer(rdb, repository.Ctx, roomID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return
	}
	if currentPlayer != playerID {
		log.Println("❌ 不是当前玩家的回合")
		return
	}
	// 1. 获取玩家取的宝石数量（从 payload 中解析）
	payload, ok := msgMap["payload"].(map[string]interface{})
	if !ok {
		log.Println("❌ 消息格式错误: payload 解析失败")
		return
	}

	// 转换为 map[string]int
	gemCount := make(map[string]int)
	for color, val := range payload {
		floatVal, ok := val.(float64) // JSON 数字默认解析为 float64
		if !ok {
			log.Printf("❌ 宝石颜色 %s 数量格式错误\n", color)
			return
		}
		gemCount[color] = int(floatVal)
	}

	// 2. 获取玩家当前的宝石
	playerGem, err := GetPlayerGem(roomID, playerID)
	if err != nil {
		log.Println("❌ 获取玩家宝石失败:", err)
		return
	}

	// 3. 获取房间宝石
	allGems, err := GetGemCounts(roomID)
	if err != nil {
		log.Println("❌ 获取宝石数量失败:", err)
		return
	}

	// 4. 更新宝石信息
	for color, num := range gemCount {
		if allGems[color] < num {
			log.Printf("❌ 房间宝石不足: %s 只剩 %d，玩家想拿 %d\n", color, allGems[color], num)
			return
		}
		allGems[color] -= num
		playerGem[color] += num
	}

	// 5. 写回 Redis
	if err := SetGemCounts(roomID, allGems); err != nil {
		log.Println("❌ 更新房间宝石失败:", err)
		return
	}
	if err := SetPlayerGem(roomID, playerID, playerGem); err != nil {
		log.Println("❌ 更新玩家宝石失败:", err)
		return
	}

	err = SetLastData(roomID, playerID, "get_gem", gemCount)
	if err != nil {
		log.Println("❌ 设置最后取的宝石失败:", err)
		return
	}
	SwitchToNextPlayer(rdb, repository.Ctx, roomID, currentPlayer)
}

func handleReserveCardMessage(conn ReadWriteConn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	currentPlayer, err := GetCurrentPlayer(rdb, repository.Ctx, roomID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return
	}
	if currentPlayer != playerID {
		log.Println("❌ 不是当前玩家的回合")
		return
	}

	cardIDFloat, ok := msgMap["payload"].(float64)
	if !ok {
		log.Println("❌ 消息格式错误: payload 不是数字")
		return
	}
	cardID := int(cardIDFloat)

	// 2. 获取卡牌信息
	card, err := GetNormalCardByID(roomID, strconv.Itoa(cardID))
	if err != nil {
		log.Println("❌ 获取卡牌失败:", err)
		return
	}

	allGems, err := GetGemCounts(roomID)
	if err != nil {
		log.Println("❌ 获取宝石数量失败:", err)
		return
	}

	if allGems["Gold"] <= 0 {
		log.Println("❌ 宝石不足")
		return
	}

	allGems["Gold"] -= 1
	if err := SetGemCounts(roomID, allGems); err != nil {
		log.Println("❌ 更新宝石数量失败:", err)
		return
	}
	// 2. 获取玩家当前的保留卡牌
	playerReserveCards, err := GetPlayerReserveCards(roomID, playerID)
	if err != nil {
		log.Println("❌ 获取玩家保留卡牌失败:", err)
		return
	}
	if len(playerReserveCards) >= 3 {
		log.Println("❌ 玩家保留卡牌已满")
		return
	}
	playerGem, err := GetPlayerGem(roomID, playerID)
	if err != nil {
		log.Println("❌ 获取玩家宝石失败:", err)
		return
	}
	playerGem["Gold"] += 1
	err = SetPlayerGem(roomID, playerID, playerGem)
	if err != nil {
		log.Println("❌ 设置玩家宝石失败:", err)
		return
	}

	playerReserveCards = append(playerReserveCards, entities.NormalCard{
		ID:     card.ID,
		Level:  card.Level,
		Bonus:  card.Bonus,
		Points: card.Points,
		Cost:   card.Cost,
		State:  entities.CardStateBought,
	})

	allCards, err := GetAllNormalCards(roomID)
	if err != nil {
		return
	}

	for _, c := range allCards {
		if c.State == entities.CardStateHidden && c.Level == card.Level {
			c.State = entities.CardStateRevealed
			if err := SetNormalCardByID(roomID, &c); err != nil {
				log.Println("❌ 翻开卡牌失败:", err)
			}
			break
		}
	}
	// 8. 设置该卡牌为已购买
	card.State = entities.CardStateBought
	if err := SetNormalCardByID(roomID, card); err != nil {
		log.Println("❌ 更新卡牌状态失败:", err)
		return
	}

	err = SetPlayerReserveCards(roomID, playerID, playerReserveCards)
	if err != nil {
		log.Println("❌ 设置玩家保留卡牌失败:", err)
		return
	}

	err = SetLastData(roomID, playerID, "preserve_card", card)
	if err != nil {
		log.Println("❌ 设置最后购买的卡牌失败:", err)
		return
	}
	SwitchToNextPlayer(rdb, repository.Ctx, roomID, currentPlayer)
}
