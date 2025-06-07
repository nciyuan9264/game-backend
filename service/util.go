package service

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"
)

func JsonMarshal(v interface{}) (string, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func IsRoomFull(roomID string) bool {
	count, err := repository.Rdb.SCard(repository.Ctx, "room:"+roomID+":players").Result()
	if err != nil {
		return false // 或者根据实际情况处理错误
	}
	return count >= 2
}

func FindConnectedTiles(startRow string, startCol int, tiles map[string]map[int]*dto.Tile, company string) []string {
	visited := make(map[string]bool)
	queue := []struct {
		row string
		col int
	}{{startRow, startCol}}

	result := []string{}

	directions := []struct {
		dr int
		dc int
	}{
		{0, 1}, {0, -1}, // 左右
		{1, 0}, {-1, 0}, // 上下
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		tileID := fmt.Sprintf("%d%s", current.col, current.row)
		if visited[tileID] {
			continue
		}
		visited[tileID] = true

		tile, exists := tiles[current.row][current.col]
		if !exists || tile.Belong != company {
			continue
		}

		result = append(result, tile.ID)

		for _, d := range directions {
			newRow := string([]rune(current.row)[0] + rune(d.dr))
			newCol := current.col + d.dc
			if newCol >= 1 && newCol <= 12 && newRow >= "A" && newRow <= "I" {
				queue = append(queue, struct {
					row string
					col int
				}{newRow, newCol})
			}
		}
	}

	return result
}
