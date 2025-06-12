package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"log"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"golang.org/x/exp/rand"
)

// 房间内的所有连接（简化版）
var rooms = make(map[string][]dto.PlayerConn)
var roomLock sync.Mutex

// 广播消息给房间内所有连接成功的玩家
func broadcastToRoom(roomID string) {
	roomLock.Lock()
	defer roomLock.Unlock()

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
	for _, pc := range rooms[roomID] {
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
		stockLeft := 25 - allStockMap[companyName]
		info.StockTotal = stockLeft
		info.Tiles = allTileMap[companyName]
		companyInfoMap[companyName] = info // 注意：结构体是值传递，需要再赋值回去！
	}

	err = SetCompanyInfo(repository.Rdb, roomID, companyInfoMap)
	if err != nil {
		log.Println("❌ 设置公司信息失败:", err)
		return
	}

	newList := []dto.PlayerConn{}
	for _, pc := range rooms[roomID] {
		// 尝试发送消息
		if err := SyncRoomMessage(pc.Conn, roomID, pc.PlayerID); err != nil {
			log.Println("广播失败，移除连接:", pc.PlayerID)
			pc.Conn.Close()
		} else {
			newList = append(newList, pc) // 连接正常保留
		}
	}
	// 只保留活跃连接
	rooms[roomID] = newList
}

func SwitchToNextPlayer(rdb *redis.Client, ctx context.Context, roomID, currentID string) error {
	roomLock.Lock()
	defer roomLock.Unlock()

	players, ok := rooms[roomID]
	if !ok || len(players) == 0 {
		return fmt.Errorf("房间 %s 没有玩家", roomID)
	}

	// 找到当前玩家索引
	var currentIndex int = -1
	for i, pc := range players {
		if pc.PlayerID == currentID {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		return fmt.Errorf("未找到当前玩家 %s", currentID)
	}

	// 下一个玩家索引（循环）
	nextIndex := (currentIndex + 1) % len(players)
	nextPlayerID := players[nextIndex].PlayerID

	// 设置当前玩家
	if err := SetCurrentPlayer(rdb, ctx, roomID, nextPlayerID); err != nil {
		return fmt.Errorf("切换当前玩家失败: %w", err)
	}

	log.Printf("✅ 已将当前玩家切换为: %s\n", nextPlayerID)
	return nil
}

// 向该客户端发送同步消息
func SyncRoomMessage(conn *websocket.Conn, roomID string, playerID string) error {
	rdb := repository.Rdb
	ctx := repository.Ctx

	// 构建 key
	infoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	stocksKey := fmt.Sprintf("room:%s:player:%s:stocks", roomID, playerID)
	tilesKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	currentPlayerKey := fmt.Sprintf("room:%s:currentPlayer", roomID)
	currentStepKey := fmt.Sprintf("room:%s:currentStep", roomID)
	roomInfoKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	companyIDsKey := fmt.Sprintf("room:%s:company_ids", roomID)

	// pipeline 批量读取
	pipe := rdb.Pipeline()

	infoCmd := pipe.HGetAll(ctx, infoKey)
	stocksCmd := pipe.HGetAll(ctx, stocksKey)
	tilesCmd := pipe.LRange(ctx, tilesKey, 0, -1)
	currentPlayerCmd := pipe.Get(ctx, currentPlayerKey)
	currentStepCmd := pipe.Get(ctx, currentStepKey)
	roomInfoCmd := pipe.HGetAll(ctx, roomInfoKey)
	companyIDsCmd := pipe.SMembers(ctx, companyIDsKey)

	// 新增的 key 相关
	dividendKey := fmt.Sprintf("room:%s:merge_bonus_temp", roomID)
	clearPlayerKey := fmt.Sprintf("room:%s:merge_clear_players_temp", roomID)
	mainHotelNameKey := fmt.Sprintf("room:%s:merge_main_hotel_name_temp", roomID)
	createTileKey := fmt.Sprintf("room:%s:create_company_tile_temp", roomID)

	// pipeline 增加对应命令
	dividendCmd := pipe.Get(ctx, dividendKey)
	clearPlayerCmd := pipe.Get(ctx, clearPlayerKey)
	mainHotelNameCmd := pipe.Get(ctx, mainHotelNameKey)
	createTileCmd := pipe.Get(ctx, createTileKey)

	// 执行 pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("pipeline 执行失败: %w", err)
	}

	// 提取结果
	info := infoCmd.Val()
	stocks := stocksCmd.Val()
	tiles := tilesCmd.Val()
	currentPlayer := currentPlayerCmd.Val()
	currentStep := currentStepCmd.Val()
	roomInfo := roomInfoCmd.Val()
	companyIDs := companyIDsCmd.Val()

	// 获取公司信息也使用 pipeline 批量读取
	companyInfo := make(map[string]map[string]string)
	pipe2 := rdb.Pipeline()
	companyCmds := make(map[string]*redis.StringStringMapCmd)

	for _, companyID := range companyIDs {
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, companyID)
		cmd := pipe2.HGetAll(ctx, companyKey)
		companyCmds[companyID] = cmd
	}
	_, err = pipe2.Exec(ctx)
	if err != nil {
		return fmt.Errorf("pipeline 获取公司信息失败: %w", err)
	}

	for companyID, cmd := range companyCmds {
		companyInfo[companyID] = cmd.Val()
	}

	// 获取房间 tile
	tileMap, err := GetAllRoomTiles(rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取房间 tile 信息失败: %w", err)
	}

	// 组装数据
	playerData := map[string]interface{}{
		"info":   info,
		"stocks": stocks,
		"tiles":  tiles,
	}
	// 执行后取出
	dividend := dividendCmd.Val()
	clearPlayer := clearPlayerCmd.Val()
	mainHotelName := mainHotelNameCmd.Val()
	createTile := createTileCmd.Val()

	merge_other_companies_temp, err := GetMergeOtherCompanies(rdb, ctx, roomID)
	if err != nil {
		return fmt.Errorf("获取合并其他公司信息失败: %w", err)
	}
	merge_main_company_temp, err := GetMergeMainCompany(rdb, ctx, roomID)
	if err != nil {
		return fmt.Errorf("获取合并主公司信息失败: %w", err)
	}
	merge_settle_player_temp, err := GetPlayerNeedSettle(rdb, ctx, roomID)
	if err != nil {
		return fmt.Errorf("获取合并玩家信息失败: %w", err)
	}

	merge_selection_temp, err := GetMergingSelection(rdb, ctx, roomID)
	if err != nil {
		return fmt.Errorf("获取合并选择信息失败: %w", err)
	}

	mergeSettleData, err := GetMergeSettleData(repository.Ctx, rdb, roomID)
	if err != nil {
		return fmt.Errorf("获取合并数据失败: %w", err)
	}

	roomData := map[string]interface{}{
		"companyInfo":                companyInfo,
		"currentPlayer":              currentPlayer,
		"currentStep":                currentStep,
		"roomInfo":                   roomInfo,
		"tiles":                      tileMap,
		"merge_other_companies_temp": merge_other_companies_temp,
		"merge_main_company_temp":    merge_main_company_temp,
		"merge_settle_player_temp":   merge_settle_player_temp,
		"merge_selection_temp":       merge_selection_temp,
		"mergeSettleData":            mergeSettleData,
	}

	// 执行 pipeline
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("pipeline 执行失败: %w", err)
	}
	// 添加到返回消息中
	msg := map[string]interface{}{
		"type":       "sync",
		"playerId":   playerID,
		"playerData": playerData,
		"roomData":   roomData,
		"temp": map[string]interface{}{
			"merge_bonus":         dividend,
			"merge_clear_players": clearPlayer,
			"merge_main_hotel":    mainHotelName,
			"create_company_tile": createTile,
		},
	}

	// 编码并发送
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// 获取房间中玩家数量
func getRoomPlayerCount(roomID string) int {
	roomLock.Lock()
	defer roomLock.Unlock()
	return len(rooms[roomID])
}

// 玩家断开连接后，从房间中移除该连接
func cleanupOnDisconnect(roomID, playerID string, conn *websocket.Conn) {
	roomLock.Lock()
	defer roomLock.Unlock()

	newList := []dto.PlayerConn{}
	for _, pc := range rooms[roomID] {
		if pc.Conn != conn && pc.PlayerID != playerID {
			newList = append(newList, pc)
		}
	}
	rooms[roomID] = newList
	roomInfo, err := GetRoomInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return
	}
	if roomInfo.RoomStatus {
		SetRoomStatus(repository.Rdb, roomID, false)
	}
	log.Printf("玩家 %s 离开房间 %s\n", playerID, roomID)
}

type messageHandler func(conn *websocket.Conn, rdb *redis.Client, roomID, playerID string, msgMap map[string]interface{})

var messageHandlers = map[string]messageHandler{
	"place_tile":        handlePlaceTileMessage,
	"create_company":    handleCreateCompanyMessage,
	"merging_settle":    handleMergingSettleMessage,
	"buy_stock":         handleBuyStockMessage,
	"merging_selection": handleMergingSelectionMessage,
}

// 持续监听客户端消息，并将其广播给房间内其他玩家
func listenAndBroadcastMessages(conn *websocket.Conn, roomID, playerID string) {
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("读取消息失败:", err)
			break
		}
		msgMap := make(map[string]interface{})
		msgMap["playerID"] = playerID
		if err := json.Unmarshal(msg, &msgMap); err != nil {
			log.Println("消息解析失败:", err)
			continue
		}
		if msgType, ok := msgMap["type"].(string); ok {
			if handler, found := messageHandlers[msgType]; found {
				handler(conn, repository.Rdb, roomID, playerID, msgMap)
			} else {
				log.Printf("⚠️ 未知的消息类型: %s", msgType)
			}
		}
		broadcastToRoom(roomID)
	}
}

// WebSocket 主入口（处理每个连接）
func HandleWebSocket(c *gin.Context) {
	conn, err := upgradeConnection(c)
	if err != nil {
		return
	}
	defer conn.Close()

	// 获取房间 ID
	roomID := c.Query("roomID")
	if roomID == "" {
		log.Println("缺少 roomID")
		return
	}

	// 获取玩家 ID（从前端传来的 userId）
	playerID := c.Query("userId")
	if playerID == "" {
		log.Println("缺少 userId")
		return
	}

	// 尝试加入房间
	ok, maxPlayers := validateAndJoinRoom(roomID, playerID, conn)
	if !ok {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"房间已满"}`))
		return
	}
	// 离开时清理资源
	defer cleanupOnDisconnect(roomID, playerID, conn)

	InitPlayerData(roomID, playerID)

	// 获取房间当前人数
	playerCount := getRoomPlayerCount(roomID)
	log.Printf("玩家加入 room=%s，ID=%s，当前人数=%d/%d", roomID, playerID, playerCount, maxPlayers)

	if playerCount == maxPlayers {
		err = SetRoomStatus(repository.Rdb, roomID, true)
		if err != nil {
			log.Println("❌ 设置房间状态失败:", err)
			return
		}

		playerID, err := GetCurrentPlayer(repository.Rdb, repository.Ctx, roomID)
		if err != nil {
			log.Println("❌ 获取当前玩家失败:", err)
			return
		}
		if playerID == "" {
			randomPlayerID := rooms[roomID][rand.Intn(maxPlayers)]
			err := SetCurrentPlayer(repository.Rdb, repository.Ctx, roomID, randomPlayerID.PlayerID)
			if err != nil {
				log.Println("❌ 设置当前玩家失败:", err)
				return
			}
		}
	}
	broadcastToRoom(roomID)
	listenAndBroadcastMessages(conn, roomID, playerID)
}
