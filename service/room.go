package service

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"strconv"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

func GetTile(rdb *redis.Client, roomID, tileID string) (*dto.Tile, error) {
	ctx := repository.Ctx
	key := fmt.Sprintf("room:%s:tiles", roomID)
	result, err := rdb.HGet(ctx, key, tileID).Result()
	if err != nil {
		return nil, err
	}
	var tile dto.Tile
	if err := json.Unmarshal([]byte(result), &tile); err != nil {
		return nil, err
	}
	return &tile, nil
}

func CreateRoom(maxPlayers int) (string, error) {
	ctx := repository.Ctx
	rdb := repository.Rdb

	// 生成唯一 Room ID（例如 8位）
	uuidStr := uuid.New().String()
	roomID := strings.ReplaceAll(uuidStr, "-", "")[:8]

	roomKey := fmt.Sprintf("room:%s:roomInfo", roomID)

	// 初始化房间信息
	_, err := rdb.HSet(ctx, roomKey, map[string]interface{}{
		"maxPlayers": maxPlayers,
		"status":     "waiting",
	}).Result()
	if err != nil {
		return "", fmt.Errorf("初始化房间信息失败: %w", err)
	}

	companyData := map[string]map[string]interface{}{
		"Sackson": {
			"name":       "Sackson",
			"stockTotal": 25,
			"tiles":      0,    // 初始数量
			"stockPrice": 200,  // 初始参考股价（可调整）
			"valuation":  5000, // 初始估值
		},
		"Tower": {
			"name":       "Tower",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
			"valuation":  5000,
		},
		"American": {
			"name":       "American",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
			"valuation":  5000,
		},
		"Festival": {
			"name":       "Festival",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
			"valuation":  5000,
		},
		"Worldwide": {
			"name":       "Worldwide",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
			"valuation":  5000,
		},
		"Continental": {
			"name":       "Continental",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
			"valuation":  5000,
		},
		"Imperial": {
			"name":       "Imperial",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
			"valuation":  5000,
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

	_, err = pipe.Exec(ctx)
	if err != nil {
		return "", fmt.Errorf("tile 初始化 Redis 写入失败: %w", err)
	}

	// 2. 初始化当前操作玩家（第一个玩家开始）
	currentPlayerKey := fmt.Sprintf("room:%s:currentPlayer", roomID)
	if err := rdb.Set(ctx, currentPlayerKey, "", 0).Err(); err != nil {
		return "", err
	}

	// 3. 初始化当前玩家操作步数（从0开始）
	currentStepKey := fmt.Sprintf("room:%s:currentStep", roomID)
	if err := rdb.Set(ctx, currentStepKey, 0, 0).Err(); err != nil {
		return "", err
	}

	return roomID, nil
}

func GetRoomList() ([]dto.RoomInfo, error) {
	ctx := repository.Ctx
	rdb := repository.Rdb

	// 获取所有房间 key（匹配 room:*:info）
	keys, err := rdb.Keys(ctx, "room:*:roomInfo").Result()
	if err != nil {
		return nil, err
	}

	var rooms []dto.RoomInfo

	for _, key := range keys {
		// 获取 roomId（从 key 中提取）
		// key 格式：room:<roomID>:info
		parts := strings.Split(key, ":")
		if len(parts) < 3 {
			continue
		}
		roomID := parts[1]

		// 获取 room 的 hash 数据
		data, err := rdb.HGetAll(ctx, key).Result()
		if err != nil {
			continue
		}

		maxPlayers, _ := strconv.Atoi(data["maxPlayers"])
		status := data["status"]

		room := dto.RoomInfo{
			RoomID:     roomID,
			MaxPlayers: maxPlayers,
			Status:     status,
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

// func JoinRoom(roomID, playerID string) error {
// 	// 1. 检查房间状态，比如是否已满
// 	if IsRoomFull(roomID) {
// 		return errors.New("房间已满")
// 	}
// 	// 2. 将玩家加入房间
// 	err := AddPlayerToRoom(roomID, playerID)
// 	if err != nil {
// 		return err
// 	}
// 	// 3. 其他业务逻辑，比如恢复玩家数据等
// 	return nil
// }

// func AddPlayerToRoom(roomID, playerID string) any {
// 	panic("unimplemented")
// }
