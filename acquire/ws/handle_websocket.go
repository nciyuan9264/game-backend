package ws

import (
	"encoding/json"
	"go-game/domain/domain"
	"go-game/domain/roompkg"
	"go-game/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// 将 HTTP 请求升级为 WebSocket 连接
func upgradeConnection(c *gin.Context) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		utils.Error("WebSocket 升级失败", utils.F("error", err))
	}
	return conn, err
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
			Payload:  msg.Payload,
			PlayerID: playerID,
			Conn:     conn,
		}
	}
}

func HandleWebSocket(c *gin.Context) {
	conn, err := upgradeConnection(c)
	if err != nil {
		return
	}

	// 创建 RealConn 实例
	realConn := &domain.RealConn{Conn: conn}

	// 1️⃣ 解析参数（保持不变）
	roomID := c.Query("roomID")
	playerID := c.Query("userID")
	if roomID == "" || playerID == "" {
		realConn.Close()
		return
	}

	// 2️⃣ 拿房间（只读，不改）
	room := roompkg.Rooms[roomID]
	if room == nil {
		realConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"房间不存在"}`))
		realConn.Close()
		return
	}

	// 3️⃣ 通知房间：有人 join（Command）
	room.Room.CmdCh <- domain.Command{
		Type:     "connect",
		PlayerID: playerID,
		Conn:     realConn,
	}

	// 4️⃣ 启动 WS 读循环（每个连接一个 goroutine）
	go readLoop(realConn.Conn, room.Room, playerID)
}
