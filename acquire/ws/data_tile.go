package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"

	"github.com/go-redis/redis/v8"
)

// GetTileFromRedis 获取指定房间的某个 tile 信息
func GetTileFromRedis(rdb *redis.Client, ctx context.Context, roomID, tileKey string) (dto.Tile, error) {
	redisKey := fmt.Sprintf("room:%s:tiles", roomID)
	tileData, err := rdb.HGet(ctx, redisKey, tileKey).Result()
	if err == redis.Nil {
		return dto.Tile{}, fmt.Errorf("🚫 Tile 不存在: %s\n", tileKey)
	} else if err != nil {
		return dto.Tile{}, fmt.Errorf("❌ Redis 获取 tile 失败: %v\n", err)
	}

	// 解析为结构体
	var tile dto.Tile
	if err := json.Unmarshal([]byte(tileData), &tile); err != nil {
		return dto.Tile{}, fmt.Errorf("❌ 解析 Tile JSON 失败:", err)
	}
	return tile, nil
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

// 获取房间所有 tile 信息（key 为 tileID，value 为 Tile struct）
func GetAllRoomTiles(rdb *redis.Client, roomID string) (map[string]dto.Tile, error) {
	tileMap := make(map[string]dto.Tile)

	// Redis Hash Key
	key := fmt.Sprintf("room:%s:tiles", roomID)

	// 获取 Redis Hash 所有字段
	roomTiles, err := rdb.HGetAll(repository.Ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("获取房间牌堆失败: %w", err)
	}

	// 解码每个 tile 的 JSON 字符串
	for tileID, value := range roomTiles {
		var tileInfo dto.Tile
		if err := json.Unmarshal([]byte(value), &tileInfo); err != nil {
			continue // 无效数据直接跳过
		}
		tileMap[tileID] = tileInfo
	}

	return tileMap, nil
}

func SetAllRoomTiles(rdb *redis.Client, roomID string, tiles map[string]dto.Tile) error {
	// 构建 Redis Hash 数据
	hashData := make(map[string]interface{})
	for tileID, tile := range tiles {
		tileJSON, err := json.Marshal(tile)
		if err != nil {
			return fmt.Errorf("tile JSON 编码失败: %w", err)
		}
		hashData[tileID] = tileJSON
	}
	// Redis Hash Key
	key := fmt.Sprintf("room:%s:tiles", roomID)
	// 写入 Redis Hash
	if err := rdb.HSet(repository.Ctx, key, hashData).Err(); err != nil {
		return fmt.Errorf("设置房间牌堆失败: %w", err)
	}
	return nil
}
