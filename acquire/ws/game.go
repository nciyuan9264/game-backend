package ws

import (
	"encoding/json"
	"go-game/domain/domain"
	"go-game/domain/roompkg"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func readLoop(conn *websocket.Conn, room *domain.Room, playerID string) {
	defer func() {
		// 断线也是 Command
		room.CmdCh <- domain.Command{
			PlayerID: playerID,
			Type:     "disconnect",
		}
		conn.Close()
	}()

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}

		// WS → Command
		room.CmdCh <- domain.Command{
			Type:     msg.Type,
			PlayerID: playerID,
			Payload:  msg.Payload,
			Conn:     conn,
		}
	}
}

func HandleWebSocket(c *gin.Context) {
	conn, err := upgradeConnection(c)
	if err != nil {
		return
	}

	// 1️⃣ 解析参数（保持不变）
	roomID := c.Query("roomID")
	playerID := c.Query("userID")
	if roomID == "" || playerID == "" {
		conn.Close()
		return
	}

	// 2️⃣ 拿房间（只读，不改）
	room := roompkg.Rooms[roomID]
	if room == nil {
		conn.WriteJSON(map[string]string{
			"type":    "error",
			"message": "ROOM_NOT_FOUND",
		})
		conn.Close()
		return
	}

	// 3️⃣ 通知房间：有人 join（Command）
	room.Room.CmdCh <- domain.Command{
		Type:     "connect",
		PlayerID: playerID,
		Conn:     conn,
	}

	// 4️⃣ 启动 WS 读循环（每个连接一个 goroutine）
	go readLoop(conn, room.Room, playerID)
}
