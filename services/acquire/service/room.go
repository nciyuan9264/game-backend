package service

import (
	"acquire-service/client"
	"acquire-service/dto"
	"acquire-service/entities"
	_ "acquire-service/proto"
	roomproto "acquire-service/proto" // 导入生成的 proto 包
	"acquire-service/repository"
	"acquire-service/ws"
	"context"
	"encoding/json"
	"fmt"
	"log"
)

func InitRoom(roomID string, aiCount int) (string, error) {
	ctx := repository.Ctx
	rdb := repository.Rdb

	// // 简洁的时间前缀：月日_时分秒
	// timePrefix := time.Now().Format("0102_150405")
	// // 生成 4 位随机码
	// randomSuffix := RandString(4)
	// // roomID 示例：0620_153045_dA9X
	// roomID := fmt.Sprintf("%s_%s", timePrefix, randomSuffix)

	// // 初始化房间信息
	// err := ws.SetRoomInfo(rdb, repository.Ctx, roomID, entities.RoomInfo{
	// 	MaxPlayers: params.MaxPlayers,
	// 	GameStatus: dto.RoomStatusSetTile,
	// 	RoomStatus: false,
	// 	UserID:     params.UserID,
	// })
	// if err != nil {
	// 	return "", fmt.Errorf("初始化房间信息失败: %w", err)
	// }

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
		if _, err := rdb.HSet(ctx, companyKey, data).Result(); err != nil {
			return "", fmt.Errorf("初始化公司[%s]失败: %w", id, err)
		}
		rdb.SAdd(ctx, fmt.Sprintf("room:%s:company_ids", roomID), id)
	}

	tileKey := fmt.Sprintf("room:%s:tiles", roomID)
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
				return "", fmt.Errorf("tile %s 序列化失败: %w", id, err)
			}
			pipe.HSet(ctx, tileKey, id, tileJSON)
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return "", fmt.Errorf("tile 初始化 Redis 写入失败: %w", err)
	}
	Rooms[roomID] = []dto.PlayerConn{}

	for i := 1; i <= aiCount; i++ {
		JoinRoomAsAI(roomID, fmt.Sprintf("ai_%03d", i))
	}

	// if params.AiCount > 0 {
	// 	ws.JoinRoomAsAI(roomID, "ai_001")
	// ws.JoinRoomAsAI(roomID, "ai_002")
	// err := ws.SetCurrentPlayer(repository.Rdb, repository.Ctx, roomID, "ai_001")
	// if err != nil {
	// 	log.Println("❌ 设置当前玩家失败:", err)
	// }
	// err = ws.SetRoomStatus(repository.Rdb, roomID, true)
	// if err != nil {
	// 	log.Println("❌ 设置房间状态失败:", err)
	// }
	// ws.BroadcastToRoom(roomID)
	// }
	return roomID, nil
}

func GetRoomList(gameType string) ([]*roomproto.RoomInfo, error) {
	log.Printf("[DEBUG] GetRoomList 开始执行，gameType: %s", gameType)
	ctx := context.Background()

	// 使用 RoomClient 的 ListRooms 方法，而不是直接调用 gRPC 客户端
	resp, err := client.RoomServiceClient.ListRooms(ctx, gameType)
	if err != nil {
		log.Printf("[ERROR] 获取房间列表失败: %v", err)
		return nil, err
	}
	log.Printf("[SUCCESS] 获取房间列表成功，房间数量: %d", len(resp.Rooms))
	log.Printf("[DATA] 房间详情: %+v", resp.Rooms)

	// 直接返回 resp.Rooms，因为它已经是 []*roomproto.RoomInfo 类型
	return resp.Rooms, nil
}

func convertPlayers(protoPlayers []*roomproto.RoomPlayer) []*roomproto.RoomPlayer {
	// 直接返回原始的指针切片，因为类型已经匹配
	return protoPlayers
}

// GetRoomListFromHub 从Hub获取房间列表
func GetRoomListFromHub(hub *ws.Hub) ([]*roomproto.RoomInfo, error) {
	if hub == nil {
		return nil, fmt.Errorf("Hub未初始化")
	}

	// 从Hub获取所有房间
	rooms := hub.GetAllRooms()
	var roomInfos []*roomproto.RoomInfo

	for roomID, room := range rooms {
		// 获取房间的客户端信息
		clients := hub.GetRoomClients(roomID)

		// 构造玩家列表
		var players []*roomproto.RoomPlayer
		for playerID := range clients {
			players = append(players, &roomproto.RoomPlayer{
				PlayerId: playerID,
				Online:   true,
			})
		}

		// 从Redis获取房间详细信息
		roomInfo, err := GetRoomInfo(repository.Rdb, repository.Ctx, roomID)
		if err != nil {
			log.Printf("获取房间[%s]信息失败: %v", roomID, err)
			roomInfo = &entities.RoomInfo{
				MaxPlayers: 6,
				GameStatus: dto.RoomStatusWaiting,
				RoomStatus: true,
				UserID:     "",
			}
		}

		protoRoomInfo := &roomproto.RoomInfo{
			RoomId:     roomID,
			UserId:     roomInfo.UserID,
			MaxPlayers: int32(roomInfo.MaxPlayers),
			Status:     string(roomInfo.GameStatus),
			Players:    players,
		}

		roomInfos = append(roomInfos, protoRoomInfo)
	}

	return roomInfos, nil
}
