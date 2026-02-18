package data

import (
	"context"
	"fmt"
	"go-game/domain/domain"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"go-game/utils"
	"strconv"

	"github.com/go-redis/redis/v8"
	"golang.org/x/exp/rand"
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
	roomInfo.OwnerID = roomInfoMap["ownerID"]
	// 字符串转 int
	maxPlayersStr := roomInfoMap["maxPlayers"]
	if maxPlayersStr != "" {
		if val, err := strconv.Atoi(maxPlayersStr); err == nil {
			roomInfo.MaxPlayers = val
		} else {
			utils.Error("maxPlayers 转换失败", utils.F("error", err))
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
		"ownerID":    info.OwnerID,
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
		utils.Error("更新房间状态失败", utils.F("room_id", roomID), utils.F("game_status", status), utils.F("error", err))
		return err
	}
	utils.Info("房间（roomID: %s）状态已更新为：%s", utils.F("room_id", roomID), utils.F("game_status", status))
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
	utils.Info("✅ 当前玩家已设置: roomID=%s playerID=%s", utils.F("room_id", roomID), utils.F("player_id", playerID))
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

// 初始化玩家数据
func InitPlayerData(room *domain.Room, playerID string) error {
	// 1. 检查玩家数据是否已存在
	exists, err := IsPlayerInfoExists(repository.Rdb, repository.Ctx, room.ID, playerID)
	if err != nil {
		utils.Error("检查玩家数据失败", utils.F("room_id", room.ID), utils.F("player_id", playerID), utils.F("error", err))
		return err
	}
	if exists {
		return fmt.Errorf("玩家数据已存在")
	}
	// 2. 设置初始资金
	err = SetPlayerInfoField(repository.Rdb, repository.Ctx, room.ID, playerID, "money", 6000)
	if err != nil {
		utils.Error("设置玩家信息失败", utils.F("room_id", room.ID), utils.F("player_id", playerID), utils.F("error", err))
		return err
	}

	// 2. 随机抽取起始 Tiles（比如每人 5 个）
	allTiles, err := GenerateAvailableTiles(room)
	if err != nil {
		utils.Error("生成可用 tile 失败", utils.F("room_id", room.ID), utils.F("player_id", playerID), utils.F("error", err))
		return err
	}
	rand.Shuffle(len(allTiles), func(i, j int) { allTiles[i], allTiles[j] = allTiles[j], allTiles[i] })

	playerTiles := utils.SafeSlice(allTiles, 5)
	err = SetPlayerTiles(repository.Rdb, repository.Ctx, room.ID, playerID, playerTiles)
	if err != nil {
		utils.Error("添加 tile 失败", utils.F("room_id", room.ID), utils.F("player_id", playerID), utils.F("error", err))
		return err
	}
	// 3. 初始化玩家股票（全部为 0）
	// 3.1 获取公司ID列表
	companyIDs, err := GetCompanyIDs(room.ID)
	if err != nil {
		utils.Error("获取公司ID失败", utils.F("room_id", room.ID), utils.F("player_id", playerID), utils.F("error", err))
		return err
	}
	// 3.2 初始化玩家股票为0
	playerStocks := make(map[string]int)
	for _, company := range companyIDs {
		playerStocks[company] = 0
	}
	err = SetPlayerStocks(repository.Rdb, repository.Ctx, room.ID, playerID, playerStocks)
	if err != nil {
		utils.Error("写入玩家股票失败", utils.F("room_id", room.ID), utils.F("player_id", playerID), utils.F("error", err))
		return err
	}

	return nil
}
