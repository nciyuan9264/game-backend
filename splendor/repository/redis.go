// redis.go
package repository

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/go-redis/redis/v8"
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
		log.Fatalf("Redis 连接失败: %v", err)
	}
	log.Println("✅ Redis 连接成功")
}
