package main

import (
	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/roompkg"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/router"
	"github.com/nciyuan9264/game-backend/pkg/httpserver"
)

func main() {
	r := httpserver.New()
	go roompkg.ScheduleDailyRoomReset()
	router.InitRouter(r)
	r.Run(":8000")
}
