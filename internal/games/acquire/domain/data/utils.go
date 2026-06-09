package data

import (
	"fmt"
	"strconv"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/arrayutil"

	"golang.org/x/exp/rand"
)

// GenerateAvailableTiles 生成count个当前房间中未被任何玩家使用的 tile
func GenerateAvailableTiles(room *domain.Room, count int) ([]string, error) {
	// 1. 收集所有玩家手中的 tile
	playerTiles := make(map[string]struct{})

	for _, player := range room.State.Players {
		if player == nil {
			continue
		}
		for _, tile := range player.Tiles {
			playerTiles[tile] = struct{}{}
		}
	}

	// 2. 收集可用 tile
	available := make([]string, 0, len(room.State.BoardTiles))
	for tileID, tile := range room.State.BoardTiles {
		if tile.Belong != "" {
			continue
		}
		if _, used := playerTiles[tileID]; used {
			continue
		}
		available = append(available, tileID)
	}

	rand.Shuffle(len(available), func(i, j int) { available[i], available[j] = available[j], available[i] })
	tiles := arrayutil.SafeSlice(available, count)

	return tiles, nil
}

// GetConnectedTiles 用于从 tileKey 开始，递归查找相邻、归属一致的 tile
func GetConnectedTiles(r *domain.Room, startTileKey string) []string {
	visited := make(map[string]bool)
	queue := []string{startTileKey}
	var connected []string

	startTile := r.State.BoardTiles[startTileKey]
	startTileOwner := startTile.Belong

	for len(queue) > 0 {
		tile := queue[0]
		queue = queue[1:]

		if visited[tile] {
			continue
		}
		visited[tile] = true
		connected = append(connected, tile)

		neighbors := GetAdjacentTileKeys(tile)
		for _, neighbor := range neighbors {
			if visited[neighbor] {
				continue
			}
			tile := r.State.BoardTiles[neighbor]
			belong := tile.Belong
			if belong == startTileOwner {
				queue = append(queue, neighbor)
			}
		}
	}

	return connected
}

// getAdjacentTileKeys 用于获取指定 tileKey 的上下左右邻接的 tileKey 列表
func GetAdjacentTileKeys(tileKey string) []string {
	row := tileKey[:len(tileKey)-1] // 例如 "6"
	col := tileKey[len(tileKey)-1:] // 例如 "A"

	// 上下左右邻接逻辑
	rowNum, err := strconv.Atoi(row)
	if err != nil {
		return nil
	}

	var adjacent []string

	// 上 (row-1)
	if rowNum > 1 {
		adjacent = append(adjacent, fmt.Sprintf("%d%s", rowNum-1, col))
	}
	// 下 (row+1)
	if rowNum < 12 {
		adjacent = append(adjacent, fmt.Sprintf("%d%s", rowNum+1, col))
	}
	// 左 (col-1)
	if col[0] > 'A' {
		adjacent = append(adjacent, fmt.Sprintf("%d%s", rowNum, string(col[0]-1)))
	}
	// 右 (col+1)
	if col[0] < 'I' {
		adjacent = append(adjacent, fmt.Sprintf("%d%s", rowNum, string(col[0]+1)))
	}

	return adjacent
}

// 初始化玩家数据
func InitPlayerData(r *domain.Room, playerID string) error {
	// 1. 检查玩家数据是否已存在
	if r.State.Players[playerID] != nil {
		return fmt.Errorf("玩家数据已存在")
	}

	// 2. 随机抽取起始 Tiles（比如每人 5 个）
	tiles, err := GenerateAvailableTiles(r, 5)
	if err != nil {
		return fmt.Errorf("生成可用 tiles 失败: %w", err)
	}

	r.State.Players[playerID] = &domain.PlayerState{
		Money: 6000,
		Stocks: map[string]int{
			"Sackson":     0,
			"Tower":       0,
			"American":    0,
			"Festival":    0,
			"Worldwide":   0,
			"Continental": 0,
			"Imperial":    0,
		},
		Tiles: tiles,
	}

	return nil
}
