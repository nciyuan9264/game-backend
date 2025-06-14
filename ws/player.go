package ws

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"go-game/utils"
	"log"
	"math/rand/v2"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

func generateAvailableTiles(roomID string) ([]string, error) {
	ctx := repository.Ctx
	rdb := repository.Rdb

	// 获取所有 tile 的占用信息
	tileKey := fmt.Sprintf("room:%s:tiles", roomID)
	tileMap, err := rdb.HGetAll(ctx, tileKey).Result()
	if err != nil {
		return nil, fmt.Errorf("获取 tiles 信息失败: %w", err)
	}

	playerTiles := make(map[string]struct{})
	for _, pc := range Rooms[roomID] {
		tiles, err := GetPlayerTiles(rdb, ctx, roomID, pc.PlayerID)
		if err != nil {
			log.Printf("❌ 获取玩家 %s 的 tiles 失败: %v\n", pc.PlayerID, err)
			continue
		}
		for _, tile := range tiles {
			playerTiles[tile] = struct{}{}
		}
	}

	var available []string
	for tileID, value := range tileMap {
		var tileInfo dto.Tile
		err := json.Unmarshal([]byte(value), &tileInfo)
		if err != nil {
			continue
		}
		_, exists := playerTiles[tileID]
		if exists {
			continue
		}

		if tileInfo.Belong == "" && !exists {
			available = append(available, tileID)
		}
	}

	return available, nil
}

// 初始化玩家数据
func InitPlayerData(roomID string, playerID string) error {
	// 1. 检查玩家数据是否已存在
	exists, err := IsPlayerInfoExists(repository.Rdb, repository.Ctx, roomID, playerID)
	if err != nil {
		log.Println(err)
		return err
	}
	if exists {
		return fmt.Errorf("玩家数据已存在")
	}
	// 2. 设置初始资金
	err = SetPlayerInfoField(repository.Rdb, repository.Ctx, roomID, playerID, "money", 6000)
	if err != nil {
		log.Println("设置玩家信息失败:", err)
	}

	// 2. 随机抽取起始 Tiles（比如每人 3 个）
	allTiles, err := generateAvailableTiles(roomID)
	if err != nil {
		return err
	}
	rand.Shuffle(len(allTiles), func(i, j int) { allTiles[i], allTiles[j] = allTiles[j], allTiles[i] })

	playerTiles := utils.SafeSlice(allTiles, 5)
	err = SetPlayerTiles(repository.Rdb, repository.Ctx, roomID, playerID, playerTiles)
	if err != nil {
		log.Println(err)
	}
	// 3. 初始化玩家股票（全部为 0）
	// 3.1 获取公司ID列表
	companyIDs, err := getCompanyIDs(roomID)
	if err != nil {
		log.Println("获取公司ID失败:", err)
		return err
	}
	// 3.2 初始化玩家股票为0
	playerStocks := make(map[string]int)
	for _, company := range companyIDs {
		playerStocks[company] = 0
	}
	err = SetPlayerStocks(repository.Rdb, repository.Ctx, roomID, playerID, playerStocks)
	if err != nil {
		log.Println("写入玩家股票失败:", err)
	}

	return nil
}

type TriggerRuleParams struct {
	Conn     *websocket.Conn
	Rdb      *redis.Client
	RoomID   string
	PlayerID string
	TileKey  string
}

func getKeysFromSet(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	return keys
}
