package httpserver

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// DefaultAllowOrigins 默认允许的跨域前端域名
var DefaultAllowOrigins = []string{
	"https://board.gamebus.online",
	"https://gamebus.online",
	"http://localhost:5173",
}

// New 创建一个带统一 CORS 配置的 gin 引擎
func New() *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     DefaultAllowOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	return r
}
