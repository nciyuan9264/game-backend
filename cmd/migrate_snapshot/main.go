// Command migrate_snapshot 一次性数据迁移：把老对局的 game_events.state_snapshot（未压缩 jsonb）
// 转码为 state_blob（gzip 压缩 + 瘦身 BoardTiles），并清空 state_snapshot 以释放空间。
//
// 用法（需设置 POSTGRES_DSN 环境变量）：
//
//	go run ./cmd/migrate_snapshot            # 实际迁移
//	go run ./cmd/migrate_snapshot -dry-run   # 只统计、不写库
//	go run ./cmd/migrate_snapshot -game 51   # 只迁移指定 game_id
//
// 设计要点：state_snapshot 是大字段，读取本身较慢，因此按 game_id 分组、逐局事务处理，
// 避免一次性把全表大字段拉进内存。可重复执行（幂等）：只处理 state_blob 仍为空的行。
package main

import (
	"flag"

	acgame "github.com/nciyuan9264/game-backend/internal/games/acquire/domain/game"
	"github.com/nciyuan9264/game-backend/pkg/database"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"gorm.io/gorm"
)

func main() {
	var (
		dryRun bool
		gameID int64
	)
	flag.BoolVar(&dryRun, "dry-run", false, "只统计待迁移行数，不写库")
	flag.Int64Var(&gameID, "game", 0, "只迁移指定 game_id（0 表示全部）")
	flag.Parse()

	database.InitPostgres()
	if database.DB == nil {
		logger.Error("数据库未初始化，请设置 POSTGRES_DSN")
		return
	}
	db := database.DB

	// 确保 state_blob 列已存在。
	if err := gamehistory.NewRepo(db).AutoMigrate(); err != nil {
		logger.Error("AutoMigrate 失败", logger.F("error", err))
		return
	}

	// 找出所有"有老快照、尚未转码"的 game_id。
	var gameIDs []int64
	q := db.Model(&gamehistory.GameEvent{}).
		Where("state_snapshot IS NOT NULL AND state_blob IS NULL")
	if gameID > 0 {
		q = q.Where("game_id = ?", gameID)
	}
	if err := q.Distinct().Order("game_id ASC").Pluck("game_id", &gameIDs).Error; err != nil {
		logger.Error("查询待迁移 game_id 失败", logger.F("error", err))
		return
	}

	logger.Info("待迁移对局统计", logger.F("games", len(gameIDs)), logger.F("dry_run", dryRun))
	if dryRun || len(gameIDs) == 0 {
		return
	}

	var totalRows int
	for _, gid := range gameIDs {
		n, err := migrateGame(db, gid)
		if err != nil {
			logger.Error("迁移对局失败，跳过", logger.F("game_id", gid), logger.F("error", err))
			continue
		}
		totalRows += n
		logger.Info("对局迁移完成", logger.F("game_id", gid), logger.F("rows", n))
	}
	logger.Info("迁移结束", logger.F("games", len(gameIDs)), logger.F("rows", totalRows))
}

// migrateGame 在单个事务内迁移一局的所有待转码事件，返回转码行数。
func migrateGame(db *gorm.DB, gid int64) (int, error) {
	var rows int
	err := db.Transaction(func(tx *gorm.DB) error {
		var events []gamehistory.GameEvent
		if err := tx.Where("game_id = ? AND state_snapshot IS NOT NULL AND state_blob IS NULL", gid).
			Order("seq ASC").Find(&events).Error; err != nil {
			return err
		}
		for i := range events {
			e := events[i]
			state, result, ok := acgame.DecodeStateSnapshot(e.StateSnapshot)
			if !ok {
				logger.Warn("解码 state_snapshot 失败，跳过该行",
					logger.F("game_id", gid), logger.F("seq", e.Seq))
				continue
			}
			blob, err := acgame.EncodeStateSnapshot(state, result)
			if err != nil {
				return err
			}
			// 写入 state_blob，并清空 state_snapshot 释放空间。
			if err := tx.Model(&gamehistory.GameEvent{}).
				Where("id = ?", e.ID).
				Updates(map[string]any{
					"state_blob":     blob,
					"state_snapshot": gorm.Expr("NULL"),
				}).Error; err != nil {
				return err
			}
			rows++
		}
		return nil
	})
	return rows, err
}
