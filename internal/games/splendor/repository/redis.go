// redis.go
package repository

import (
	"github.com/go-redis/redis/v8"
	"github.com/nciyuan9264/game-backend/pkg/redisclient"
)

// Rdb 复用 pkg/redisclient 的全局 Redis 客户端
var Rdb *redis.Client

// Ctx 复用 pkg/redisclient 的全局 Context
var Ctx = redisclient.Ctx

// InitRedis 调用 pkg/redisclient.InitRedis 并同步全局变量
func InitRedis() {
	redisclient.InitRedis()
	Rdb = redisclient.Rdb
}
