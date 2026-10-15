package router

import (
	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/loadtest"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/service"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/ws"
	"github.com/nciyuan9264/game-backend/pkg/auth"
	"github.com/nciyuan9264/game-backend/pkg/historyctl"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
)

func InitRouter(r *gin.Engine) {
	authCenterURL := auth.CenterURL()

	profileAPI := r.Group("/room")
	profileAPI.Use(auth.AuthProfileMiddlewareViaAuthCenter(authCenterURL)) // 创建房间需要姓名和头像做展示。
	{
		profileAPI.POST("/create", createRoom)
	}

	api := r.Group("/room")
	api.Use(auth.JWTMiddlewareViaAuthCenter(authCenterURL))
	{
		api.GET("/lookup", lookupRoom)
		api.GET("/list", listRooms)
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
			DefaultGameType: "acquire",
			AllowSnapshot:   true,
			SnapshotHandler: service.HistoryGameSnapshot,
		}
		history.GET("/games", historyctl.MakeListGamesHandler(historyOpts))
		history.GET("/game/:id", historyctl.MakeGameDetailHandler(historyOpts))
		history.GET("/game/:id/snapshot", historyctl.MakeSnapshotHandler(historyOpts))
		history.GET("/game/:id/snapshots", service.HistoryGameSnapshots)
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
			DefaultGameType: "acquire",
		}
		ranking.GET("/leaderboard", historyctl.MakeLeaderboardHandler(rankingOpts))
	}

	// WebSocket 路由
	r.GET("/ws", auth.AuthProfileMiddlewareViaAuthCenter(authCenterURL), ws.HandleWebSocket)

	if loadtest.Enabled() {
		loadtest.RegisterRoutes(r)
	}
}
