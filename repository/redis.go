// redis.go
package repository

import (
	"context"
	"log"

	"github.com/go-redis/redis/v8"
)

var (
	Rdb *redis.Client
	Ctx = context.Background()
)

func InitRedis() {
	Rdb = redis.NewClient(&redis.Options{
		Addr:     "192.168.3.6:6379", //"redis:6379", // Redis 地址（Docker 里用服务名或内网IP）
		Password: "",                 // 如果有密码，写在这里
		DB:       0,                  // 默认使用 0 号数据库
	})

	_, err := Rdb.Ping(Ctx).Result()
	if err != nil {
		log.Fatalf("Redis 连接失败: %v", err)
	}
	log.Println("✅ Redis 连接成功")
}
