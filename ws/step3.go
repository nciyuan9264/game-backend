package ws

import (
	"context"
	"fmt"
	"go-game/utils"
	"log"
	"math/rand/v2"

	"github.com/go-redis/redis/v8"
)

func GiveRandomTileToPlayer(rdb *redis.Client, ctx context.Context, roomID, playerID string) error {
	allTiles, err := generateAvailableTiles(roomID)
	if err != nil {
		return fmt.Errorf("生成可用 tiles 失败: %w", err)
	}

	if len(allTiles) == 0 {
		log.Println("❌ 没有可用的 tiles")
		return nil
	}

	rand.Shuffle(len(allTiles), func(i, j int) {
		allTiles[i], allTiles[j] = allTiles[j], allTiles[i]
	})

	// 使用 SafeSlice 安全获取一张 tile
	selected := utils.SafeSlice(allTiles, 1)
	if len(selected) == 0 {
		return fmt.Errorf("无法为玩家分配 tile")
	}

	// 添加到玩家 tiles 中
	if err := AddPlayerTile(rdb, ctx, roomID, playerID, selected[0]); err != nil {
		return fmt.Errorf("添加 tile 失败: %w", err)
	}

	log.Printf("✅ 玩家 %s 获得 tile：%s\n", playerID, selected[0])
	return nil
}
