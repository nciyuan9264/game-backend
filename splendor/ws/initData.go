package ws

import (
	"encoding/json"
	"fmt"
	"go-game/const_data"
	"go-game/entities"
	"go-game/repository"
	"log"

	"golang.org/x/exp/rand"
)

func InitRoomData(roomID string) error {
	roomInfo, err := GetRoomInfo(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %w", err)
	}
	// 初始化卡牌信息
	cardKey := fmt.Sprintf("room:%s:card", roomID)
	pipe := repository.Rdb.Pipeline()

	for _, cards := range const_data.SplendorCards {
		shuffled := rand.Perm(len(cards))
		for idx, rnd := range shuffled {
			card := cards[rnd]
			if idx < 4 {
				card.State = entities.CardStateRevealed
			} else {
				card.State = entities.CardStateHidden
			}

			cardJSON, err := json.Marshal(card)
			if err != nil {
				return fmt.Errorf("卡牌序列化失败: %w", err)
			}
			pipe.HSet(repository.Ctx, cardKey, card.ID, cardJSON)
		}
	}

	if _, err := pipe.Exec(repository.Ctx); err != nil {
		return fmt.Errorf("初始化卡牌失败: %w", err)
	}

	// 初始化宝石 token 池
	gemCounts := map[string]int{
		"Blue":  7,
		"Green": 7,
		"Red":   7,
		"White": 7,
		"Black": 7,
		"Gold":  5,
	}
	gemKey := fmt.Sprintf("room:%s:gems", roomID)
	for color, cnt := range gemCounts {
		if _, err := repository.Rdb.HSet(repository.Ctx, gemKey, color, cnt).Result(); err != nil {
			return fmt.Errorf("初始化宝石[%s]失败: %w", color, err)
		}
	}
	noblesKey := fmt.Sprintf("room:%s:nobles", roomID)
	randomNobleTilesList := rand.Perm(len(const_data.NobleTilesList))
	pipe = repository.Rdb.Pipeline()

	for idx, nobleIdx := range randomNobleTilesList {
		noble := const_data.NobleTilesList[nobleIdx] // ✅ 获取实际 NobleCard

		if idx < roomInfo.MaxPlayers+1 {
			noble.State = entities.CardStateRevealed // ✅ 设置为可展示
		}

		nobleJSON, err := json.Marshal(noble) // ✅ 放在设置 State 后面
		if err != nil {
			return fmt.Errorf("贵族瓷砖序列化失败: %w", err)
		}

		pipe.HSet(repository.Ctx, noblesKey, noble.ID, nobleJSON) // ✅ pipeline 写入
	}

	if _, err := pipe.Exec(repository.Ctx); err != nil {
		return fmt.Errorf("初始化贵族瓷砖失败: %w", err)
	}
	return nil
}

func InitPlayerDataToRedis(roomID, playerID string) error {
	// 初始化卡牌
	initCard := map[string]int{
		"Red":   0,
		"Green": 0,
		"White": 0,
		"Blue":  0,
		"Black": 0,
		"Gold":  0,
	}
	if err := SetPlayerCard(roomID, playerID, initCard); err != nil {
		log.Fatal(err)
	}

	if err := SetPlayerScore(roomID, playerID, 0); err != nil {
		log.Println("设置分数失败:", err)
	}

	// 初始化宝石
	initGem := map[string]int{
		"Red":   0,
		"Green": 0,
		"White": 0,
		"Blue":  0,
		"Black": 0,
		"Gold":  0,
	}
	if err := SetPlayerGem(roomID, playerID, initGem); err != nil {
		log.Println("设置宝石失败:", err)
	}

	// 初始化预留卡牌
	reserveCard := []entities.NormalCard{}
	if err := SetPlayerReserveCards("room123", "player1", reserveCard); err != nil {
		log.Println("设置预定卡牌失败:", err)
	}

	return nil
}
