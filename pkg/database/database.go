package database

import (
	"os"
	"time"

	"github.com/nciyuan9264/game-backend/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB 是全局 GORM 句柄。InitPostgres 之后才可用。
var DB *gorm.DB

// InitPostgres 从环境变量 POSTGRES_DSN 读取连接串并初始化全局 *gorm.DB。
// 失败直接 os.Exit(1)。
func InitPostgres() {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		logger.Error("POSTGRES_DSN is empty, skip postgres init")
		return
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		logger.Error("Postgres 连接失败", logger.F("error", err))
		os.Exit(1)
	}

	sqlDB, err := db.DB()
	if err != nil {
		logger.Error("获取 *sql.DB 失败", logger.F("error", err))
		os.Exit(1)
	}
	sqlDB.SetMaxIdleConns(8)
	sqlDB.SetMaxOpenConns(64)
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = db
	logger.Info("Postgres 连接成功")
}
