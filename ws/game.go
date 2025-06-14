package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"go-game/utils"
	"log"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"golang.org/x/exp/rand"
)

// 房间内的所有连接（简化版）
var Rooms = make(map[string][]dto.PlayerConn)
var roomLock sync.Mutex

// 广播消息给房间内所有连接成功的玩家
func broadcastToRoom(roomID string) {
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

	for _, pc := range Rooms[roomID] {
		if pc.Online {
			// 尝试发送消息
			if err := SyncRoomMessage(pc.Conn, roomID, pc.PlayerID); err != nil {
				log.Println("广播失败，移除连接:", pc.PlayerID)
				pc.Conn.Close()
			}
		}
	}
}

func SwitchToNextPlayer(rdb *redis.Client, ctx context.Context, roomID, currentID string) error {
	roomLock.Lock()
	defer roomLock.Unlock()

	players, ok := Rooms[roomID]
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

	// ------- 构造 Redis Key -------
	infoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	stocksKey := fmt.Sprintf("room:%s:player:%s:stocks", roomID, playerID)
	tilesKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	currentPlayerKey := fmt.Sprintf("room:%s:currentPlayer", roomID)
	companyIDsKey := fmt.Sprintf("room:%s:company_ids", roomID)
	lastTileKey := fmt.Sprintf("room:%s:last_tile_key_temp", roomID)

	// ------- 第一次 pipeline：玩家、房间、tile 基础数据 -------
	pipe := rdb.Pipeline()
	infoCmd := pipe.HGetAll(ctx, infoKey)
	stocksCmd := pipe.HGetAll(ctx, stocksKey)
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
	stocks := stocksCmd.Val()
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

	companyInfo := make(map[string]map[string]string)
	for companyID, cmd := range companyCmds {
		companyInfo[companyID] = cmd.Val()
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

	// ------- 组装消息 -------
	msg := map[string]interface{}{
		"type":     "sync",
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

	return conn.WriteMessage(websocket.TextMessage, data)
}

// 获取房间中玩家数量
func getRoomPlayerCount(roomID string) int {
	roomLock.Lock()
	defer roomLock.Unlock()
	return len(Rooms[roomID])
}

// 玩家断开连接后，从房间中移除该连接
func cleanupOnDisconnect(roomID, playerID string, conn *websocket.Conn) {
	roomLock.Lock()
	defer roomLock.Unlock()

	// 遍历查找玩家，并标记为离线
	for i, pc := range Rooms[roomID] {
		if pc.PlayerID == playerID {
			if pc.Conn == conn {
				Rooms[roomID][i].Online = false
				Rooms[roomID][i].Conn = nil // 连接置空，方便回收
				log.Printf("玩家 %s 标记为离线\n", playerID)
			}
			break
		}
	}

	roomInfo, err := GetRoomInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return
	}
	if roomInfo.RoomStatus {
		SetRoomStatus(repository.Rdb, roomID, false)
	}
	broadcastToRoom(roomID)
}

type messageHandler func(conn *websocket.Conn, rdb *redis.Client, roomID, playerID string, msgMap map[string]interface{})

var messageHandlers = map[string]messageHandler{
	"ready":             handleReadyMessage,
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
				broadcastToRoom(roomID)
			} else {
				log.Printf("⚠️ 未知的消息类型: %s", msgType)
			}
		}
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
	ok := validateAndJoinRoom(roomID, playerID, conn)
	if !ok {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"房间已满"}`))
		return
	}
	broadcastToRoom(roomID)
	// 离开时清理资源
	defer cleanupOnDisconnect(roomID, playerID, conn)
	listenAndBroadcastMessages(conn, roomID, playerID)
}

func handleReadyMessage(conn *websocket.Conn, rdb *redis.Client, roomID, playerID string, msgMap map[string]interface{}) {
	roomInfo, err := GetRoomInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 无法获取房间信息:", err)
		return
	}
	maxPlayers := roomInfo.MaxPlayers
	InitPlayerData(roomID, playerID)
	// 获取房间当前人数
	playerCount := getRoomPlayerCount(roomID)
	log.Printf("玩家加入 room=%s，ID=%s，当前人数=%d/%d", roomID, playerID, playerCount, maxPlayers)

	if playerCount == maxPlayers {
		err := SetRoomStatus(repository.Rdb, roomID, true)
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
			randomPlayerID := Rooms[roomID][rand.Intn(maxPlayers)]
			err := SetCurrentPlayer(repository.Rdb, repository.Ctx, roomID, randomPlayerID.PlayerID)
			if err != nil {
				log.Println("❌ 设置当前玩家失败:", err)
				return
			}
		}
	}
}
