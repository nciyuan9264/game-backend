package main

import (
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/roompkg"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/router"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/service"
	"github.com/nciyuan9264/game-backend/pkg/database"
	"github.com/nciyuan9264/game-backend/pkg/httpserver"
)

func main() {
	database.InitPostgres()
	roompkg.InitHistoryRepo()

	r := httpserver.New()
	go service.ScheduleWeeklyRoomReset()

	router.InitRouter(r)

	r.Run(":8000")
}
