package ws

import (
	"encoding/json"
	"log"
	"splendor-service/entities"
	"splendor-service/repository"
	"strings"
	"time"
)

var _ WriteOnlyConn = (*VirtualConn)(nil) // 编译期断言实现

func chooseActionForAI(roomID, playerID string) string {

	return ""
}

func IsAIPlayer(playerID string) bool {
	return strings.HasPrefix(playerID, "ai_") // 简单策略，也可以是数据库字段
}

func MaybeRunAIIfNeeded(roomID string, data []byte) bool {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Println("❌ AI 消息格式错误:", err)
		return false
	}

	// 提取当前玩家
	roomData, ok := msg["roomData"].(map[string]interface{})
	if !ok {
		return false
	}
	currentPlayerID, ok := roomData["currentPlayer"].(string)
	if !ok || currentPlayerID == "" {
		return false
	}

	// 提取当前状态
	roomInfo, ok := roomData["roomInfo"].(map[string]interface{})
	if !ok {
		return false
	}

	gameStatusStr, ok := roomInfo["gameStatus"].(string)
	if !ok || gameStatusStr == "" {
		return false
	}
	gameStatus := entities.RoomStatus(gameStatusStr)

	// 判断是否是 AI 玩家
	if !IsAIPlayer(currentPlayerID) {
		return false
	}

	log.Printf("🤖 当前是 AI 玩家 %s 的回合，状态为 %s，准备延迟执行 AI 行动...", currentPlayerID, gameStatus)

	// ---------- 在协程中延迟执行 ----------
	go func() {
		time.Sleep(3 * time.Second)

		conn := &VirtualConn{PlayerID: currentPlayerID, RoomID: roomID}
		rdb := repository.Rdb

		var aiMsg map[string]interface{}

		switch gameStatus {
		case "playing":
			tile := chooseActionForAI(roomID, currentPlayerID)
			if tile == "" {
				log.Println("🤖 AI 未选择有效 tile")
				return
			}
			aiMsg = map[string]interface{}{
				"type":    "place_tile",
				"payload": tile,
			}
		case "end":
			aiMsg = map[string]interface{}{
				"type": "restart_game",
			}
		default:
			log.Printf("⚠️ 当前状态 %s 未定义 AI 行为", gameStatus)
			return
		}

		// 加入 playerID 然后交给 handler 执行
		aiMsg["playerID"] = currentPlayerID
		if handler, found := messageHandlers[aiMsg["type"].(string)]; found {
			log.Printf("🤖 AI [%s] 执行操作: %s", currentPlayerID, aiMsg["type"])
			handler(conn, rdb, roomID, currentPlayerID, aiMsg)
			BroadcastToRoom(roomID)
		} else {
			log.Printf("❌ AI 未找到 handler 类型: %s", aiMsg["type"])
		}
	}()

	return true
}
