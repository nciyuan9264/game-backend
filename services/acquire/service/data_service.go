package service

import (
	"acquire-service/dto"
	"acquire-service/entities"
	"acquire-service/repository"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

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

func SetMergeSettleData(ctx context.Context, rdb *redis.Client, roomID string, data map[string]dto.SettleData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化 SettleData map 失败: %w", err)
	}

	key := fmt.Sprintf("room:%s:merge_settle_temp", roomID)
	if err := rdb.Set(ctx, key, jsonData, 0).Err(); err != nil {
		return fmt.Errorf("写入 Redis 失败: %w", err)
	}
	return nil
}

func GetMergeSettleData(ctx context.Context, rdb *redis.Client, roomID string) (map[string]dto.SettleData, error) {
	key := fmt.Sprintf("room:%s:merge_settle_temp", roomID)

	result, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return map[string]dto.SettleData{}, nil
		}
		return nil, fmt.Errorf("从 Redis 获取数据失败: %w", err)
	}

	var data map[string]dto.SettleData
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return nil, fmt.Errorf("反序列化 SettleData map 失败: %w", err)
	}

	return data, nil
}

func SetMergingSelection(rdb *redis.Client, ctx context.Context, roomID string, company entities.MergingSelection) error {
	key := fmt.Sprintf("room:%s:merge_selection_temp", roomID)

	// 将结构体序列化为 JSON
	companyJson, err := json.Marshal(company)
	if err != nil {
		return fmt.Errorf("序列化合并选择失败: %w", err)
	}

	// 存储到 Redis
	if err := rdb.Set(ctx, key, companyJson, 0).Err(); err != nil {
		return fmt.Errorf("设置合并选择失败: %w", err)
	}

	return nil
}

// GetMergeOtherCompanies 从Redis获取合并的其他公司列表
func GetMergingSelection(rdb *redis.Client, ctx context.Context, roomID string) (entities.MergingSelection, error) {
	key := fmt.Sprintf("room:%s:merge_selection_temp", roomID)

	var selection entities.MergingSelection

	// 从 Redis 中获取 JSON 数据
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return entities.MergingSelection{}, nil
		}
		return selection, fmt.Errorf("❌ 获取合并选择失败: %w", err)
	}

	// 反序列化 JSON 到结构体
	if err := json.Unmarshal([]byte(data), &selection); err != nil {
		return selection, fmt.Errorf("❌ 解析合并选择 JSON 失败: %w", err)
	}

	return selection, nil
}

// SetLastTileKey 保存刚才放置的tile
func SetLastTileKey(rdb *redis.Client, ctx context.Context, roomID, playerID, tileKey string) error {
	createTileKey := fmt.Sprintf("room:%s:last_tile_key_temp", roomID)
	if err := rdb.Set(ctx, createTileKey, tileKey, 0).Err(); err != nil {
		return fmt.Errorf("保存触发创建公司tile编号失败: %w", err)
	}
	return nil
}

// GetLastTileKey 获取刚才放置的tile
func GetLastTileKey(rdb *redis.Client, ctx context.Context, roomID string) (string, error) {
	createTileKey := fmt.Sprintf("room:%s:last_tile_key_temp", roomID)
	tileKey, err := rdb.Get(ctx, createTileKey).Result()
	if err != nil {
		return "", fmt.Errorf("获取触发创建公司tile编号失败: %w", err)
	}
	return tileKey, nil
}

// SetMergeMainCompany 设置合并的主公司名称
func SetMergeMainCompany(rdb *redis.Client, ctx context.Context, roomID string, company string) error {
	mainCompanyNameKey := fmt.Sprintf("room:%s:merge_main_company_temp", roomID)
	if err := rdb.Set(ctx, mainCompanyNameKey, company, 0).Err(); err != nil {
		return fmt.Errorf("设置合并主公司失败: %w", err)
	}
	return nil
}

// GetMergeMainCompany 从Redis获取合并的主公司名称
func GetMergeMainCompany(rdb *redis.Client, ctx context.Context, roomID string) (string, error) {
	mainCompanyKey := fmt.Sprintf("room:%s:merge_main_company_temp", roomID)

	// 从Redis获取主公司名称
	company, err := rdb.Get(ctx, mainCompanyKey).Result()
	if err != nil {
		if err == redis.Nil {
			// 键不存在时返回空字符串
			return "", nil
		}
		return "", fmt.Errorf("获取主公司名称失败: %w", err)
	}

	return company, nil
}

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

// SetCompanyInfo 批量设置公司信息(companyInfo仅在每次广播时同步即可，日常无需修改)
func SetCompanyInfo(rdb *redis.Client, roomID string, companyInfoMap map[string]entities.CompanyInfo) error {
	for companyID, info := range companyInfoMap {
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, companyID)

		// 使用 HSet 设置哈希字段
		err := rdb.HSet(repository.Ctx, companyKey, map[string]interface{}{
			"name":       info.Name,
			"stockPrice": info.StockPrice,
			"stockTotal": info.StockTotal,
			"tiles":      info.Tiles,
		}).Err()
		if err != nil {
			log.Printf("❌ 写入公司[%s]信息失败: %v\n", companyID, err)
			return fmt.Errorf("写入公司[%s]信息失败: %w", companyID, err)
		}

		// 添加 companyID 到 room 的公司集合中，确保可以被 Get 时遍历到
		err = rdb.SAdd(repository.Ctx, fmt.Sprintf("room:%s:company_ids", roomID), companyID).Err()
		if err != nil {
			log.Printf("⚠️ 添加公司[%s]到集合失败: %v\n", companyID, err)
			// 非致命，可以继续
		}
	}

	return nil
}

// GetCompanyInfo 返回所有公司信息
func GetCompanyInfo(rdb *redis.Client, roomID string) (map[string]entities.CompanyInfo, error) {
	companyIDs, err := rdb.SMembers(repository.Ctx, fmt.Sprintf("room:%s:company_ids", roomID)).Result()
	if err != nil {
		return nil, fmt.Errorf("获取公司ID失败: %w", err)
	}

	companyInfo := make(map[string]entities.CompanyInfo)
	for _, companyID := range companyIDs {
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, companyID)
		data, err := rdb.HGetAll(repository.Ctx, companyKey).Result()
		if err != nil {
			log.Printf("❌ 获取公司[%s]信息失败: %v\n", companyID, err)
			continue
		}

		// 转换字段
		stockPrice, _ := strconv.Atoi(data["stockPrice"])
		stockTotal, _ := strconv.Atoi(data["stockTotal"])
		tiles, _ := strconv.Atoi(data["tiles"])

		companyInfo[companyID] = entities.CompanyInfo{
			Name:       data["name"],
			StockPrice: stockPrice,
			StockTotal: stockTotal,
			Tiles:      tiles,
		}
	}

	return companyInfo, nil
}
