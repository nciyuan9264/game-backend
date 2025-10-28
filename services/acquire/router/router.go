package router

import (
	"acquire-service/controller"
	"acquire-service/repository"
	"acquire-service/service"
	"acquire-service/ws"

	"github.com/gin-gonic/gin"
)

func InitRouter(r *gin.Engine) {
	// 初始化服务
	gameService := service.NewGameService(repository.Rdb)

	// 创建消息处理器
	messageHandler := service.NewMessageHandler(gameService)

	// 创建Hub
	hub := ws.NewHub(messageHandler)

	// 游戏接口路由
	api := r.Group("/room")
	{
		api.POST("/create", controller.CreateRoom)
		api.POST("/delete", controller.DeleteRoom)
		api.GET("/list", controller.GetRoomList)
	}

	// WebSocket 路由
	r.GET("/ws", func(c *gin.Context) {
		ws.HandleWebSocket(hub, c)
	})
}
