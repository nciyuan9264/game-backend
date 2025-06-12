package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/entities"

	"github.com/go-redis/redis/v8"
)

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
