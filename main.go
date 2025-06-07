package main

import (
	"go-game/repository"
	"go-game/router"

	"github.com/gin-gonic/gin"
)

func main() {
	repository.InitRedis()
	r := gin.Default()
	router.InitRouter(r)
	r.Run(":8000")
}
