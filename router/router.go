package router

import (
	"go-game/controller"
	"go-game/ws"

	"github.com/gin-gonic/gin"
)

func InitRouter(r *gin.Engine) {
	// 游戏接口路由
	api := r.Group("/room")
	{
		api.POST("/create", controller.CreateRoom)
		api.GET("/list", controller.GetRoomList) // ✅ 新增这一行
		// api.POST("/join", controller.JoinRoom)
		api.GET("/:roomID", controller.GetRoomInfo)
	}

	// WebSocket 路由
	r.GET("/ws", ws.HandleWebSocket)
}
