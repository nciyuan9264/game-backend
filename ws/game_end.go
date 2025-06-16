package ws

import (
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

func handleGameEndMessage(conn *websocket.Conn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	broadcastToRoom(roomID)
}
