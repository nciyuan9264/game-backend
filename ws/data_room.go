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

// SetTileToRedis 将 tile 的 JSON 数据写回 Redis 中
func SetTileToRedis(rdb *redis.Client, ctx context.Context, roomID, tileKey string, tile dto.Tile) error {
	tileJson, err := json.Marshal(tile)
	if err != nil {
		return fmt.Errorf("❌ 重新编码 Tile JSON 失败:", err)
	}
	redisKey := fmt.Sprintf("room:%s:tiles", roomID)
	if err := rdb.HSet(ctx, redisKey, tileKey, tileJson).Err(); err != nil {
		return fmt.Errorf("❌ 写入 Redis 失败: %v\n", err)
	}
	return nil
}

func saveCreateCompanyTileKey(rdb *redis.Client, ctx context.Context, roomID, playerID, tileKey string) error {
	createTileKey := fmt.Sprintf("room:%s:create_company_tile_temp", roomID)
	if err := rdb.Set(ctx, createTileKey, tileKey, 0).Err(); err != nil {
		return fmt.Errorf("保存触发创建公司tile编号失败: %w", err)
	}
	return nil
}

func getCreateCompanyTileKey(rdb *redis.Client, ctx context.Context, roomID string) (string, error) {
	createTileKey := fmt.Sprintf("room:%s:create_company_tile_temp", roomID)
	tileKey, err := rdb.Get(ctx, createTileKey).Result()
	if err != nil {
		return "", fmt.Errorf("获取触发创建公司tile编号失败: %w", err)
	}
	return tileKey, nil
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

func SetCurrentPlayer(rdb *redis.Client, ctx context.Context, roomID, playerID string) error {
	key := fmt.Sprintf("room:%s:currentPlayer", roomID)
	if err := rdb.Set(ctx, key, playerID, 0).Err(); err != nil {
		return fmt.Errorf("设置当前玩家失败: %w", err)
	}
	log.Printf("✅ 当前玩家已设置: roomID=%s playerID=%s\n", roomID, playerID)
	return nil
}

func GetCurrentPlayer(rdb *redis.Client, ctx context.Context, roomID string) (string, error) {
	key := fmt.Sprintf("room:%s:currentPlayer", roomID)
	playerID, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("获取当前玩家失败: %w", err)
	}
	return playerID, nil
}

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

func SetMergeOtherCompanies(rdb *redis.Client, ctx context.Context, roomID string, otherCompanies []string) error {
	otherCompaniesKey := fmt.Sprintf("room:%s:merge_other_companies_temp", roomID)

	// 将字符串切片序列化为JSON
	companiesJson, err := json.Marshal(otherCompanies)
	if err != nil {
		return fmt.Errorf("序列化公司列表失败: %w", err)
	}

	// 存储到Redis
	if err := rdb.Set(ctx, otherCompaniesKey, companiesJson, 0).Err(); err != nil {
		return fmt.Errorf("设置合并其他公司失败: %w", err)
	}

	return nil
}

// GetMergeOtherCompanies 从Redis获取合并的其他公司列表
func GetMergeOtherCompanies(rdb *redis.Client, ctx context.Context, roomID string) ([]string, error) {
	otherCompaniesKey := fmt.Sprintf("room:%s:merge_other_companies_temp", roomID)

	// 从Redis获取JSON数据
	jsonData, err := rdb.Get(ctx, otherCompaniesKey).Result()
	if err != nil {
		if err == redis.Nil {
			// 键不存在时返回空切片而不是错误
			return []string{}, nil
		}
		return nil, fmt.Errorf("获取其他公司列表失败: %w", err)
	}

	// 反序列化JSON到字符串切片
	var otherCompanies []string
	if err := json.Unmarshal([]byte(jsonData), &otherCompanies); err != nil {
		return nil, fmt.Errorf("解析其他公司列表失败: %w", err)
	}

	return otherCompanies, nil
}

func SetPlayerNeedSettle(rdb *redis.Client, ctx context.Context, roomID string, player []string) error {
	mergePlayerSettleKey := fmt.Sprintf("room:%s:merge_settle_player_temp", roomID)

	// 将字符串切片序列化为JSON
	playersJson, err := json.Marshal(mergePlayerSettleKey)
	if err != nil {
		return fmt.Errorf("序列化玩家列表失败: %w", err)
	}
	// 存储到Redis
	if err := rdb.Set(ctx, mergePlayerSettleKey, playersJson, 0).Err(); err != nil {
		return fmt.Errorf("设置玩家失败: %w", err)
	}

	return nil
}

// GetMergeOtherCompanies 从Redis获取合并的其他公司列表
func GetPlayerNeedSettle(rdb *redis.Client, ctx context.Context, roomID string) ([]string, error) {
	mergePlayerSettleKey := fmt.Sprintf("room:%s:merge_settle_player_temp", roomID)

	// 从Redis获取JSON数据
	jsonData, err := rdb.Get(ctx, mergePlayerSettleKey).Result()
	if err != nil {
		if err == redis.Nil {
			// 键不存在时返回空切片而不是错误
			return []string{}, nil
		}
		return nil, fmt.Errorf("获取玩家列表失败: %w", err)
	}

	// 反序列化JSON到字符串切片
	var playerNeedSettle []string
	if err := json.Unmarshal([]byte(jsonData), &playerNeedSettle); err != nil {
		return nil, fmt.Errorf("解析玩家列表失败: %w", err)
	}

	return playerNeedSettle, nil
}
