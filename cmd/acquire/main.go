package main

import (
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/router"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/service"
	"github.com/nciyuan9264/game-backend/pkg/database"
	"github.com/nciyuan9264/game-backend/pkg/httpserver"
)

func main() {
	database.InitPostgres()
	roompkg.InitHistoryRepo()

	r := httpserver.New()
	go service.ScheduleDailyRoomReset()

	router.InitRouter(r)

	r.Run(":8000")
}
