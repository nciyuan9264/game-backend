package data

import (
	"go-game/domain/domain"
)

func GenerateAvailableTiles(room *domain.Room) ([]string, error) {
	// 获取所有 tile 的占用信息
	playerTiles := make(map[string]struct{})
	for _, pc := range room.Connections {
		if playerInfo, ok := room.State.Players[pc.PlayerID]; ok && playerInfo != nil {
			for _, tile := range playerInfo.Tiles {
				playerTiles[tile] = struct{}{}
			}
		}
	}

	var available []string
	for tileID, value := range room.State.BoardTiles {
		_, exists := playerTiles[tileID]

		if value.Belong == "" && !exists {
			available = append(available, tileID)
		}
	}

	return available, nil
}
