package ws

import (
	"encoding/json"
	"fmt"
	"go-game/repository"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

// 向单个客户端发送初始化消息（初始化自己的 playerId）
func sendInitMessage(conn *websocket.Conn, playerID string) {
	msg := map[string]string{
		"type":     "init",
		"playerId": playerID,
	}
	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)
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

		// 给消息打上来源玩家的标识
		msgMap["from"] = playerID

		jsonMsg, _ := json.Marshal(msgMap)
		broadcastToRoom(roomID, jsonMsg)
	}
}

// 校验房间是否有空位，并将玩家加入房间
func validateAndJoinRoom(roomID, playerID string, conn *websocket.Conn) (bool, int) {
	roomKey := fmt.Sprintf("room:%s:info", roomID)

	// 从 Redis 获取房间最大人数
	maxPlayersStr, err := repository.Rdb.HGet(repository.Ctx, roomKey, "maxPlayers").Result()
	if err != nil {
		log.Println("获取房间人数失败:", err)
		return false, 0
	}
	maxPlayers, _ := strconv.Atoi(maxPlayersStr)

	roomLock.Lock()
	defer roomLock.Unlock()

	// 如果已满
	if len(rooms[roomID]) >= maxPlayers {
		return false, maxPlayers
	}

	// 加入房间
	rooms[roomID] = append(rooms[roomID], PlayerConn{PlayerID: playerID, Conn: conn})
	return true, maxPlayers
}

// 生成匿名玩家ID（使用 UUID）
func generateAnonymousPlayerID() string {
	return uuid.New().String()
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
	roomID := c.Query("roomId")
	if roomID == "" {
		log.Println("缺少 roomId")
		return
	}

	// 获取玩家 ID（从前端传来的 userId）
	playerID := c.Query("userId")
	if playerID == "" {
		log.Println("缺少 userId")
		conn.Close()
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

	// 向该客户端发送初始化消息
	sendInitMessage(conn, playerID)

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
