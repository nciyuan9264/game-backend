package game

import (
	"context"
	"fmt"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/data"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/logger"

	"github.com/go-redis/redis/v8"
)

func GiveRandomTileToPlayer(rdb *redis.Client, ctx context.Context, r *domain.Room, playerID string) error {
	selected, err := data.GenerateAvailableTiles(r, 1)
	if err != nil {
		return fmt.Errorf("生成可用 tiles 失败: %w", err)
	}

	if len(selected) == 0 {
		logger.Warn("没有可用的 tiles")
		return nil
	}

	// 使用 SafeSlice 安全获取一张 tile
	if len(selected) == 0 {
		return fmt.Errorf("无法为玩家分配 tile")
	}

	// 添加到玩家 tiles 中
	r.State.Players[playerID].Tiles = append(r.State.Players[playerID].Tiles, selected[0])

	logger.Info("玩家获得 tile", logger.F("room_id", r.ID), logger.F("player_id", playerID), logger.F("tile", selected[0]))
	return nil
}
