package ws

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"go-game/utils"
	"log"
	"math/rand/v2"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

func handlePlayAudioMessage(conn *websocket.Conn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
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

	for _, pc := range Rooms[roomID] {
		if pc.Online && pc.Conn != nil {
			err := pc.Conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				log.Printf("❌ 向玩家 %s 发送音频消息失败: %v\n", pc.PlayerID, err)
			}
		}
	}
}

func handleRestartGameMessage(conn *websocket.Conn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	// 重置上次落子
	if err := SetLastTileKey(rdb, repository.Ctx, roomID, playerID, ""); err != nil {
		log.Println("❌ 设置最后放置的 tile 失败:", err)
		return
	}
	// 重置游戏状态
	SetGameStatus(rdb, roomID, dto.RoomStatusSetTile)
	// 重置tiles
	tile, err := GetAllRoomTiles(rdb, roomID)
	if err != nil {
		log.Println("❌ 获取所有 tile 失败:", err)
		return
	}
	for tileKey, tileInfo := range tile {
		tileInfo.Belong = ""
		tile[tileKey] = tileInfo
	}
	err = SetAllRoomTiles(rdb, roomID, tile)
	if err != nil {
		log.Println("❌ 重置 tile 失败:", err)
		return
	}

	for _, pc := range Rooms[roomID] {
		playerID := pc.PlayerID
		// 2. 设置初始资金
		err = SetPlayerInfoField(repository.Rdb, repository.Ctx, roomID, playerID, "money", 6000)
		if err != nil {
			log.Println("设置玩家信息失败:", err)
		}

		allTiles, err := generateAvailableTiles(roomID)
		if err != nil {
			log.Println(err)
		}
		rand.Shuffle(len(allTiles), func(i, j int) { allTiles[i], allTiles[j] = allTiles[j], allTiles[i] })
		playerTiles := utils.SafeSlice(allTiles, 5)
		err = SetPlayerTiles(repository.Rdb, repository.Ctx, roomID, playerID, playerTiles)
		if err != nil {
			log.Println(err)
		}
		companyIDs, err := getCompanyIDs(roomID)
		if err != nil {
			log.Println("获取公司ID失败:", err)
			return
		}
		// 3.2 初始化玩家股票为0
		playerStocks := make(map[string]int)
		for _, company := range companyIDs {
			playerStocks[company] = 0
		}
		err = SetPlayerStocks(repository.Rdb, repository.Ctx, roomID, playerID, playerStocks)
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
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, id)
		if _, err := rdb.HSet(repository.Ctx, companyKey, data).Result(); err != nil {
			return
		}
		rdb.SAdd(repository.Ctx, fmt.Sprintf("room:%s:company_ids", roomID), id)
	}

	broadcastToRoom(roomID)
}
