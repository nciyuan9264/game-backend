package ws

import (
	"go-game/entities"
	"log"

	"github.com/go-redis/redis/v8"
)

func handleGameEndMessage(conn ReadWriteConn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	err := SetGameStatus(rdb, roomID, entities.RoomStatusEnd)
	if err != nil {
		log.Println("Error setting game status:", err)
		return
	}

	logPath := getGameLogFilePath(roomID)
	log.Println("✅ 游戏日志保存于:", logPath)

	BroadcastToRoom(roomID)
}
