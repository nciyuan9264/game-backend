package game

import (
	"context"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/utils"

	"github.com/go-redis/redis/v8"
)

func GiveRandomTileToPlayer(rdb *redis.Client, ctx context.Context, r *domain.Room, playerID string) error {
	selected, err := data.GenerateAvailableTiles(r, 1)
	if err != nil {
		return fmt.Errorf("生成可用 tiles 失败: %w", err)
	}

	if len(selected) == 0 {
		utils.Warn("没有可用的 tiles")
		return nil
	}

	// 使用 SafeSlice 安全获取一张 tile
	if len(selected) == 0 {
		return fmt.Errorf("无法为玩家分配 tile")
	}

	// 添加到玩家 tiles 中
	r.State.Players[playerID].Tiles = append(r.State.Players[playerID].Tiles, selected[0])

	utils.Info("玩家获得 tile", utils.F("room_id", r.ID), utils.F("player_id", playerID), utils.F("tile", selected[0]))
	return nil
}
