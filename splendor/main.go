package main

import (
	"go-game/repository"
	"go-game/router"
	"go-game/ws"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	repository.InitRedis()

	r := gin.Default()
	go ws.ScheduleDailyRoomReset()
	// 设置 CORS 中间件，允许所有域名、所有方法、所有 header
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://board.gamebus.online", "https://gamebus.online", "http://localhost:5173"}, // 允许你的前端域名跨域访问
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.InitRouter(r)

	r.Run(":8000")
}
