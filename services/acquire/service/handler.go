package service

// import (
// 	"acquire-service/repository"
// 	"encoding/json"
// 	"log"

// 	"github.com/gin-gonic/gin"
// 	"github.com/go-redis/redis/v8"
// 	"github.com/gorilla/websocket"
// )

// // 消息处理函数类型
// type messageHandler func(conn ReadWriteConn, rdb *redis.Client, roomID, playerID string, msgMap map[string]interface{})

// // 消息处理函数映射
// var messageHandlers = map[string]messageHandler{
// 	"ready":             handleReadyMessage,
// 	"place_tile":        handlePlaceTileMessage,
// 	"create_company":    handleCreateCompanyMessage,
// 	"merging_settle":    handleMergingSettleMessage,
// 	"buy_stock":         handleBuyStockMessage,
// 	"merging_selection": handleMergingSelectionMessage,
// 	"game_end":          handleGameEndMessage,
// 	"play_audio":        handlePlayAudioMessage,
// 	"restart_game":      handleRestartGameMessage,
// }

// // 持续监听客户端消息，并将其广播给房间内其他玩家
type WriteOnlyConn interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// // 读写接口，供真实客户端连接用，支持读取消息
type ReadWriteConn interface {
	WriteOnlyConn
	ReadMessage() (messageType int, p []byte, err error)
}

// // 修改listenAndBroadcastMessages签名，接收读写接口
// func listenAndBroadcastMessages(conn ReadWriteConn, roomID, playerID string) {
// 	for {
// 		_, msg, err := conn.ReadMessage()
// 		if err != nil {
// 			log.Println("读取消息失败:", err)
// 			break
// 		}
// 		msgMap := make(map[string]interface{})
// 		msgMap["playerID"] = playerID
// 		if err := json.Unmarshal(msg, &msgMap); err != nil {
// 			log.Println("消息解析失败:", err)
// 			continue
// 		}
// 		if msgType, ok := msgMap["type"].(string); ok {
// 			if handler, found := messageHandlers[msgType]; found {
// 				handler(conn, repository.Rdb, roomID, playerID, msgMap)
// 				BroadcastToRoom(roomID)
// 			} else {
// 				log.Printf("⚠️ 未知的消息类型: %s", msgType)
// 			}
// 		}
// 	}
// }

// // WebSocket 主入口（处理每个连接）
// func HandleWebSocket(c *gin.Context) {
// 	conn, err := upgradeConnection(c)
// 	if err != nil {
// 		return
// 	}
// 	defer conn.Close()

// 	// 获取房间 ID
// 	roomID := c.Query("roomID")
// 	if roomID == "" {
// 		log.Println("缺少 roomID")
// 		return
// 	}
// 	// 获取玩家 ID（从前端传来的 userId）
// 	playerID := c.Query("userID")
// 	if playerID == "" {
// 		log.Println("缺少 userID")
// 		return
// 	}

// 	// 尝试加入房间
// 	ok := validateAndJoinRoom(roomID, playerID, conn)
// 	if !ok {
// 		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"房间已满"}`))
// 		return
// 	}
// 	BroadcastToRoom(roomID)
// 	// 离开时清理资源
// 	defer cleanupOnDisconnect(roomID, playerID, conn)
// 	listenAndBroadcastMessages(conn, roomID, playerID)
// }
