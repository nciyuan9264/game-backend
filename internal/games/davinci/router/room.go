package router

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/service"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
)

func requestUserID(c *gin.Context) (string, bool) {
	id, ok := c.Get("user_id")
	if !ok {
		roomError(c, http.StatusUnauthorized, "unauthorized", "请先登录")
		return "", false
	}
	return fmt.Sprint(id), true
}

func requestPlayerID(c *gin.Context) (string, bool) {
	id, ok := requestUserID(c)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s-%s", c.GetString("name"), id), true
}

func roomError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"status_code": status, "code": code, "message": message})
}

func createRoom(c *gin.Context) {
	playerID, ok := requestPlayerID(c)
	if !ok {
		return
	}
	roomID, err := service.CreateRoomWithProfile(playerID, c.GetString("name"), c.GetString("avatar"))
	switch {
	case errors.Is(err, roomctl.ErrTooManyRooms):
		roomError(c, http.StatusConflict, "too_many_rooms", "你最多只能同时拥有 5 个房间")
	case err != nil:
		roomError(c, http.StatusInternalServerError, "create_failed", "创建房间失败，请重试")
	default:
		c.JSON(http.StatusOK, gin.H{
			"status_code": http.StatusOK,
			"message":     "房间创建成功",
			"data":        gin.H{"roomID": roomID},
		})
	}
}

func listRooms(c *gin.Context) {
	userID, ok := requestUserID(c)
	if !ok {
		return
	}
	rooms, err := service.GetRoomList(userID)
	if err != nil {
		roomError(c, http.StatusInternalServerError, "list_failed", "获取房间列表失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"message":     "获取成功",
		"data":        gin.H{"rooms": rooms},
	})
}
