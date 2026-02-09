package data

import (
	"encoding/json"
	"fmt"
	"go-game/domain/domain"
	"go-game/dto"
	"go-game/repository"
	"log"
)

func GenerateAvailableTiles(room *domain.Room) ([]string, error) {
	ctx := repository.Ctx
	rdb := repository.Rdb

	// 获取所有 tile 的占用信息
	tileKey := fmt.Sprintf("room:%s:tiles", room.ID)
	tileMap, err := rdb.HGetAll(ctx, tileKey).Result()
	if err != nil {
		return nil, fmt.Errorf("获取 tiles 信息失败: %w", err)
	}

	playerTiles := make(map[string]struct{})
	for _, pc := range room.Players {
		tiles, err := GetPlayerTiles(rdb, ctx, room.ID, pc.PlayerID)
		if err != nil {
			log.Printf("❌ 获取玩家 %s 的 tiles 失败: %v\n", pc.PlayerID, err)
			continue
		}
		for _, tile := range tiles {
			playerTiles[tile] = struct{}{}
		}
	}

	var available []string
	for tileID, value := range tileMap {
		var tileInfo dto.Tile
		err := json.Unmarshal([]byte(value), &tileInfo)
		if err != nil {
			continue
		}
		_, exists := playerTiles[tileID]
		if exists {
			continue
		}

		if tileInfo.Belong == "" && !exists {
			available = append(available, tileID)
		}
	}

	return available, nil
}
