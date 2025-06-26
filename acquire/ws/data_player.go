package ws

import (
	"context"
	"fmt"
	"go-game/dto"
	"log"
	"strconv"

	"github.com/go-redis/redis/v8"
)

func SetPlayerInfoField(rdb *redis.Client, ctx context.Context, roomID, playerID, field string, value interface{}) error {
	playerInfoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	if err := rdb.HSet(ctx, playerInfoKey, field, value).Err(); err != nil {
		return err
	}
	return nil
}
func GetPlayerInfoField(rdb *redis.Client, ctx context.Context, roomID, playerID, field string) (dto.PlayerInfo, error) {
	playerInfoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	value, err := rdb.HGet(ctx, playerInfoKey, field).Result()
	if err != nil {
		return dto.PlayerInfo{}, err
	}
	if field == "money" {
		intVal, convErr := strconv.Atoi(value)
		if convErr != nil {
			return dto.PlayerInfo{}, convErr
		}
		return dto.PlayerInfo{
			Money: intVal,
		}, nil // 重新格式化输出（可选）
	}
	return dto.PlayerInfo{}, nil
}

func AddPlayerMoney(rdb *redis.Client, ctx context.Context, roomID, playerID string, amount int) error {
	playerInfoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	err := rdb.HIncrBy(ctx, playerInfoKey, "money", int64(amount)).Err()
	if err != nil {
		return fmt.Errorf("添加余额失败[%s]: %w", playerID, err)
	}
	return nil
}

// 将玩家的牌组批量写入 Redis 列表（覆盖）
func SetPlayerTiles(rdb *redis.Client, ctx context.Context, roomID, playerID string, tiles []string) error {
	tileListKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)

	// 删除旧的列表
	if err := rdb.Del(ctx, tileListKey).Err(); err != nil {
		return fmt.Errorf("删除旧的牌组失败: %w", err)
	}

	// 没有新的牌就直接返回
	if len(tiles) == 0 {
		return nil
	}

	// RPush 需要 interface{} 类型参数
	args := make([]interface{}, len(tiles))
	for i, t := range tiles {
		args[i] = t
	}

	// 插入新的列表
	if err := rdb.RPush(ctx, tileListKey, args...).Err(); err != nil {
		return fmt.Errorf("设置新的牌组失败: %w", err)
	}
	return nil
}

func GetPlayerTiles(rdb *redis.Client, ctx context.Context, roomID, playerID string) ([]string, error) {
	tileListKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	tiles, err := rdb.LRange(ctx, tileListKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("获取玩家牌组失败: %w", err)
	}
	return tiles, nil
}

// AddPlayerTile 向指定玩家的 tile 列表中添加一个 tile
func AddPlayerTile(rdb *redis.Client, ctx context.Context, roomID, playerID, tileKey string) error {
	playerTileKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	if err := rdb.RPush(ctx, playerTileKey, tileKey).Err(); err != nil {
		log.Printf("❌ 向玩家 %s 添加 tile %s 失败: %v\n", playerID, tileKey, err)
		return err
	}
	log.Printf("✅ 向玩家 %s 添加 tile %s 成功\n", playerID, tileKey)
	return nil
}

// RemovePlayerTile 从指定玩家的 tile 列表中移除某个 tile
func RemovePlayerTile(rdb *redis.Client, ctx context.Context, roomID, playerID, tileKey string) error {
	playerTileKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	if err := rdb.LRem(ctx, playerTileKey, 1, tileKey).Err(); err != nil {
		return fmt.Errorf("从玩家 %s 的 tile 列表移除失败: %w", playerID, err)
	}
	return nil
}

// GetPlayerStocks 读取玩家的所有股票及持股数，返回 map[companyID]stockCountStr
func GetPlayerStocks(rdb *redis.Client, ctx context.Context, roomID, playerID string) (map[string]int, error) {
	key := fmt.Sprintf("room:%s:player:%s:stocks", roomID, playerID)
	result, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	intMap := make(map[string]int)
	for k, v := range result {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("字段[%s]值[%s]转换失败: %w", k, v, err)
		}
		intMap[k] = n
	}

	return intMap, nil
}

// SetPlayerStocks 设置玩家的股票信息，playerStocks 格式为 map[companyID]持股数量
func SetPlayerStocks(rdb *redis.Client, ctx context.Context, roomID, playerID string, playerStocks map[string]int) error {
	key := fmt.Sprintf("room:%s:player:%s:stocks", roomID, playerID)
	hashData := make(map[string]interface{})
	for k, v := range playerStocks {
		hashData[k] = strconv.Itoa(v)
	}
	return rdb.HSet(ctx, key, hashData).Err()
}
