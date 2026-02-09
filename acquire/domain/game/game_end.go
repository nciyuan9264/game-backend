package game

import (
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/dto"
	"go-game/repository"
	"log"
)

func HandleGameEndMessage(r *domain.Room, cmd domain.Command) {
	err := data.SetGameStatus(repository.Rdb, r.ID, dto.RoomStatusEnd)
	if err != nil {
		log.Println("Error setting game status:", err)
		return
	}
	logPath := getGameLogFilePath(r.ID)
	log.Println("✅ 游戏日志保存于:", logPath)
}
