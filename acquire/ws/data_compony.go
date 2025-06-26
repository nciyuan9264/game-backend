package ws

import (
	"fmt"
	"go-game/entities"
	"go-game/repository"
	"log"
	"strconv"

	"github.com/go-redis/redis/v8"
)

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
