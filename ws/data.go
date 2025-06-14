package ws

import (
	"context"
	"encoding/json"
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

func SetPlayerInfoField(rdb *redis.Client, ctx context.Context, roomID, playerID, field string, value interface{}) error {
	playerInfoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	if err := rdb.HSet(ctx, playerInfoKey, field, value).Err(); err != nil {
		return err
	}
	return nil
}
func AddPlayerMoney(rdb *redis.Client, ctx context.Context, roomID, playerID string, amount int) error {
	playerInfoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	err := rdb.HIncrBy(ctx, playerInfoKey, "money", int64(amount)).Err()
	if err != nil {
		return fmt.Errorf("添加余额失败[%s]: %w", playerID, err)
	}
	return nil
}
func DeductPlayerMoney(rdb *redis.Client, ctx context.Context, roomID, playerID string, amount int) error {
	if amount < 0 {
		return fmt.Errorf("扣款金额不能为负数")
	}
	return AddPlayerMoney(rdb, ctx, roomID, playerID, -amount)
}
func GetPlayerInfoField(rdb *redis.Client, ctx context.Context, roomID, playerID, field string) (string, error) {
	playerInfoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	value, err := rdb.HGet(ctx, playerInfoKey, field).Result()
	if err != nil {
		return "", err
	}
	return value, nil
}

// 将玩家的牌组批量写入 Redis 列表（尾部追加）
func SetPlayerTiles(rdb *redis.Client, ctx context.Context, roomID, playerID string, tiles []string) error {
	tileListKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	if len(tiles) == 0 {
		return nil // 没有牌就直接返回
	}

	// RPush 支持可变参数，需要转成 interface{} 切片
	args := make([]interface{}, len(tiles))
	for i, t := range tiles {
		args[i] = t
	}

	if err := rdb.RPush(ctx, tileListKey, args...).Err(); err != nil {
		return fmt.Errorf("添加玩家牌组失败: %w", err)
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

func getCompanyIDs(roomID string) ([]string, error) {
	ctx := repository.Ctx
	rdb := repository.Rdb

	key := fmt.Sprintf("room:%s:company_ids", roomID)
	ids, err := rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("获取公司ID失败: %w", err)
	}
	return ids, nil
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

// GetTileFromRedis 从 Redis 获取指定房间的某个 tile 信息
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
