// Package roomctl 提供跨游戏复用的房间相关 HTTP Handler 工厂。
//
// 各游戏的 service 层只需暴露相同签名的函数，即可由本包生成统一形态的 gin.HandlerFunc，
// 从而消除 controller 层的样板代码。
package roomctl

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 默认占位用户信息（在 auth 中间件未启用时使用，便于本地联调）
const (
	defaultPlaceholderID   = "3"
	defaultPlaceholderName = "wzy"
)

// MaxOwnedRooms 同一玩家最多同时持有的房间数。
const MaxOwnedRooms = 5

// ErrTooManyRooms 业务级错误：同一玩家持有的房间数已达 MaxOwnedRooms。
// service 层用 `return "", roomctl.ErrTooManyRooms` 即可，由 MakeCreateHandler 翻译为 409。
var ErrTooManyRooms = errors.New("too many rooms for this user")

// CountOwnedRooms 在给定房间快照里数 ownerID == userID 的房间数。
// snapshot：从 Registry.Snapshot() 拿到的 map[string]T。
// ownerOf：一个回调，用于从 T 取出 OwnerID。
func CountOwnedRooms[T any](snapshot map[string]T, ownerOf func(T) string, userID string) int {
	n := 0
	for _, rs := range snapshot {
		if ownerOf(rs) == userID {
			n++
		}
	}
	return n
}

// resolveUserID 从 gin.Context 中解析 userID。
// 优先取 auth 中间件注入的 user_id / name；缺失时回退到占位值，方便本地开发。
func resolveUserID(c *gin.Context) string {
	var id any = defaultPlaceholderID
	var name any = defaultPlaceholderName
	if v, ok := c.Get("user_id"); ok {
		id = v
	}
	if v, ok := c.Get("name"); ok {
		name = v
	}
	return fmt.Sprintf("%v-%v", name, id)
}

// CreateRoomService 创建房间的业务函数签名。
type CreateRoomService func(userID string) (string, error)

// MakeCreateHandler 生成统一的 POST /room/create 处理器。
// 响应：
//   - 成功：200 + { status_code, message, data: { roomID } }
//   - ErrTooManyRooms：409 + { status_code, error: "too_many_rooms", message }
//   - 其他错误：500 + { error }
func MakeCreateHandler(svc CreateRoomService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := resolveUserID(c)
		roomID, err := svc(userID)
		if err != nil {
			if errors.Is(err, ErrTooManyRooms) {
				c.JSON(http.StatusConflict, gin.H{
					"status_code": http.StatusConflict,
					"error":       "too_many_rooms",
					"message":     "你最多只能同时拥有 5 个房间",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status_code": http.StatusOK,
			"message":     "房间创建成功",
			"data":        gin.H{"roomID": roomID},
		})
	}
}

// MakeListHandler 生成统一的 GET /room/list 处理器，泛型化以适配各游戏的 RoomInfo 类型。
// 响应固定为 { status_code, message, data: { rooms } }。
func MakeListHandler[T any](svc func() ([]T, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		rooms, err := svc()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "获取房间列表失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status_code": http.StatusOK,
			"message":     "获取成功",
			"data":        gin.H{"rooms": rooms},
		})
	}
}

// MakeStatusHandler 生成统一的 GET /room/game_status 处理器，泛型化以适配各游戏的状态类型。
// svc 返回 nil 视为房间不存在。
func MakeStatusHandler[T any](svc func(roomID string) *T) gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID := c.Query("room_id")
		if roomID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "room_id is required"})
			return
		}
		status := svc(roomID)
		if status == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "房间不存在"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status_code": http.StatusOK,
			"message":     "获取游戏状态成功",
			"data":        status,
		})
	}
}
