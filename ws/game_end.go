package ws

import (
	"go-game/dto"
	"log"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

func handleGameEndMessage(conn *websocket.Conn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	err := SetGameStatus(rdb, roomID, dto.RoomStatusEnd)
	if err != nil {
		log.Println("Error setting game status:", err)
		return
	}

	broadcastToRoom(roomID)
}
