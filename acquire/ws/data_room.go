package ws

import (
	"context"
	"fmt"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"log"
	"strconv"

	"github.com/go-redis/redis/v8"
)

// 判断玩家信息是否存在
func IsPlayerInfoExists(rdb *redis.Client, ctx context.Context, roomID, playerID string) (bool, error) {
	playerInfoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	exists, err := rdb.Exists(ctx, playerInfoKey).Result()
	if err != nil {
		return false, fmt.Errorf("检查玩家数据失败: %w", err)
	}
	return exists > 0, nil
}

// GetRoomInfo 获取房间的全部信息（Hash）
func GetRoomInfo(rdb *redis.Client, roomID string) (*entities.RoomInfo, error) {
	roomKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	roomInfoMap, err := rdb.HGetAll(repository.Ctx, roomKey).Result()
	if err != nil {
		return nil, fmt.Errorf("❌ 获取房间信息失败: %w", err)
	}
	if len(roomInfoMap) == 0 {
		return nil, fmt.Errorf("房间信息为空")
	}

	roomInfo := &entities.RoomInfo{}
	startStr := roomInfoMap["roomStatus"]
	roomStatus, err := strconv.ParseBool(startStr)
	if err != nil {
		return nil, fmt.Errorf("roomStatus 字段解析失败: %w", err)
	}
	roomInfo.RoomStatus = roomStatus
	roomInfo.GameStatus = dto.RoomStatus(roomInfoMap["gameStatus"])
	roomInfo.UserID = roomInfoMap["userID"]
	// 字符串转 int
	maxPlayersStr := roomInfoMap["maxPlayers"]
	if maxPlayersStr != "" {
		if val, err := strconv.Atoi(maxPlayersStr); err == nil {
			roomInfo.MaxPlayers = val
		} else {
			log.Printf("⚠️ maxPlayers 转换失败: %v\n", err)
		}
	}

	return roomInfo, nil
}

// SetRoomInfo 设置房间的全部信息（Hash）
func SetRoomInfo(rdb *redis.Client, ctx context.Context, roomID string, info entities.RoomInfo) error {
	roomKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	roomStatus := strconv.FormatBool(info.RoomStatus)

	data := map[string]interface{}{
		"gameStatus": string(info.GameStatus),
		"roomStatus": roomStatus,
		"maxPlayers": strconv.Itoa(info.MaxPlayers),
		"userID":     info.UserID,
	}

	if err := rdb.HSet(ctx, roomKey, data).Err(); err != nil {
		return fmt.Errorf("❌ 设置房间信息失败: %w", err)
	}

	return nil
}

func SetGameStatus(rdb *redis.Client, roomID string, status dto.RoomStatus) error {
	roomInfoKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	err := rdb.HSet(repository.Ctx, roomInfoKey, "gameStatus", string(status)).Err()
	if err != nil {
		log.Printf("更新房间状态失败（roomID: %s，gameStatus: %s）: %v\n", roomID, status, err)
		return err
	}
	log.Printf("房间（roomID: %s）状态已更新为：%s\n", roomID, status)
	return nil
}

func SetRoomStatus(rdb *redis.Client, roomID string, status bool) error {
	roomInfoKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	statusStr := strconv.FormatBool(status) // 将 bool 转为字符串 "true"/"false"

	err := rdb.HSet(repository.Ctx, roomInfoKey, "roomStatus", statusStr).Err()
	if err != nil {
		return fmt.Errorf("更新房间状态失败: %w", err)
	}
	return nil
}

// SetCurrentPlayer 设置当前玩家
func SetCurrentPlayer(rdb *redis.Client, ctx context.Context, roomID, playerID string) error {
	key := fmt.Sprintf("room:%s:currentPlayer", roomID)
	if err := rdb.Set(ctx, key, playerID, 0).Err(); err != nil {
		return fmt.Errorf("设置当前玩家失败: %w", err)
	}
	log.Printf("✅ 当前玩家已设置: roomID=%s playerID=%s\n", roomID, playerID)
	return nil
}

// GetCurrentPlayer 获取当前玩家
func GetCurrentPlayer(rdb *redis.Client, ctx context.Context, roomID string) (string, error) {
	key := fmt.Sprintf("room:%s:currentPlayer", roomID)
	playerID, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", fmt.Errorf("获取当前玩家失败: %w", err)
	}
	return playerID, nil
}
