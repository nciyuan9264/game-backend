package ws

import (
	"fmt"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"log"
	"net/http"
	"reflect"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func StringInSlice(target string, list []string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

// 将 HTTP 请求升级为 WebSocket 连接
func upgradeConnection(c *gin.Context) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket 升级失败:", err)
	}
	return conn, err
}

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

func SetGameStatus(rdb *redis.Client, roomID string, status dto.RoomStatus) error {
	roomInfoKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	err := rdb.HSet(repository.Ctx, roomInfoKey, "gameStatus", string(status)).Err()
	if err != nil {
		log.Printf("更新房间状态失败（roomID: %s，gameStatus: %s）: %v\n", roomID, status, err)
		return err
	}
	log.Printf("房间（roomID: %s）状态已更新为：%s\n", roomID, status)
	return nil
}

func SetRoomStatus(rdb *redis.Client, roomID string, status bool) error {
	roomInfoKey := fmt.Sprintf("room:%s:roomInfo", roomID)
	statusStr := strconv.FormatBool(status) // 将 bool 转为字符串 "true"/"false"

	err := rdb.HSet(repository.Ctx, roomInfoKey, "roomStatus", statusStr).Err()
	if err != nil {
		return fmt.Errorf("更新房间状态失败: %w", err)
	}
	return nil
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

// GetConn 用于根据 roomID 和 playerID 获取对应的 WebSocket 连接
func GetConn(roomID string, playerID string) (*websocket.Conn, error) {
	players, ok := Rooms[roomID]
	if !ok {
		return nil, fmt.Errorf("房间[%s]不存在", roomID)
	}
	var conn *websocket.Conn
	for _, p := range players {
		if p.PlayerID == playerID {
			conn = p.Conn
			break
		}
	}
	return conn, nil
}

// getAdjacentTileKeys 用于获取指定 tileKey 的上下左右邻接的 tileKey 列表
func getAdjacentTileKeys(tileKey string) []string {
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

func removeAtIndex(slice []string, index int) []string {
	if index < 0 || index >= len(slice) {
		return slice // 越界则不修改
	}
	return append(slice[:index], slice[index+1:]...)
}

func CalculateTotalValue(playerStocks map[string]int, companyInfoMap map[string]entities.CompanyInfo) int {
	totalValue := 0
	for company, count := range playerStocks {
		companyInfo, ok := companyInfoMap[company]
		if !ok {
			log.Printf("无法找到公司信息: %s\n", company)
			continue
		}
		totalValue += count * companyInfo.StockPrice
	}
	return totalValue
}
