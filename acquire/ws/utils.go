package ws

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// 将 HTTP 请求升级为 WebSocket 连接
func upgradeConnection(c *gin.Context) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket 升级失败:", err)
	}
	return conn, err
}

// GetConn 用于根据 roomID 和 playerID 获取对应的 WebSocket 连接
// func GetConn(roomID string, playerID string) (dto.ConnInterface, error) {
// 	players := room.Rooms[roomID].Players
// 	var conn dto.ConnInterface
// 	for _, p := range players {
// 		if p.PlayerID == playerID {
// 			conn = p.Conn
// 			break
// 		}
// 	}
// 	return conn, nil
// }
