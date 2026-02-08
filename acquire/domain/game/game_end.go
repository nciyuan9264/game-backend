package game

import (
	"go-game/domain/data"
	"go-game/domain/room"
	"go-game/dto"
	"log"

	"github.com/go-redis/redis/v8"
)

func HandleGameEndMessage(conn room.ReadWriteConn, rdb *redis.Client, room *dto.Room, playerID string, msgMap map[string]interface{}) {
	err := data.SetGameStatus(rdb, room.ID, dto.RoomStatusEnd)
	if err != nil {
		log.Println("Error setting game status:", err)
		return
	}
	logPath := getGameLogFilePath(room.ID)
	log.Println("✅ 游戏日志保存于:", logPath)

	// 暂时注释掉，需要找到 BroadcastToRoom 函数的定义
	BroadcastToRoom(room.ID)
}
