package game

import (
	"encoding/json"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"go-game/utils"
	"log"
	"os"
	"path"
	"reflect"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
)

// 自定义 HookFunc，把字符串转换成 int
func stringToIntHookFunc() mapstructure.DecodeHookFunc {
	return func(from reflect.Kind, to reflect.Kind, data interface{}) (interface{}, error) {
		if from == reflect.String && to == reflect.Int {
			return strconv.Atoi(data.(string))
		}
		return data, nil
	}
}

func CalculateTotalValue(playerStocks map[string]int, companyInfoMap map[string]entities.CompanyInfo) int {
	totalValue := 0
	for company, count := range playerStocks {
		companyInfo, ok := companyInfoMap[company]
		if !ok {
			log.Printf("无法找到公司信息: %s\n", company)
			continue
		}
		totalValue += count * companyInfo.StockPrice
	}
	return totalValue
}

func getGameLogFilePath(roomID string) string {
	// 建议你在房间初始化时设置一个 startTime 或 gameID
	// 这里假设你用启动时间生成文件名
	startKey := fmt.Sprintf("room:%s:game_start_time", roomID)
	startTimeStr, err := repository.Rdb.Get(repository.Ctx, startKey).Result()
	if err != nil {
		startTimeStr = time.Now().Format("20060102_150405") // fallback
		repository.Rdb.Set(repository.Ctx, startKey, time.Now().Format("20060102_150405"), 0)
	}
	fileName := fmt.Sprintf("%s_%s.json", roomID, startTimeStr)
	return path.Join("./game_logs", fileName)
}

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
func SyncRoomMessage(conn dto.ConnInterface, room *domain.Room, pc *dto.PlayerConn, result map[string]int) error {
	rdb := repository.Rdb
	ctx := repository.Ctx

	// ------- 构造 Redis Key -------
	infoKey := fmt.Sprintf("room:%s:player:%s:info", room.ID, pc.PlayerID)
	tilesKey := fmt.Sprintf("room:%s:player:%s:tiles", room.ID, pc.PlayerID)
	currentPlayerKey := fmt.Sprintf("room:%s:currentPlayer", room.ID)
	companyIDsKey := fmt.Sprintf("room:%s:company_ids", room.ID)
	lastTileKey := fmt.Sprintf("room:%s:last_tile_key_temp", room.ID)

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
		companyKey := fmt.Sprintf("room:%s:company:%s", room.ID, companyID)
		companyCmds[companyID] = pipe2.HGetAll(ctx, companyKey)
	}

	_, err = pipe2.Exec(ctx)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("❌ 获取公司信息 pipeline 执行失败: %w", err)
	}

	companyInfo, err := data.GetCompanyInfo(rdb, room.ID)
	if err != nil {
		return fmt.Errorf("❌ 获取公司信息失败: %w", err)
	}

	roomInfo, err := data.GetRoomInfo(rdb, room.ID)
	if err != nil {
		return fmt.Errorf("❌ 获取房间信息失败: %w", err)
	}

	// ------- 其他 Redis 相关调用 -------
	tileMap, err := data.GetAllRoomTiles(rdb, room.ID)
	if err != nil {
		return fmt.Errorf("❌ 获取房间 tile 信息失败: %w", err)
	}

	merge_main_company_temp, err := data.GetMergeMainCompany(rdb, ctx, room.ID)
	if err != nil {
		return fmt.Errorf("❌ 获取合并主公司信息失败: %w", err)
	}

	merge_selection_temp, err := data.GetMergingSelection(rdb, ctx, room.ID)
	if err != nil {
		return fmt.Errorf("❌ 获取合并选择信息失败: %w", err)
	}

	mergeSettleData, err := data.GetMergeSettleData(ctx, rdb, room.ID)
	if err != nil {
		return fmt.Errorf("❌ 获取合并结算信息失败: %w", err)
	}

	stocks, err := data.GetPlayerStocks(rdb, ctx, room.ID, pc.PlayerID)
	if err != nil {
		return fmt.Errorf("❌ 获取玩家股票信息失败: %w", err)
	}

	// ------- 组装消息 -------
	playersInfo := make(map[string]interface{}, 0)
	for _, pc := range room.Players {
		playersInfo[pc.PlayerID] = map[string]interface{}{
			"playerID": pc.PlayerID,
			"online":   pc.Online,
			"ai":       pc.AI,
		}
	}

	msg := map[string]interface{}{
		"type":     "ROOM_SYNC",
		"result":   result,
		"playerId": pc.PlayerID,
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
			"players":       playersInfo,
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
	if pc.PlayerID == currentPlayer {
		WriteGameLog(room.ID, pc.PlayerID, roomInfo, msg)
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

func SyncMatchMessage(conn dto.ConnInterface, room *domain.Room, pc *dto.PlayerConn, snapshot *domain.Room) error {
	playersInfo := make([]map[string]interface{}, 0)
	for _, p := range room.Players {
		playersInfo = append(playersInfo, map[string]interface{}{
			"playerID": p.PlayerID,
			"online":   p.Online,
			"ai":       p.AI,
			"ready":    p.Ready,
		})
	}

	msg := struct {
		Type     string                   `json:"type"`
		RoomID   string                   `json:"roomID"`
		PlayerID string                   `json:"playerID"`
		Room     *domain.Room             `json:"room"`
		Players  []map[string]interface{} `json:"players"`
	}{
		Type:     "MATCH_SYNC",
		RoomID:   room.ID,
		PlayerID: pc.PlayerID,
		Room:     snapshot,
		Players:  playersInfo,
	}

	// ------- JSON 编码 -------
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("❌ 编码 JSON 失败: %w", err)
	}

	// ------- 只给当前玩家写日志（如果你有这个需求） -------
	// if playerID == roomInfo.OwnerID {
	// 	WriteGameLog(roomID, playerID, roomInfo, msg)
	// }

	// ------- 发送 WS -------
	return conn.WriteMessage(websocket.TextMessage, data)
}

// 广播消息给房间内所有连接成功的玩家
func BroadcastToRoom(room *domain.Room) {
	companyInfoMap, err := data.GetCompanyInfo(repository.Rdb, room.ID)
	if err != nil {
		log.Println("获取公司信息失败:", err)
		return
	}

	tileMap, err := data.GetAllRoomTiles(repository.Rdb, room.ID)
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

	gameShouldEnd := false
	totalTiles := 0
	allCompaniesAbove11 := true

	for company, tileCount := range allTileMap {
		totalTiles += tileCount

		if tileCount >= 41 {
			gameShouldEnd = true
			log.Printf("游戏结束：公司[%s]的 tile 数量[%d] >= 41\n", company, tileCount)
			break
		}

		if tileCount <= 11 {
			allCompaniesAbove11 = false
		}
	}

	if !gameShouldEnd && allCompaniesAbove11 && totalTiles > 54 {
		gameShouldEnd = true
		log.Printf("游戏结束：每个公司 tile 都 > 11 且 tile 被公司占用总数[%d] > 54\n", totalTiles)
	}

	if gameShouldEnd {
		roomInfo, err := data.GetRoomInfo(repository.Rdb, room.ID)
		if err != nil {
			log.Println("获取房间信息失败:", err)
		} else if roomInfo.GameStatus != dto.RoomStatusEnd {
			err = data.SetGameStatus(repository.Rdb, room.ID, dto.RoomStatusEnd)
			if err != nil {
				log.Println("设置房间状态为 end 失败:", err)
			}
		}
	}

	allStockMap := make(map[string]int)
	for _, pc := range room.Players {
		stockMap, err := data.GetPlayerStocks(repository.Rdb, repository.Ctx, room.ID, pc.PlayerID)
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

	err = data.SetCompanyInfo(repository.Rdb, room.ID, companyInfoMap)
	if err != nil {
		log.Println("❌ 设置公司信息失败:", err)
		return
	}

	result := make(map[string]int)
	for _, pc := range room.Players {
		playerStocks, err := data.GetPlayerStocks(repository.Rdb, repository.Ctx, room.ID, pc.PlayerID)
		if err != nil {
			log.Printf("❌ 获取玩家[%s]股票失败: %v\n", pc.PlayerID, err)
			continue
		}
		playerInfo, err := data.GetPlayerInfoField(repository.Rdb, repository.Ctx, room.ID, pc.PlayerID, "money")
		if err != nil {
			log.Printf("❌ 获取玩家[%s]金钱失败: %v\n", pc.PlayerID, err)
			continue
		}
		result[pc.PlayerID] = CalculateTotalValue(playerStocks, companyInfoMap) + playerInfo.Money
	}

	for _, pc := range room.Players {
		utils.Info("向玩家 %s 发送同步消息", utils.F("player_id", pc.PlayerID), utils.F("pc.AI", pc.AI))

		if pc.Online {
			// 尝试发送消息
			if err := SyncRoomMessage(pc.Conn, room, pc, result); err != nil {
				log.Println("广播失败，移除连接:", pc.PlayerID)
				pc.Conn.Close()
			}
		}
	}
}

func SnapshotRoom(r *domain.Room) *domain.Room {
	copyRoom := *r
	copyRoom.Players = make(map[string]*dto.PlayerConn)

	for _, p := range r.Players {
		cp := *p
		copyRoom.Players[p.PlayerID] = &cp
	}

	return &copyRoom
}

func BroadcastToMatch(room *domain.Room) {

	// 生成一次快照，所有人共用
	snapshot := SnapshotRoom(room)

	for _, pc := range room.Players {
		if !pc.Online {
			continue
		}

		if err := SyncMatchMessage(
			pc.Conn,
			room,
			pc,
			snapshot,
		); err != nil {
			log.Println("广播失败，关闭连接:", pc.PlayerID, err)
			pc.Conn.Close()
			pc.Online = false
		}
	}
}
