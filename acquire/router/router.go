package router

import (
	"go-game/controller"
	"go-game/middleware"
	"go-game/ws"

	"github.com/gin-gonic/gin"
)

func InitRouter(r *gin.Engine) {
	authCenterURL := "https://api.gamebus.online/auth"

	api := r.Group("/room")
	api.Use(middleware.JWTMiddlewareViaAuthCenter(authCenterURL)) // 统一鉴权
	{
		api.POST("/create", controller.CreateRoom)
		// api.POST("/delete", controller.DeleteRoom)
		api.GET("/list", controller.GetRoomList)
	}

	// WebSocket 路由
	r.GET("/ws", ws.HandleWebSocket)
}
