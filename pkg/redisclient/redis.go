package redisclient

import (
	"context"
	"fmt"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

var (
	Rdb *redis.Client
	Ctx = context.Background()
)

func InitRedis() {
	addr := os.Getenv("REDIS_ADDR")
	redisDB := 0
	if dbEnv := os.Getenv("REDIS_DB"); dbEnv != "" {
		fmt.Sscanf(dbEnv, "%d", &redisDB)
	}
	if addr == "" {
		addr = "localhost:6379"
	}
	Rdb = redis.NewClient(&redis.Options{
		Addr:     addr,    // Redis 地址（Docker 里用服务名或内网IP）
		Password: "",      // 如果有密码，写在这里
		DB:       redisDB, // 默认使用 0 号数据库
	})

	_, err := Rdb.Ping(Ctx).Result()
	if err != nil {
		logger.Error("Redis 连接失败", logger.F("error", err))
		os.Exit(1)
	}
	logger.Info("Redis 连接成功")
}
