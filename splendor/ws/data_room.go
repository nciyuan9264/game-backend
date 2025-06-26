package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/entities"
	"go-game/repository"
	"log"
	"strconv"

	"github.com/go-redis/redis/v8"
)

func SetNormalCardByID(roomID string, card *entities.NormalCard) error {
	cardKey := fmt.Sprintf("room:%s:card", roomID)

	cardJSON, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("卡牌序列化失败: %w", err)
	}

	if err := repository.Rdb.HSet(repository.Ctx, cardKey, card.ID, cardJSON).Err(); err != nil {
		return fmt.Errorf("保存卡牌失败: %w", err)
	}

	return nil
}

func GetNormalCardByID(roomID string, cardID string) (*entities.NormalCard, error) {
	cardKey := fmt.Sprintf("room:%s:card", roomID)

	result, err := repository.Rdb.HGet(repository.Ctx, cardKey, cardID).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("卡牌 %s 不存在", cardID)
		}
		return nil, fmt.Errorf("获取卡牌失败: %w", err)
	}

	var card entities.NormalCard
	if err := json.Unmarshal([]byte(result), &card); err != nil {
		return nil, fmt.Errorf("卡牌解析失败: %w", err)
	}

	return &card, nil
}

func SetNobleCardByID(roomID string, card *entities.NobleCard) error {
	cardKey := fmt.Sprintf("room:%s:nobles", roomID)

	cardJSON, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("卡牌序列化失败: %w", err)
	}

	if err := repository.Rdb.HSet(repository.Ctx, cardKey, card.ID, cardJSON).Err(); err != nil {
		return fmt.Errorf("保存卡牌失败: %w", err)
	}

	return nil
}

func GetNobleCardByID(roomID string, cardID string) (*entities.NobleCard, error) {
	cardKey := fmt.Sprintf("room:%s:nobles", roomID)

	result, err := repository.Rdb.HGet(repository.Ctx, cardKey, cardID).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("卡牌 %s 不存在", cardID)
		}
		return nil, fmt.Errorf("获取卡牌失败: %w", err)
	}

	var card entities.NobleCard
	if err := json.Unmarshal([]byte(result), &card); err != nil {
		return nil, fmt.Errorf("卡牌解析失败: %w", err)
	}

	return &card, nil
}

func GetAllNormalCards(roomID string) (map[string]entities.NormalCard, error) {
	cardKey := fmt.Sprintf("room:%s:card", roomID)

	// 取出所有 field-value 对
	result, err := repository.Rdb.HGetAll(repository.Ctx, cardKey).Result()
	if err != nil {
		return nil, err
	}

	cards := make(map[string]entities.NormalCard, len(result))
	for cardID, cardJSON := range result {
		var card entities.NormalCard
		if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
			return nil, fmt.Errorf("卡牌 %s 反序列化失败: %w", cardID, err)
		}
		cards[cardID] = card
	}

	return cards, nil
}

func GetAllNobleCards(roomID string) (map[string]entities.NobleCard, error) {
	noblesKey := fmt.Sprintf("room:%s:nobles", roomID)

	result, err := repository.Rdb.HGetAll(repository.Ctx, noblesKey).Result()
	if err != nil {
		return nil, err
	}

	nobles := make(map[string]entities.NobleCard, len(result))
	for nobleID, nobleJSON := range result {
		var noble entities.NobleCard
		if err := json.Unmarshal([]byte(nobleJSON), &noble); err != nil {
			return nil, fmt.Errorf("贵族瓷砖 %s 反序列化失败: %w", nobleID, err)
		}
		nobles[nobleID] = noble
	}

	return nobles, nil
}

func SetGemCounts(roomID string, gemCounts map[string]int) error {
	gemKey := fmt.Sprintf("room:%s:gems", roomID)

	// 将 map[string]int 转换为 map[string]string
	gemStrMap := make(map[string]string, len(gemCounts))
	for color, count := range gemCounts {
		gemStrMap[color] = strconv.Itoa(count)
	}

	// 一次性写入 Redis 的 Hash
	if err := repository.Rdb.HMSet(repository.Ctx, gemKey, gemStrMap).Err(); err != nil {
		return fmt.Errorf("设置房间 %s 的宝石信息失败: %w", roomID, err)
	}

	return nil
}

func GetGemCounts(roomID string) (map[string]int, error) {
	gemKey := fmt.Sprintf("room:%s:gems", roomID)

	result, err := repository.Rdb.HGetAll(repository.Ctx, gemKey).Result()
	if err != nil {
		return nil, err
	}

	gemCounts := make(map[string]int, len(result))
	for color, countStr := range result {
		count, err := strconv.Atoi(countStr)
		if err != nil {
			return nil, fmt.Errorf("宝石颜色 %s 数量转换失败: %w", color, err)
		}
		gemCounts[color] = count
	}

	return gemCounts, nil
}

// 判断玩家信息是否存在
func IsPlayerInfoExists(rdb *redis.Client, ctx context.Context, roomID, playerID string) (bool, error) {
	playerInfoKey := fmt.Sprintf("room:%s:player:%s:gem", roomID, playerID)
	exists, err := rdb.Exists(ctx, playerInfoKey).Result()
	if err != nil {
		return false, fmt.Errorf("检查玩家数据失败: %w", err)
	}
	return exists > 0, nil
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

// GetRoomInfo 获取房间的全部信息（Hash）
func GetRoomInfo(roomID string) (*entities.RoomInfo, error) {
	roomKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	roomInfoMap, err := repository.Rdb.HGetAll(repository.Ctx, roomKey).Result()
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
	roomInfo.GameStatus = entities.RoomStatus(roomInfoMap["gameStatus"])
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

func SetGameStatus(rdb *redis.Client, roomID string, status entities.RoomStatus) error {
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

func SetFirstPlayer(rdb *redis.Client, ctx context.Context, roomID, playerID string) error {
	key := fmt.Sprintf("room:%s:firstPlayer", roomID)
	if err := rdb.Set(ctx, key, playerID, 0).Err(); err != nil {
		return fmt.Errorf("设置first玩家失败: %w", err)
	}
	log.Printf("✅ 当前玩家已设置: roomID=%s playerID=%s\n", roomID, playerID)
	return nil
}

// GetCurrentPlayer 获取当前玩家
func GetFirstPlayer(rdb *redis.Client, ctx context.Context, roomID string) (string, error) {
	key := fmt.Sprintf("room:%s:firstPlayer", roomID)
	playerID, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", fmt.Errorf("获取first玩家失败: %w", err)
	}
	return playerID, nil
}
