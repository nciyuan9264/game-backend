package ws

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"go-game/utils"
	"log"
	"os"
	"path"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

func WriteGameLog(roomID, playerID string, roomInfo *entities.RoomInfo, msg map[string]interface{}) {
	go func() {
		logPath := getGameLogFilePath(roomID)

		// 确保目录存在
		if err := os.MkdirAll(path.Dir(logPath), 0755); err != nil {
			log.Println("❌ 创建日志目录失败:", err)
			return
		}

		entry := map[string]interface{}{
			"timestamp":  time.Now().Format("2006-01-02 15:04:05"),
			"result":     msg["result"],
			"roomInfo":   roomInfo,
			"playerID":   playerID,
			"playerData": msg["playerData"],
			"roomData":   msg["roomData"],
			"tempData":   msg["tempData"],
		}

		jsonEntry, err := json.Marshal(entry)
		if err != nil {
			log.Println("❌ 序列化日志 entry 失败:", err)
			return
		}

		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Println("❌ 打开游戏日志文件失败:", err)
			return
		}
		defer f.Close()

		jsonEntry = append(jsonEntry, ',')

		if _, err := f.Write(jsonEntry); err != nil {
			log.Println("❌ 写入日志失败:", err)
			return
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			log.Println("❌ 写入换行失败:", err)
		}
	}()
}

// 向该客户端发送同步消息
func SyncRoomMessage(conn dto.ConnInterface, roomID string, playerID string, result map[string]int) error {
	rdb := repository.Rdb
	ctx := repository.Ctx

	// ------- 构造 Redis Key -------
	infoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	tilesKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	currentPlayerKey := fmt.Sprintf("room:%s:currentPlayer", roomID)
	companyIDsKey := fmt.Sprintf("room:%s:company_ids", roomID)
	lastTileKey := fmt.Sprintf("room:%s:last_tile_key_temp", roomID)

	// ------- 第一次 pipeline：玩家、房间、tile 基础数据 -------
	pipe := rdb.Pipeline()
	infoCmd := pipe.HGetAll(ctx, infoKey)
	tilesCmd := pipe.LRange(ctx, tilesKey, 0, -1)
	currentPlayerCmd := pipe.Get(ctx, currentPlayerKey)
	companyIDsCmd := pipe.SMembers(ctx, companyIDsKey)
	lastTileKeyCmd := pipe.Get(ctx, lastTileKey)

	// 执行 pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("❌ Redis pipeline 执行失败: %w", err)
	}

	// ------- 提取结果 -------
	info := infoCmd.Val()
	tiles := tilesCmd.Val()
	currentPlayer := currentPlayerCmd.Val()
	companyIDs := companyIDsCmd.Val()
	lastTile := lastTileKeyCmd.Val()

	// ------- 第二次 pipeline：批量获取所有公司信息 -------
	pipe2 := rdb.Pipeline()
	companyCmds := make(map[string]*redis.StringStringMapCmd)

	for _, companyID := range companyIDs {
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, companyID)
		companyCmds[companyID] = pipe2.HGetAll(ctx, companyKey)
	}

	_, err = pipe2.Exec(ctx)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("❌ 获取公司信息 pipeline 执行失败: %w", err)
	}

	companyInfo, err := GetCompanyInfo(rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取公司信息失败: %w", err)
	}

	roomInfo, err := GetRoomInfo(rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取房间信息失败: %w", err)
	}

	// ------- 其他 Redis 相关调用 -------
	tileMap, err := GetAllRoomTiles(rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取房间 tile 信息失败: %w", err)
	}

	merge_main_company_temp, err := GetMergeMainCompany(rdb, ctx, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取合并主公司信息失败: %w", err)
	}

	merge_selection_temp, err := GetMergingSelection(rdb, ctx, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取合并选择信息失败: %w", err)
	}

	mergeSettleData, err := GetMergeSettleData(ctx, rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取合并结算信息失败: %w", err)
	}

	stocks, err := GetPlayerStocks(rdb, ctx, roomID, playerID)
	if err != nil {
		return fmt.Errorf("❌ 获取玩家股票信息失败: %w", err)
	}

	// ------- 组装消息 -------
	msg := map[string]interface{}{
		"type":     "sync",
		"result":   result,
		"playerId": playerID,
		"playerData": map[string]interface{}{
			"info":   info,
			"stocks": stocks,
			"tiles":  tiles,
		},
		"roomData": map[string]interface{}{
			"companyInfo":   companyInfo,
			"currentPlayer": currentPlayer,
			"roomInfo":      roomInfo,
			"tiles":         tileMap,
		},
		"tempData": map[string]interface{}{
			"last_tile_key":           lastTile,
			"merge_main_company_temp": merge_main_company_temp,
			"merge_selection_temp":    merge_selection_temp,
			"mergeSettleData":         mergeSettleData,
		},
	}

	// ------- 发送 WebSocket 消息 -------
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("❌ 编码 JSON 失败: %w", err)
	}
	if playerID == currentPlayer {
		WriteGameLog(roomID, playerID, roomInfo, msg)
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// 广播消息给房间内所有连接成功的玩家
func BroadcastToRoom(roomID string) {
	companyInfoMap, err := GetCompanyInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("获取公司信息失败:", err)
		return
	}

	tileMap, err := GetAllRoomTiles(repository.Rdb, roomID)
	if err != nil {
		log.Println("获取所有 tile 失败:", err)
		return
	}
	allTileMap := make(map[string]int)
	for _, tile := range tileMap {
		if tile.Belong != "" && tile.Belong != "Blank" {
			allTileMap[tile.Belong] = allTileMap[tile.Belong] + 1
		}
	}

	allStockMap := make(map[string]int)
	for _, pc := range Rooms[roomID] {
		stockMap, err := GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, pc.PlayerID)
		if err != nil {
			log.Printf("❌ 获取玩家[%s]股票失败: %v\n", pc.PlayerID, err)
			return
		}
		for stockID, stockCount := range stockMap {
			allStockMap[stockID] += stockCount
		}
	}

	for companyName, info := range companyInfoMap {
		stockInfo := utils.GetStockInfo(companyName, allTileMap[companyName])
		stockLeft := 25 - allStockMap[companyName]
		info.StockTotal = stockLeft
		info.Tiles = allTileMap[companyName]
		info.StockPrice = stockInfo.Price
		companyInfoMap[companyName] = info
	}

	err = SetCompanyInfo(repository.Rdb, roomID, companyInfoMap)
	if err != nil {
		log.Println("❌ 设置公司信息失败:", err)
		return
	}

	result := make(map[string]int)
	for _, pc := range Rooms[roomID] {
		playerStocks, err := GetPlayerStocks(repository.Rdb, repository.Ctx, roomID, pc.PlayerID)
		if err != nil {
			log.Printf("❌ 获取玩家[%s]股票失败: %v\n", pc.PlayerID, err)
			continue
		}
		playerInfo, err := GetPlayerInfoField(repository.Rdb, repository.Ctx, roomID, pc.PlayerID, "money")
		if err != nil {
			log.Printf("❌ 获取玩家[%s]金钱失败: %v\n", pc.PlayerID, err)
			continue
		}
		result[pc.PlayerID] = CalculateTotalValue(playerStocks, companyInfoMap) + playerInfo.Money
	}

	for _, pc := range Rooms[roomID] {
		if pc.Online {
			// 尝试发送消息
			if err := SyncRoomMessage(pc.Conn, roomID, pc.PlayerID, result); err != nil {
				log.Println("广播失败，移除连接:", pc.PlayerID)
				pc.Conn.Close()
			}
		}
	}
}
