package ws

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"log"
	"reflect"
	"strconv"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
)

func getTileBelong(rdb *redis.Client, roomID, tileKey string) (string, error) {
	tileMap, err := GetAllRoomTiles(repository.Rdb, roomID)
	if err != nil {
		return "", err
	}

	if err != nil {
		return "", err
	}
	return tileMap[tileKey].Belong, nil
}

// getConnectedTiles 用于从 tileKey 开始，递归查找相邻、归属一致的 tile
func getConnectedTiles(rdb *redis.Client, roomID, startTileKey string) []string {
	visited := make(map[string]bool)
	queue := []string{startTileKey}
	var connected []string

	startTileOwner, err := getTileBelong(rdb, roomID, startTileKey)
	if err != nil {
		log.Println("无法获取起始 tile 所属公司:", err)
		return connected
	}

	for len(queue) > 0 {
		tile := queue[0]
		queue = queue[1:]

		if visited[tile] {
			continue
		}
		visited[tile] = true
		connected = append(connected, tile)

		neighbors := getAdjacentTileKeys(tile)
		for _, neighbor := range neighbors {
			if visited[neighbor] {
				continue
			}
			belong, err := getTileBelong(rdb, roomID, neighbor)
			if err == nil && belong == startTileOwner {
				queue = append(queue, neighbor)
			}
		}
	}

	return connected
}

func updateRoomStatus(rdb *redis.Client, roomID string, status dto.RoomStatus) error {
	roomInfoKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	err := rdb.HSet(repository.Ctx, roomInfoKey, "status", string(status)).Err()
	if err != nil {
		log.Printf("更新房间状态失败（roomID: %s，status: %s）: %v\n", roomID, status, err)
		return err
	}
	log.Printf("房间（roomID: %s）状态已更新为：%s\n", roomID, status)
	return nil
}

// 生成匿名玩家ID（使用 UUID）
func generateAnonymousPlayerID() string {
	return uuid.New().String()
}

// 自定义 HookFunc，把字符串转换成 int
func stringToIntHookFunc() mapstructure.DecodeHookFunc {
	return func(from reflect.Kind, to reflect.Kind, data interface{}) (interface{}, error) {
		if from == reflect.String && to == reflect.Int {
			return strconv.Atoi(data.(string))
		}
		return data, nil
	}
}

// 获取房间所有 tile 信息（key 为 tileID，value 为 Tile struct）
func GetAllRoomTiles(rdb *redis.Client, roomID string) (map[string]dto.Tile, error) {
	tileMap := make(map[string]dto.Tile)

	// Redis Hash Key
	key := fmt.Sprintf("room:%s:tiles", roomID)

	// 获取 Redis Hash 所有字段
	roomTiles, err := rdb.HGetAll(repository.Ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("获取房间牌堆失败: %w", err)
	}

	// 解码每个 tile 的 JSON 字符串
	for tileID, value := range roomTiles {
		var tileInfo dto.Tile
		if err := json.Unmarshal([]byte(value), &tileInfo); err != nil {
			continue // 无效数据直接跳过
		}
		tileMap[tileID] = tileInfo
	}

	return tileMap, nil
}
