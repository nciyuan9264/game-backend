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

func GetRandomNobles(max int) []entities.NobleCard {
	nobleList := make([]entities.NobleCard, len(const_data.NobleTilesList))
	copy(nobleList, const_data.NobleTilesList)

	// 打乱
	rand.Shuffle(len(nobleList), func(i, j int) {
		nobleList[i], nobleList[j] = nobleList[j], nobleList[i]
	})

	// 设置状态
	for i := range nobleList {
		if i < max {
			nobleList[i].State = entities.CardStateRevealed
		}
	}

	return nobleList
}
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
	randomNobles := GetRandomNobles(roomInfo.MaxPlayers + 1)

	pipe = repository.Rdb.Pipeline()
	for _, noble := range randomNobles {
		nobleJSON, err := json.Marshal(noble)
		if err != nil {
			return fmt.Errorf("贵族瓷砖序列化失败: %w", err)
		}
		pipe.HSet(repository.Ctx, noblesKey, noble.ID, nobleJSON)
	}
	_, err = pipe.Exec(repository.Ctx)
	if err != nil {
		log.Println("❌ pipeline 写入 nobles 失败:", err)
	}

	if _, err := pipe.Exec(repository.Ctx); err != nil {
		return fmt.Errorf("初始化贵族瓷砖失败: %w", err)
	}
	return nil
}
func InitPlayerDataToRedis(roomID, playerID string) error {
	// 初始化卡牌
	initNormalCard := []entities.NormalCard{}
	if err := SetPlayerNormalCard(roomID, playerID, initNormalCard); err != nil {
		log.Fatal(err)
	}

	initNobleCard := []entities.NobleCard{}
	if err := SetPlayerNobleCard(roomID, playerID, initNobleCard); err != nil {
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
	if err := SetPlayerReserveCards(roomID, playerID, reserveCard); err != nil {
		log.Println("设置预定卡牌失败:", err)
	}

	return nil
}
