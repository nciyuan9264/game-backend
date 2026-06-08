package ws

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/roompkg"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func upgradeConnection(c *gin.Context) (*websocket.Conn, error) {
	return upgrader.Upgrade(c.Writer, c.Request, nil)
}

func readLoop(conn *websocket.Conn, room *domain.Room, playerID string) {
	defer func() {
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
	realConn := domain.NewRealConn(conn)

	roomID := c.Query("roomID")
	playerID := c.Query("userID")
	if roomID == "" || playerID == "" {
		realConn.Close()
		return
	}

	room, ok := roompkg.Rooms.Get(roomID)
	if !ok {
		realConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"房间不存在"}`))
		realConn.Close()
		return
	}

	room.Room.CmdCh <- domain.Command{
		Type:     "connect",
		PlayerID: playerID,
		Conn:     realConn,
	}

	go readLoop(realConn.Conn, room.Room, playerID)
}
