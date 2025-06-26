package ws

import (
	"fmt"
	"go-game/dto"
	"go-game/repository"
	"log"
	"net/http"
	"path"
	"reflect"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
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
func GetConn(roomID string, playerID string) (dto.ConnInterface, error) {
	players, ok := Rooms[roomID]
	if !ok {
		return nil, fmt.Errorf("房间[%s]不存在", roomID)
	}
	var conn dto.ConnInterface
	for _, p := range players {
		if p.PlayerID == playerID {
			conn = p.Conn
			break
		}
	}
	return conn, nil
}

func getGameLogFilePath(roomID string) string {
	// 建议你在房间初始化时设置一个 startTime 或 gameID
	// 这里假设你用启动时间生成文件名
	startKey := fmt.Sprintf("room:%s:game_start_time", roomID)
	startTimeStr, err := repository.Rdb.Get(repository.Ctx, startKey).Result()
	if err != nil {
		startTimeStr = time.Now().Format("20060102_150405") // fallback
		repository.Rdb.Set(repository.Ctx, startKey, time.Now().Format("20060102_150405"), 0)
	}
	fileName := fmt.Sprintf("%s_%s.json", roomID, startTimeStr)
	return path.Join("./game_logs", fileName)
}
