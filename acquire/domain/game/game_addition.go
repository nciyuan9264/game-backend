package game

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/room"
	"go-game/dto"
	"go-game/repository"
	"go-game/utils"
	"log"
	"math/rand/v2"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

func SwitchToNextPlayer(rdb *redis.Client, ctx context.Context, roomID, currentID string) error {
	room.RoomLock.Lock()
	defer room.RoomLock.Unlock()

	room := room.Rooms[roomID]
	if room == nil || len(room.Players) == 0 {
		return fmt.Errorf("房间 %s 没有玩家", roomID)
	}

	// 找到当前玩家索引
	var currentIndex int = -1
	for i, pc := range room.Players {
		if pc.PlayerID == currentID {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		return fmt.Errorf("未找到当前玩家 %s", currentID)
	}

	// 下一个玩家索引（循环）
	nextIndex := (currentIndex + 1) % len(room.Players)
	nextPlayerID := room.Players[nextIndex].PlayerID

	// 设置当前玩家
	if err := data.SetCurrentPlayer(rdb, ctx, roomID, nextPlayerID); err != nil {
		return fmt.Errorf("切换当前玩家失败: %w", err)
	}

	log.Printf("✅ 已将当前玩家切换为: %s\n", nextPlayerID)
	return nil
}

func HandlePlayAudioMessage(conn room.ReadWriteConn, rdb *redis.Client, room *dto.Room, playerID string, msgMap map[string]interface{}) {
	audioType, ok := msgMap["payload"].(string)
	if !ok {
		log.Println("❌ 消息格式错误")
		return
	}

	msg := map[string]interface{}{
		"type":    "audio",
		"message": audioType,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("❌ 编码 JSON 失败:", err)
		return
	}

	for _, pc := range room.Players {
		if pc.Online && pc.Conn != nil {
			err := pc.Conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				log.Printf("❌ 向玩家 %s 发送音频消息失败: %v\n", pc.PlayerID, err)
			}
		}
	}
}

func HandleRestartGameMessage(conn room.ReadWriteConn, rdb *redis.Client, room *dto.Room, playerID string, msgMap map[string]interface{}) {
	// 重置上次落子
	if err := data.SetLastTileKey(rdb, repository.Ctx, room.ID, playerID, ""); err != nil {
		log.Println("❌ 设置最后放置的 tile 失败:", err)
		return
	}
	// 重置游戏状态
	room.Status = dto.RoomStatusSetTile
	// 重置tiles
	tile, err := data.GetAllRoomTiles(rdb, room.ID)
	if err != nil {
		log.Println("❌ 获取所有 tile 失败:", err)
		return
	}
	for tileKey, tileInfo := range tile {
		tileInfo.Belong = ""
		tile[tileKey] = tileInfo
	}
	err = data.SetAllRoomTiles(rdb, room.ID, tile)
	if err != nil {
		log.Println("❌ 重置 tile 失败:", err)
		return
	}

	for _, pc := range room.Players {
		playerID := pc.PlayerID
		// 2. 设置初始资金
		err = data.SetPlayerInfoField(repository.Rdb, repository.Ctx, room.ID, playerID, "money", 6000)
		if err != nil {
			log.Println("设置玩家信息失败:", err)
		}

		allTiles, err := data.GenerateAvailableTiles(room)
		if err != nil {
			log.Println(err)
		}
		rand.Shuffle(len(allTiles), func(i, j int) { allTiles[i], allTiles[j] = allTiles[j], allTiles[i] })
		playerTiles := utils.SafeSlice(allTiles, 5)
		err = data.SetPlayerTiles(repository.Rdb, repository.Ctx, room.ID, playerID, playerTiles)
		if err != nil {
			log.Println(err)
		}
		companyIDs, err := data.GetCompanyIDs(room.ID)
		if err != nil {
			log.Println("获取公司ID失败:", err)
			return
		}
		// 3.2 初始化玩家股票为0
		playerStocks := make(map[string]int)
		for _, company := range companyIDs {
			playerStocks[company] = 0
		}
		err = data.SetPlayerStocks(repository.Rdb, repository.Ctx, room.ID, playerID, playerStocks)
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
		companyKey := fmt.Sprintf("room:%s:company:%s", room.ID, id)
		if _, err := rdb.HSet(repository.Ctx, companyKey, data).Result(); err != nil {
			return
		}
		rdb.SAdd(repository.Ctx, fmt.Sprintf("room:%s:company_ids", room.ID), id)
	}
	startKey := fmt.Sprintf("room:%s:game_start_time", room.ID)
	repository.Rdb.Set(repository.Ctx, startKey, time.Now().Format("20060102_150405"), 0)

	time.Sleep(2 * time.Second)

	BroadcastToRoom(room.ID)
}
