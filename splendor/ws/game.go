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
)

// 房间内的所有连接（简化版）
var Rooms = make(map[string][]dto.PlayerConn)
var roomLock sync.Mutex

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

	roomInfo, err := GetRoomInfo(roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return
	}
	if roomInfo.RoomStatus {
		SetRoomStatus(repository.Rdb, roomID, false)
	}
	BroadcastToRoom(roomID)
}

// 消息处理函数类型
type messageHandler func(conn ReadWriteConn, rdb *redis.Client, roomID, playerID string, msgMap map[string]interface{})

// 消息处理函数映射
var messageHandlers = map[string]messageHandler{
	"ready":         handleReadyMessage,
	"get_gem":       handleGetGemMessage,
	"buy_card":      handleBuyCardMessage,
	"preserve_card": handleReserveCardMessage,
	"game_end":      handleGameEndMessage,
	"play_audio":    handlePlayAudioMessage,
	"restart_game":  handleRestartGameMessage,
}

// 持续监听客户端消息，并将其广播给房间内其他玩家
type WriteOnlyConn interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// 读写接口，供真实客户端连接用，支持读取消息
type ReadWriteConn interface {
	WriteOnlyConn
	ReadMessage() (messageType int, p []byte, err error)
}

// 修改listenAndBroadcastMessages签名，接收读写接口
func listenAndBroadcastMessages(conn ReadWriteConn, roomID, playerID string) {
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
				BroadcastToRoom(roomID)
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
	playerID := c.Query("userID")
	if playerID == "" {
		log.Println("缺少 userID")
		return
	}

	// 尝试加入房间
	ok := validateAndJoinRoom(roomID, playerID, conn)
	if !ok {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"房间已满"}`))
		return
	}
	BroadcastToRoom(roomID)
	// 离开时清理资源
	defer cleanupOnDisconnect(roomID, playerID, conn)
	listenAndBroadcastMessages(conn, roomID, playerID)
}
