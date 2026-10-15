package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/service"
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
	payload := struct {
		Visibility string `json:"visibility"`
	}{Visibility: "public"}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	err := decoder.Decode(&payload)
	if err != nil && !errors.Is(err, io.EOF) {
		roomError(c, http.StatusBadRequest, "invalid_payload", "房间设置格式不正确")
		return
	}
	if err == nil {
		var extra any
		if !errors.Is(decoder.Decode(&extra), io.EOF) {
			roomError(c, http.StatusBadRequest, "invalid_payload", "房间设置格式不正确")
			return
		}
	}
	roomID, err := service.CreateRoomWithSettings(playerID, payload.Visibility, c.GetString("name"), c.GetString("avatar"))
	switch {
	case errors.Is(err, service.ErrInvalidVisibility):
		roomError(c, http.StatusBadRequest, "invalid_visibility", "请选择公开或私密房间")
	case errors.Is(err, roomctl.ErrTooManyRooms):
		roomError(c, http.StatusConflict, "too_many_rooms", "你最多只能同时拥有 5 个房间")
	case err != nil:
		roomError(c, http.StatusInternalServerError, "create_failed", "创建房间失败，请重试")
	default:
		c.JSON(http.StatusOK, gin.H{"status_code": http.StatusOK, "data": gin.H{"roomID": roomID}})
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

func lookupRoom(c *gin.Context) {
	userID, ok := requestUserID(c)
	if !ok {
		return
	}
	roomID := strings.TrimSpace(c.Query("roomID"))
	if roomID == "" {
		roomError(c, http.StatusBadRequest, "invalid_room_id", "请输入完整房间号")
		return
	}
	room, err := service.LookupRoom(c.Request.Context(), roomID, userID)
	switch {
	case errors.Is(err, service.ErrRoomNotFound):
		roomError(c, http.StatusNotFound, "room_not_found", "房间不存在或已关闭")
	case err != nil:
		roomError(c, http.StatusServiceUnavailable, "room_unavailable", "暂时无法查询房间，请重试")
	default:
		c.JSON(http.StatusOK, gin.H{"status_code": http.StatusOK, "data": room})
	}
}
