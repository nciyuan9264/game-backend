package router

import (
	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/controller"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/service"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/ws"
	"github.com/nciyuan9264/game-backend/pkg/auth"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
)

func InitRouter(r *gin.Engine) {
	authCenterURL := "https://api.gamebus.online/auth"

	api := r.Group("/room")
	api.Use(auth.JWTMiddlewareViaAuthCenter(authCenterURL)) // 统一鉴权
	{
		api.POST("/create", roomctl.MakeCreateHandler(service.CreateRoom))
		api.POST("/delete", controller.DeleteRoom)
		api.GET("/list", roomctl.MakeListHandler(service.GetRoomList))
		api.GET("/game_status", roomctl.MakeStatusHandler(service.GetGameStatus))
	}

	// WebSocket 路由
	r.GET("/ws", ws.HandleWebSocket)
}
