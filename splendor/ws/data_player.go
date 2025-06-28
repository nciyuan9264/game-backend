package ws

import (
	"fmt"
	"go-game/entities"
	"go-game/repository"
	"strconv"

	"encoding/json"

	"github.com/go-redis/redis/v8"
)

// SetJSONToRedisHash 将任意结构体序列化为 JSON 存入 Redis 哈希的 "data" 字段
func SetPlayerNormalCard(roomID, playerID string, value []entities.NormalCard) error {
	playerNormalCardKey := fmt.Sprintf("room:%s:player:%s:normalCard", roomID, playerID)
	bytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化玩家预留贵族卡失败: %w", err)
	}

	return repository.Rdb.HSet(repository.Ctx, playerNormalCardKey, "data", bytes).Err()
}

// GetJSONFromRedisHash 从 Redis 哈希的 "data" 字段获取并反序列化为传入的目标结构
func GetPlayerNormalCard(roomID, playerID string) ([]entities.NormalCard, error) {
	playerNormalCardKey := fmt.Sprintf("room:%s:player:%s:normalCard", roomID, playerID)

	val, err := repository.Rdb.HGet(repository.Ctx, playerNormalCardKey, "data").Result()
	if err != redis.Nil && err != nil {
		return nil, err
	}
	if err == redis.Nil {
		return []entities.NormalCard{}, fmt.Errorf("获取玩家normal卡失败: %w", err)
	}

	var reserve []entities.NormalCard
	if err := json.Unmarshal([]byte(val), &reserve); err != nil {
		return nil, fmt.Errorf("反序列化玩家normal卡失败: %w", err)
	}

	return reserve, nil
}

func SetPlayerNobleCard(roomID, playerID string, value []entities.NobleCard) error {
	playerCardKey := fmt.Sprintf("room:%s:player:%s:nobleCard", roomID, playerID)
	bytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return repository.Rdb.HSet(repository.Ctx, playerCardKey, "data", bytes).Err()
}

// GetJSONFromRedisHash 从 Redis 哈希的 "data" 字段获取并反序列化为传入的目标结构
func GetPlayerNobleCard(roomID, playerID string) ([]entities.NobleCard, error) {
	playerCardKey := fmt.Sprintf("room:%s:player:%s:nobleCard", roomID, playerID)

	// 从 Redis 获取字符串形式的 JSON 数据
	val, err := repository.Rdb.HGet(repository.Ctx, playerCardKey, "data").Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	if err == redis.Nil {
		return []entities.NobleCard{}, nil
	}

	var nobleCards []entities.NobleCard
	if err := json.Unmarshal([]byte(val), &nobleCards); err != nil {
		return nil, err
	}

	return nobleCards, nil
}

func SetPlayerGem(roomID, playerID string, gem map[string]int) error {
	key := fmt.Sprintf("room:%s:player:%s:gem", roomID, playerID)
	bytes, err := json.Marshal(gem)
	if err != nil {
		return err
	}
	return repository.Rdb.HSet(repository.Ctx, key, "data", bytes).Err()
}

func GetPlayerGem(roomID, playerID string) (map[string]int, error) {
	key := fmt.Sprintf("room:%s:player:%s:gem", roomID, playerID)

	val, err := repository.Rdb.HGet(repository.Ctx, key, "data").Result()
	if err != nil {
		return nil, err
	}

	var gem map[string]int
	if err := json.Unmarshal([]byte(val), &gem); err != nil {
		return nil, err
	}

	return gem, nil
}

// SetPlayerScore 将玩家的分数写入 Redis 哈希中（field 为 "data"）
func SetPlayerScore(roomID, playerID string, score int) error {
	key := fmt.Sprintf("room:%s:player:%s:score", roomID, playerID)
	return repository.Rdb.HSet(repository.Ctx, key, "data", score).Err()
}

// GetPlayerScore 从 Redis 中获取玩家的分数（从 field "data" 取出并转为 int）
func GetPlayerScore(roomID, playerID string) (int, error) {
	key := fmt.Sprintf("room:%s:player:%s:score", roomID, playerID)

	val, err := repository.Rdb.HGet(repository.Ctx, key, "data").Result()
	if err != nil {
		return 0, err
	}

	score, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}

	return score, nil
}

func SetPlayerReserveCards(roomID, playerID string, reserve []entities.NormalCard) error {
	key := fmt.Sprintf("room:%s:player:%s:reserve", roomID, playerID)

	bytes, err := json.Marshal(reserve)
	if err != nil {
		return fmt.Errorf("序列化玩家预留贵族卡失败: %w", err)
	}

	return repository.Rdb.HSet(repository.Ctx, key, "data", bytes).Err()
}

func GetPlayerReserveCards(roomID, playerID string) ([]entities.NormalCard, error) {
	key := fmt.Sprintf("room:%s:player:%s:reserve", roomID, playerID)

	val, err := repository.Rdb.HGet(repository.Ctx, key, "data").Result()
	if err == redis.Nil {
		// 说明字段不存在，直接返回空列表，不报错
		return []entities.NormalCard{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("获取玩家预留卡失败: %w", err)
	}

	var reserve []entities.NormalCard
	if err := json.Unmarshal([]byte(val), &reserve); err != nil {
		return nil, fmt.Errorf("反序列化玩家预留卡失败: %w", err)
	}

	return reserve, nil
}
