package game

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/dto"
	"go-game/repository"
	"go-game/utils"
	"log"
	"math/rand/v2"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

func SwitchToNextPlayer(
	rdb *redis.Client,
	ctx context.Context,
	r *domain.Room,
	currentPlayerID string,
) error {

	if r == nil || len(r.PlayerSeq) == 0 {
		return fmt.Errorf("房间 %s 没有玩家", r.ID)
	}

	// 1️⃣ 找到当前玩家在顺序表中的索引
	currentIdx := -1
	for i, pid := range r.PlayerSeq {
		if pid == currentPlayerID {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return fmt.Errorf("未找到当前玩家 %s", currentPlayerID)
	}

	// 2️⃣ 计算下一个玩家索引（循环）
	nextIdx := (currentIdx + 1) % len(r.PlayerSeq)
	nextPlayerID := r.PlayerSeq[nextIdx]

	// 3️⃣ 校验玩家是否仍存在（防止中途退出）
	if _, ok := r.Players[nextPlayerID]; !ok {
		return fmt.Errorf("下一个玩家 %s 不存在", nextPlayerID)
	}

	// 4️⃣ 设置当前玩家（持久化）
	if err := data.SetCurrentPlayer(rdb, ctx, r.ID, nextPlayerID); err != nil {
		return fmt.Errorf("切换当前玩家失败: %w", err)
	}

	log.Printf("✅ 房间 %s 当前玩家切换为: %s\n", r.ID, nextPlayerID)
	return nil
}

type PlayAudioPayload struct {
	AudioType string `json:"audioType"`
}

func HandlePlayAudioMessage(r *domain.Room, cmd domain.Command) {
	var p PlayAudioPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		log.Println("❌ 消息格式错误:", err)
		return
	}
	audioType := p.AudioType

	msg := map[string]interface{}{
		"type":    "audio",
		"message": audioType,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("❌ 编码 JSON 失败:", err)
		return
	}

	for _, pc := range r.Players {
		if pc.Online && pc.Conn != nil {
			err := pc.Conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				log.Printf("❌ 向玩家 %s 发送音频消息失败: %v\n", pc.PlayerID, err)
			}
		}
	}
}

func HandleRestartGameMessage(r *domain.Room, cmd domain.Command) {
	// 重置上次落子
	if err := data.SetLastTileKey(repository.Rdb, repository.Ctx, r.ID, cmd.PlayerID, ""); err != nil {
		log.Println("❌ 设置最后放置的 tile 失败:", err)
		return
	}
	// 重置游戏状态
	r.Status = dto.RoomStatusMatch
	// 重置tiles
	tile, err := data.GetAllRoomTiles(repository.Rdb, r.ID)
	if err != nil {
		log.Println("❌ 获取所有 tile 失败:", err)
		return
	}
	for tileKey, tileInfo := range tile {
		tileInfo.Belong = ""
		tile[tileKey] = tileInfo
	}
	err = data.SetAllRoomTiles(repository.Rdb, r.ID, tile)
	if err != nil {
		log.Println("❌ 重置 tile 失败:", err)
		return
	}

	for _, pc := range r.Players {
		playerID := pc.PlayerID
		// 2. 设置初始资金
		err = data.SetPlayerInfoField(repository.Rdb, repository.Ctx, r.ID, playerID, "money", 6000)
		if err != nil {
			log.Println("设置玩家信息失败:", err)
		}

		allTiles, err := data.GenerateAvailableTiles(r)
		if err != nil {
			log.Println(err)
		}
		rand.Shuffle(len(allTiles), func(i, j int) { allTiles[i], allTiles[j] = allTiles[j], allTiles[i] })
		playerTiles := utils.SafeSlice(allTiles, 5)
		err = data.SetPlayerTiles(repository.Rdb, repository.Ctx, r.ID, playerID, playerTiles)
		if err != nil {
			log.Println(err)
		}
		companyIDs, err := data.GetCompanyIDs(r.ID)
		if err != nil {
			log.Println("获取公司ID失败:", err)
			return
		}
		// 3.2 初始化玩家股票为0
		playerStocks := make(map[string]int)
		for _, company := range companyIDs {
			playerStocks[company] = 0
		}
		err = data.SetPlayerStocks(repository.Rdb, repository.Ctx, r.ID, playerID, playerStocks)
		if err != nil {
			log.Println("写入玩家股票失败:", err)
		}
	}

	companyData := map[string]map[string]interface{}{
		"Sackson": {
			"name":       "Sackson",
			"stockTotal": 25,
			"tiles":      0,   // 初始数量
			"stockPrice": 200, // 初始参考股价（可调整）
		},
		"Tower": {
			"name":       "Tower",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"American": {
			"name":       "American",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Festival": {
			"name":       "Festival",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Worldwide": {
			"name":       "Worldwide",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Continental": {
			"name":       "Continental",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Imperial": {
			"name":       "Imperial",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
	}

	for id, data := range companyData {
		companyKey := fmt.Sprintf("room:%s:company:%s", r.ID, id)
		if _, err := repository.Rdb.HSet(repository.Ctx, companyKey, data).Result(); err != nil {
			return
		}
		repository.Rdb.SAdd(repository.Ctx, fmt.Sprintf("room:%s:company_ids", r.ID), id)
	}
	startKey := fmt.Sprintf("room:%s:game_start_time", r.ID)
	repository.Rdb.Set(repository.Ctx, startKey, time.Now().Format("20060102_150405"), 0)
}
