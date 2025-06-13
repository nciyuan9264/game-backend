package main

import (
	"go-game/repository"
	"go-game/router"

	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	repository.InitRedis()

	r := gin.Default()

	// 设置 CORS 中间件，允许所有域名、所有方法、所有 header
	r.Use(cors.New(cors.Config{
		AllowAllOrigins: true, // 允许所有来源
		AllowMethods:    []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:    []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:   []string{"Content-Length"},
		MaxAge:          12 * time.Hour,
	}))

	router.InitRouter(r)

	r.Run(":8000")
}
