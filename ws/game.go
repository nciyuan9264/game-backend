package ws

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/mitchellh/mapstructure"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// 房间内的所有连接（简化版）
var rooms = make(map[string][]PlayerConn)
var roomLock sync.Mutex

// 广播消息给房间内所有连接成功的玩家
func broadcastToRoom(roomID string, message []byte) {
	roomLock.Lock()
	defer roomLock.Unlock()

	newList := []PlayerConn{}
	for _, pc := range rooms[roomID] {
		// 尝试发送消息
		if err := pc.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Println("广播失败，移除连接:", pc.PlayerID)
			pc.Conn.Close()
		} else {
			newList = append(newList, pc) // 连接正常保留
		}
	}
	// 只保留活跃连接
	rooms[roomID] = newList
}

// 玩家连接对象结构体
type PlayerConn struct {
	PlayerID string          // 玩家ID
	Conn     *websocket.Conn // 连接对象
}

// 构建一条统一格式的消息（type + data）
func buildMessage(msgType string, data map[string]interface{}) []byte {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["type"] = msgType // 加入消息类型字段
	msg, _ := json.Marshal(data)
	return msg
}

// 获取房间中玩家数量
func getRoomPlayerCount(roomID string) int {
	roomLock.Lock()
	defer roomLock.Unlock()
	return len(rooms[roomID])
}

func sendRoomMessage(roomID string, playerID string) error {
	infoKey := fmt.Sprintf("room:%s:player:%s:info", roomID, playerID)
	stocksKey := fmt.Sprintf("room:%s:player:%s:stocks", roomID, playerID)
	tilesKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)

	// 分别取
	info, err := repository.Rdb.HGetAll(repository.Ctx, infoKey).Result()
	if err != nil {
		return err
	}
	stocks, err := repository.Rdb.HGetAll(repository.Ctx, stocksKey).Result()
	if err != nil {
		return err
	}
	tiles, err := repository.Rdb.LRange(repository.Ctx, tilesKey, 0, -1).Result()
	if err != nil {
		return err
	}

	// 组装成一个结构体或 map 返回给前端
	playerData := map[string]interface{}{
		"info":   info,
		"stocks": stocks,
		"tiles":  tiles,
	}

	currentPlayer, err := repository.Rdb.Get(repository.Ctx, fmt.Sprintf("room:%s:currentPlayer", roomID)).Result()
	if err != nil {
		return fmt.Errorf("获取当前玩家失败: %w", err)
	}
	currentStep, err := repository.Rdb.Get(repository.Ctx, fmt.Sprintf("room:%s:currentStep", roomID)).Result()
	if err != nil {
		return fmt.Errorf("获取当前步骤失败: %w", err)
	}
	roomInfo, err := repository.Rdb.HGetAll(repository.Ctx, fmt.Sprintf("room:%s:roomInfo", roomID)).Result()
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %w", err)
	}
	tileMap, err := GetAllRoomTiles(repository.Rdb, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取房间 tile 信息失败: %w", err)
	}
	companyIDs, err := repository.Rdb.SMembers(repository.Ctx, fmt.Sprintf("room:%s:company_ids", roomID)).Result()
	if err != nil {
		return fmt.Errorf("获取公司ID失败: %w", err)
	}

	companyInfo := make(map[string]map[string]string) // key: companyID, value: 公司具体信息的map

	for _, companyID := range companyIDs {
		companyKey := fmt.Sprintf("room:%s:company:%s", roomID, companyID)
		data, err := repository.Rdb.HGetAll(repository.Ctx, companyKey).Result()
		if err != nil {
			return fmt.Errorf("获取公司[%s]信息失败: %w", companyID, err)
		}
		companyInfo[companyID] = data
	}
	roomData := map[string]interface{}{
		"companyInfo":   companyInfo,
		"currentPlayer": currentPlayer,
		"currentStep":   currentStep,
		"roomInfo":      roomInfo,
		"tiles":         tileMap,
	}

	msg := map[string]interface{}{
		"type":       "sync",
		"playerId":   playerID,
		"playerData": playerData, // 玩家数据
		"roomData":   roomData,   // 房间信息
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	broadcastToRoom(roomID, data)
	// err = conn.WriteMessage(websocket.TextMessage, data)
	// if err != nil {
	// 	return err
	// }
	return nil
}

// 玩家断开连接后，从房间中移除该连接
func cleanupOnDisconnect(roomID, playerID string, conn *websocket.Conn) {
	roomLock.Lock()
	defer roomLock.Unlock()

	newList := []PlayerConn{}
	for _, pc := range rooms[roomID] {
		if pc.Conn != conn {
			newList = append(newList, pc)
		}
	}
	rooms[roomID] = newList
	log.Printf("玩家 %s 离开房间 %s\n", playerID, roomID)
}

func handleSelectTileMessage(conn *websocket.Conn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	tileKey, ok := msgMap["payload"].(string)
	if !ok {
		log.Println("无效的 payload")
		return
	}
	log.Println("收到 select_tile 消息，目标 tile:", tileKey)
	redisKey := "room:" + roomID + ":tiles"
	// 取出 tile 的原始数据
	jsonStr, err := rdb.HGet(repository.Ctx, redisKey, tileKey).Result()
	if err == redis.Nil {
		log.Println("Tile 不存在:", tileKey)
		return
	} else if err != nil {
		log.Println("读取 Redis 失败:", err)
		return
	}

	// 解析为结构体
	var tile dto.Tile
	if err := json.Unmarshal([]byte(jsonStr), &tile); err != nil {
		log.Println("解析 Tile JSON 失败:", err)
		return
	}

	// 修改 belong 字段为 "blank"
	tile.Belong = "Blank"

	// 重新编码为 JSON 字符串
	modifiedJson, err := json.Marshal(tile)
	if err != nil {
		log.Println("重新编码 Tile JSON 失败:", err)
		return
	}
	// 写回 Redis
	if err := rdb.HSet(repository.Ctx, redisKey, tileKey, modifiedJson).Err(); err != nil {
		log.Println("写回 Redis 失败:", err)
		return
	}
	log.Println("已将", tileKey, "的 belong 修改为 blank 并写回 Redis")
	// 🔥 将 tile 从该玩家的 tile 列表中移除
	playerTileKey := "room:" + roomID + ":player:" + playerID + ":tiles"
	// 用 LREM 移除该 tile
	if err := rdb.LRem(repository.Ctx, playerTileKey, 1, tileKey).Err(); err != nil {
		log.Println("从玩家 tile 列表移除失败:", err)
		return
	}
	log.Println("已从玩家", playerID, "的 tile 列表中移除", tileKey)
	checkTileTriggerRules(TriggerRuleParams{Conn: conn, Rdb: repository.Rdb, RoomID: roomID, PlayerID: playerID, TileKey: tileKey})
}

func handleCreateCompanyMessage(conn *websocket.Conn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	company, ok := msgMap["payload"].(string)
	if !ok {
		log.Println("❌ 无效的 payload")
		return
	}
	log.Println("✅ 收到 create_company 消息，目标 company:", company)

	// Step 1: 取出 createTileKey
	createTileKey := fmt.Sprintf("room:%s:create_company_tile:%s", roomID, playerID)
	tileKey, err := rdb.Get(repository.Ctx, createTileKey).Result()
	if err != nil {
		log.Println("❌ 获取 createTileKey 失败:", err)
		return
	}
	log.Println("✅ 创建公司使用的 tileKey:", tileKey)

	// Step 2: 修改公司数据（仍用 Hash 类型保存）
	companyKey := fmt.Sprintf("room:%s:company:%s", roomID, company)

	// 获取公司 Hash 数据
	companyMap, err := rdb.HGetAll(repository.Ctx, companyKey).Result()
	if len(companyMap) == 0 {
		log.Println("❌ 公司 Hash 数据为空")
		return
	}

	var companyData dto.Company
	decoderConfig := &mapstructure.DecoderConfig{
		DecodeHook: stringToIntHookFunc(),
		Result:     &companyData,
		TagName:    "json",
	}
	decoder, _ := mapstructure.NewDecoder(decoderConfig)
	if err := decoder.Decode(companyMap); err != nil {
		log.Println("❌ 公司数据解析失败:", err)
		return
	}
	// 统计公司 tiles 数量
	connectedTiles := getConnectedTiles(rdb, roomID, tileKey)
	companyData.Tiles = len(connectedTiles)
	companyData.StockTotal--

	// 写回 Hash
	companyUpdateMap := map[string]interface{}{
		"tiles":      companyData.Tiles,
		"stockTotal": companyData.StockTotal,
	}

	if err := rdb.HSet(repository.Ctx, companyKey, companyUpdateMap).Err(); err != nil {
		log.Println("❌ 写回公司数据失败:", err)
		return
	}

	log.Println("✅ 公司数据已更新:", companyData)

	tileMap, err := GetAllRoomTiles(rdb, roomID)
	if err != nil {
		log.Println("❌ 获取房间所有 tile 数据失败:", err)
		return
	}

	for _, tileKey := range connectedTiles {
		tile, ok := tileMap[tileKey]
		if !ok {
			log.Printf("⚠️ tileKey %s 不存在，跳过", tileKey)
			continue
		}

		// 修改归属
		tile.Belong = company

		// 写回 Redis
		if err := UpdateTileValue(rdb, roomID, tileKey, tile); err != nil {
			log.Printf("❌ 更新 tile %s 失败: %v", tileKey, err)
		} else {
			log.Printf("✅ 成功更新 tile %s 的归属为 %s", tileKey, company)
		}
	}
	// Step 3: 增加玩家的股票数据
	playerStockKey := fmt.Sprintf("room:%s:player:%s:stocks", roomID, playerID)
	if err := rdb.HIncrBy(repository.Ctx, playerStockKey, company, 1).Err(); err != nil {
		log.Println("❌ 增加玩家股票失败:", err)
		return
	}
	log.Println("✅ 玩家获得 1 股", company, "股票")

	// Step 4: 清除 createTileKey
	_ = rdb.Del(repository.Ctx, createTileKey).Err()
	// Step 5:🔥 清除玩家的 tile
	updateRoomStatus(rdb, roomID, dto.RoomStatusBuyStock)
	// Step 6: 通知前端更新（可选）
	sendRoomMessage(roomID, playerID)
}

// 持续监听客户端消息，并将其广播给房间内其他玩家
func listenAndBroadcastMessages(roomID, playerID string, conn *websocket.Conn) {
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("读取消息失败:", err)
			break
		}

		var msgMap map[string]interface{}
		if err := json.Unmarshal(msg, &msgMap); err != nil {
			log.Println("消息解析失败:", err)
			continue
		}
		if msgType, ok := msgMap["type"].(string); ok && msgType == "select_tile" {
			handleSelectTileMessage(conn, repository.Rdb, roomID, playerID, msgMap)
		}
		if msgType, ok := msgMap["type"].(string); ok && msgType == "create_company" {
			handleCreateCompanyMessage(conn, repository.Rdb, roomID, playerID, msgMap)
		}

		// 给消息打上来源玩家的标识
		msgMap["from"] = playerID

		sendRoomMessage(roomID, playerID)

	}
}

// 将 HTTP 请求升级为 WebSocket 连接
func upgradeConnection(c *gin.Context) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket 升级失败:", err)
	}
	return conn, err
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
	// 向该客户端发送初始化消息
	sendRoomMessage(roomID, playerID)

	// 获取房间当前人数
	playerCount := getRoomPlayerCount(roomID)
	log.Printf("玩家加入 room=%s，ID=%s，当前人数=%d/%d", roomID, playerID, playerCount, maxPlayers)

	// 如果人满了，则广播开始消息
	if playerCount == maxPlayers {
		broadcastToRoom(roomID, buildMessage("start", nil))
	}

	// 进入消息监听循环
	listenAndBroadcastMessages(roomID, playerID, conn)
}
