package ws

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
)

// RemovePlayerTile 从指定玩家的 tile 列表中移除某个 tile
func RemovePlayerTile(rdb *redis.Client, ctx context.Context, roomID, playerID, tileKey string) error {
	playerTileKey := fmt.Sprintf("room:%s:player:%s:tiles", roomID, playerID)
	if err := rdb.LRem(ctx, playerTileKey, 1, tileKey).Err(); err != nil {
		return fmt.Errorf("从玩家 %s 的 tile 列表移除失败: %w", playerID, err)
	}
	return nil
}
