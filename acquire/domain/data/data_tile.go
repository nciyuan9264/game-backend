package data

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/domain/domain"
	"go-game/repository"
	"go-game/utils"
	"strconv"

	"github.com/go-redis/redis/v8"
)

// GetConnectedTiles 用于从 tileKey 开始，递归查找相邻、归属一致的 tile
func GetConnectedTiles(rdb *redis.Client, roomID, startTileKey string) []string {
	visited := make(map[string]bool)
	queue := []string{startTileKey}
	var connected []string

	startTile, err := GetTileFromRedis(rdb, repository.Ctx, roomID, startTileKey)
	if err != nil {
		utils.Error("无法获取起始 tile", utils.F("room_id", roomID), utils.F("tile_key", startTileKey), utils.F("error", err))
		return connected
	}
	startTileOwner := startTile.Belong

	for len(queue) > 0 {
		tile := queue[0]
		queue = queue[1:]

		if visited[tile] {
			continue
		}
		visited[tile] = true
		connected = append(connected, tile)

		neighbors := GetAdjacentTileKeys(tile)
		for _, neighbor := range neighbors {
			if visited[neighbor] {
				continue
			}
			tile, err := GetTileFromRedis(rdb, repository.Ctx, roomID, neighbor)
			belong := tile.Belong
			if err == nil && belong == startTileOwner {
				queue = append(queue, neighbor)
			}
		}
	}

	return connected
}

// getAdjacentTileKeys 用于获取指定 tileKey 的上下左右邻接的 tileKey 列表
func GetAdjacentTileKeys(tileKey string) []string {
	row := tileKey[:len(tileKey)-1] // 例如 "6"
	col := tileKey[len(tileKey)-1:] // 例如 "A"

	// 上下左右邻接逻辑
	rowNum, err := strconv.Atoi(row)
	if err != nil {
		return nil
	}

	var adjacent []string

	// 上 (row-1)
	if rowNum > 1 {
		adjacent = append(adjacent, fmt.Sprintf("%d%s", rowNum-1, col))
	}
	// 下 (row+1)
	if rowNum < 12 {
		adjacent = append(adjacent, fmt.Sprintf("%d%s", rowNum+1, col))
	}
	// 左 (col-1)
	if col[0] > 'A' {
		adjacent = append(adjacent, fmt.Sprintf("%d%s", rowNum, string(col[0]-1)))
	}
	// 右 (col+1)
	if col[0] < 'I' {
		adjacent = append(adjacent, fmt.Sprintf("%d%s", rowNum, string(col[0]+1)))
	}

	return adjacent
}

// GetTileFromRedis 获取指定房间的某个 tile 信息
func GetTileFromRedis(rdb *redis.Client, ctx context.Context, roomID, tileKey string) (domain.Tile, error) {
	redisKey := fmt.Sprintf("room:%s:tiles", roomID)
	tileData, err := rdb.HGet(ctx, redisKey, tileKey).Result()
	if err == redis.Nil {
		return domain.Tile{}, fmt.Errorf("🚫 Tile 不存在: %s\n", tileKey)
	} else if err != nil {
		return domain.Tile{}, fmt.Errorf("❌ Redis 获取 tile 失败: %v\n", err)
	}

	// 解析为结构体
	var tile domain.Tile
	if err := json.Unmarshal([]byte(tileData), &tile); err != nil {
		return domain.Tile{}, fmt.Errorf("❌ 解析 Tile JSON 失败:", err)
	}
	return tile, nil
}

// UpdateTileValue 用于将某个 tile 对象整体写入 Redis（覆盖旧值）
func UpdateTileValue(rdb *redis.Client, roomID string, tileKey string, updatedTile domain.Tile) error {
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
func GetAllRoomTiles(rdb *redis.Client, roomID string) (map[string]domain.Tile, error) {
	tileMap := make(map[string]domain.Tile)

	// Redis Hash Key
	key := fmt.Sprintf("room:%s:tiles", roomID)

	// 获取 Redis Hash 所有字段
	roomTiles, err := rdb.HGetAll(repository.Ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("获取房间牌堆失败: %w", err)
	}

	// 解码每个 tile 的 JSON 字符串
	for tileID, value := range roomTiles {
		var tileInfo domain.Tile
		if err := json.Unmarshal([]byte(value), &tileInfo); err != nil {
			continue // 无效数据直接跳过
		}
		tileMap[tileID] = tileInfo
	}

	return tileMap, nil
}

func SetAllRoomTiles(rdb *redis.Client, roomID string, tiles map[string]domain.Tile) error {
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
