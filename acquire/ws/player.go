package ws

import (
	"encoding/json"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/room"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"log"

	"github.com/go-redis/redis/v8"
)

func HandleMatchBegin(conn room.ReadWriteConn, rdb *redis.Client, currentRoom *dto.Room, playerID string, msgMap map[string]interface{}) {
	ctx := repository.Ctx

	// 检查是否所有玩家都已准备
	allReady := true
	for _, pc := range currentRoom.Players {
		if !pc.Ready {
			allReady = false
			break
		}
	}

	if !allReady {
		log.Println("❌ 不是所有玩家都已准备")
		return
	}

	// 初始化房间信息
	err := data.SetRoomInfo(rdb, repository.Ctx, currentRoom.ID, entities.RoomInfo{
		RoomStatus: false,
		GameStatus: dto.RoomStatusWaiting,
		MaxPlayers: len(currentRoom.Players),
		UserID:     currentRoom.OwnerID,
	})
	if err != nil {
		log.Printf("❌ 初始化房间信息失败: %v\n", err)
		return
	}

	// 初始化公司数据
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
		companyKey := fmt.Sprintf("room:%s:company:%s", currentRoom.ID, id)
		if _, err := rdb.HSet(ctx, companyKey, data).Result(); err != nil {
			log.Printf("❌ 初始化公司[%s]失败: %v\n", id, err)
			return
		}
		rdb.SAdd(ctx, fmt.Sprintf("room:%s:company_ids", currentRoom.ID), id)
	}

	// 初始化游戏棋盘（12x9 个 tile）
	tileKey := fmt.Sprintf("room:%s:tiles", currentRoom.ID)
	pipe := rdb.Pipeline()

	for col := 1; col <= 12; col++ {
		for row := 'A'; row <= 'I'; row++ {
			id := fmt.Sprintf("%d%c", col, row)
			tile := dto.Tile{
				ID:     id,
				Belong: "",
			}
			tileJSON, err := json.Marshal(tile)
			if err != nil {
				log.Printf("❌ tile %s 序列化失败: %v\n", id, err)
				return
			}
			pipe.HSet(ctx, tileKey, id, tileJSON)
		}
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		log.Printf("❌ tile 初始化 Redis 写入失败: %v\n", err)
		return
	}

	for i := 1; i <= len(currentRoom.Players); i++ {
		if currentRoom.Players[i-1].AI {
			JoinRoomAsAI(currentRoom.ID, fmt.Sprintf("ai_%03d", i))
		} else {
			continue
		}
	}

	// 更新房间状态为匹配中
	err = room.SetRoomStatusCache(currentRoom.ID, dto.RoomStatusWaiting)
	if err != nil {
		log.Printf("❌ 内存设置房间状态失败: %v\n", err)
		return
	}
	err = data.SetGameStatus(rdb, currentRoom.ID, dto.RoomStatusWaiting)
	if err != nil {
		log.Printf("❌ redis设置游戏状态失败: %v\n", err)
		return
	}

	// 开始游戏
	log.Println("所有玩家都已准备，开始游戏")
}

func HandleAddAI(conn room.ReadWriteConn, rdb *redis.Client, currentRoom *dto.Room, playerID string, msgMap map[string]interface{}) {
	// 检查房间是否已满
	if len(currentRoom.Players) >= MaxPlayers {
		log.Println("❌ 房间已满，无法添加 AI")
		return
	}

	// 检查房间状态是否为等待加入
	if currentRoom.Status != dto.RoomStatusMatch {
		log.Println("❌ 房间状态不是等待加入，无法添加 AI")
		return
	}

	// 加入 AI 玩家
	JoinMatchAsAI(currentRoom.ID, fmt.Sprintf("ai_%03d", len(currentRoom.Players)+1))
}

func HandleRemovePlayer(conn room.ReadWriteConn, rdb *redis.Client, currentRoom *dto.Room, playerID string, msgMap map[string]interface{}) {
	removePlayerID, ok := msgMap["payload"].(string)
	if !ok {
		log.Println("无效的 payload")
		return
	}

	// 检查房间状态是否为等待加入
	if currentRoom.Status != dto.RoomStatusMatch {
		log.Println("❌ 房间状态不是等待加入，无法移除玩家")
		return
	}

	// 移除玩家
	RemovePlayer(currentRoom.ID, removePlayerID)
}
