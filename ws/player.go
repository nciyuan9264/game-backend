package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"log"
	"math/rand/v2"
	"strconv"

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

	var available []string
	for tileID, value := range tileMap {
		var tileInfo dto.Tile
		err := json.Unmarshal([]byte(value), &tileInfo)
		if err != nil {
			// 这里可以打印错误或者忽略
			continue
		}

		if tileInfo.Belong == "" {
			available = append(available, tileID)
		}
	}

	return available, nil
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

func InitPlayerData(roomID string, playerID string) error {
	ctx := context.Background()
	rdb := repository.Rdb
	playerInfoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)

	// 判断玩家数据是否已存在
	exists, err := rdb.Exists(ctx, playerInfoKey).Result()
	if err != nil {
		return fmt.Errorf("检查玩家数据失败: %w", err)
	}

	if exists > 0 {
		// 玩家信息已经存在，跳过初始化
		return nil
	}
	// 1. 设置初始资金
	if err := rdb.HSet(ctx, playerInfoKey, "money", 6000).Err(); err != nil {
		return err
	}

	// 2. 随机抽取起始 Tiles（比如每人 3 个）
	allTiles, err := generateAvailableTiles(roomID)
	if err != nil {
		return err
	}
	rand.Shuffle(len(allTiles), func(i, j int) { allTiles[i], allTiles[j] = allTiles[j], allTiles[i] })

	playerTiles := allTiles[:20]
	tileListKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	if err := rdb.RPush(ctx, tileListKey, playerTiles).Err(); err != nil {
		return err
	}

	// 3. 初始化玩家股票（全部为 0）
	companyIDs, err := getCompanyIDs(roomID)
	if err != nil {
		log.Println("获取公司ID失败:", err)
		return err
	}
	fmt.Println("公司ID列表：", companyIDs)
	playerStockKey := fmt.Sprintf("room:%s:player:%s:stocks", roomID, playerID)
	playerStocks := make(map[string]interface{})
	for _, company := range companyIDs {
		playerStocks[company] = 0
	}
	if err := rdb.HSet(ctx, playerStockKey, playerStocks).Err(); err != nil {
		return err
	}

	return nil
}

func getAdjacentTileKeys(tileKey string) []string {
	if len(tileKey) < 2 {
		return nil
	}
	row := tileKey[:len(tileKey)-1] // 例如 "6"
	col := tileKey[len(tileKey)-1:] // 例如 "A"

	// 上下左右邻接逻辑
	rowNum, err := strconv.Atoi(row)
	if err != nil {
		return nil
	}
	adjacent := []string{
		fmt.Sprintf("%d%s", rowNum-1, col),            // 上
		fmt.Sprintf("%d%s", rowNum+1, col),            // 下
		fmt.Sprintf("%d%s", rowNum, string(col[0]-1)), // 左
		fmt.Sprintf("%d%s", rowNum, string(col[0]+1)), // 右
	}
	return adjacent
}

type TriggerRuleParams struct {
	Conn     *websocket.Conn
	Rdb      *redis.Client
	RoomID   string
	PlayerID string
	TileKey  string
}

func checkTileTriggerRules(params TriggerRuleParams) {
	rdb := params.Rdb
	roomID := params.RoomID
	playerID := params.PlayerID
	tileKey := params.TileKey
	adjTiles := getAdjacentTileKeys(tileKey)
	redisKey := "room:" + roomID + ":tiles"

	hotelSet := make(map[string]struct{})
	blankTileCount := 0

	for _, adjKey := range adjTiles {
		jsonStr, err := rdb.HGet(repository.Ctx, redisKey, adjKey).Result()
		if err != nil {
			continue // 邻接 tile 不存在
		}

		var tile dto.Tile
		if err := json.Unmarshal([]byte(jsonStr), &tile); err != nil {
			continue
		}

		switch tile.Belong {
		case "Blank":
			blankTileCount++
		case "": // 未被占用
			continue
		default:
			hotelSet[tile.Belong] = struct{}{}
		}
	}

	if len(hotelSet) >= 2 {
		log.Println("⚠️ 触发并购规则！邻接多个酒店:", hotelSet)

		return
	}

	if blankTileCount >= 1 {
		log.Println("✅ 触发建立新酒店的可能性，邻接空地数量:", blankTileCount)

		// Step 1: 修改房间状态为“创建公司状态”
		updateRoomStatus(rdb, roomID, dto.RoomStatusCreateCompany)
		// Step 2: 保存当前玩家放置的 tile 编号
		createTileKey := "room:" + roomID + ":create_company_tile:" + playerID
		if err := rdb.Set(repository.Ctx, createTileKey, tileKey, 0).Err(); err != nil {
			log.Println("❌ 保存触发创建公司 tile 编号失败:", err)
		} else {
			log.Println("✅ 已保存触发创建公司的 tile 编号:", tileKey)
		}

		// Step 3: 发送消息同步到前端
		sendRoomMessage(roomID, playerID)

		return
	}

	log.Println("未触发规则")
}

func getKeysFromSet(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	return keys
}
