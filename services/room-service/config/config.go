package config

import (
	"os"
	"strconv"
)

type Config struct {
	RedisAddr string
	RedisDB   int
	GRPCPort  string
	LogLevel  string
}

func LoadConfig() *Config {
	redisDB := 0
	if dbEnv := os.Getenv("REDIS_DB"); dbEnv != "" {
		if db, err := strconv.Atoi(dbEnv); err == nil {
			redisDB = db
		}
	}

	return &Config{
		RedisAddr: getEnv("REDIS_ADDR", "localhost:6379"),
		RedisDB:   redisDB,
		GRPCPort:  getEnv("GRPC_PORT", ":9001"),
		LogLevel:  getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
