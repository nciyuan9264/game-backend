package ws

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"

	"github.com/go-redis/redis/v8"
)

// GetRoomInfo 获取房间的全部信息（Hash）
func GetRoomInfo(rdb *redis.Client, roomID string) (map[string]string, error) {
	roomKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	roomInfo, err := rdb.HGetAll(repository.Ctx, roomKey).Result()
	if err != nil {
		return nil, fmt.Errorf("❌ 获取房间信息失败:", err)
	}
	if len(roomInfo) == 0 {
		return nil, fmt.Errorf("房间信息为空")
	}
	return roomInfo, nil
}

// UpdateTileValue 用于将某个 tile 对象整体写入 Redis（覆盖旧值）
func UpdateTileValue(rdb *redis.Client, roomID string, tileKey string, updatedTile dto.Tile) error {
	// 编码为 JSON 字符串
	updatedTileBytes, err := json.Marshal(updatedTile)
	if err != nil {
		return fmt.Errorf("tile JSON 编码失败: %w", err)
	}

	// 写入 Redis Hash
	tileHashKey := fmt.Sprintf("room:%s:tiles", roomID)
	if err := rdb.HSet(repository.Ctx, tileHashKey, tileKey, updatedTileBytes).Err(); err != nil {
		return fmt.Errorf("更新 Redis 中的 tile 失败: %w", err)
	}

	return nil
}
