package router

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/roompkg"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/service"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/ws"
	"github.com/nciyuan9264/game-backend/pkg/auth"
	"github.com/nciyuan9264/game-backend/pkg/historyctl"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
)

func InitRouter(r *gin.Engine) {
	authCenterURL := os.Getenv("AUTH_CENTER_URL")
	if authCenterURL == "" {
		authCenterURL = "https://api.gamebus.online/auth"
	}

	api := r.Group("/room")
	api.Use(auth.JWTMiddlewareViaAuthCenter(authCenterURL)) // 统一鉴权
	{
		api.POST("/create", roomctl.MakeCreateHandler(service.CreateRoom))
		api.GET("/list", roomctl.MakeListHandler(service.GetRoomList))
		api.GET("/game_status", roomctl.MakeStatusHandler(service.GetGameStatus))
	}

	history := r.Group("/history")
	history.Use(auth.JWTMiddlewareViaAuthCenter(authCenterURL))
	{
		var store historyctl.HistoryStore
		if roompkg.HistoryRepo != nil {
			store = roompkg.HistoryRepo
		}
		historyOpts := historyctl.Options{
			Store:           store,
			DefaultGameType: "davinci",
		}
		history.GET("/games", historyctl.MakeListGamesHandler(historyOpts))
		history.GET("/game/:id", historyctl.MakeGameDetailHandler(historyOpts))
		history.GET("/stats", historyctl.MakeStatsHandler(historyOpts))
	}

	ranking := r.Group("/ranking")
	{
		var store historyctl.HistoryStore
		if roompkg.HistoryRepo != nil {
			store = roompkg.HistoryRepo
		}
		rankingOpts := historyctl.Options{
			Store:           store,
			DefaultGameType: "davinci",
		}
		ranking.GET("/leaderboard", historyctl.MakeLeaderboardHandler(rankingOpts))
	}

	// WebSocket 路由
	r.GET("/ws", ws.HandleWebSocket)
}
