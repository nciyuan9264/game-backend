package controller

import (
	"fmt"
	"go-game/dto"
	"go-game/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func CreateRoom(c *gin.Context) {
	id, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user_id not found"})
		return
	}
	name, exists := c.Get("name")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "name not found"})
		return
	}

	// id := "3"
	// name := "wzy"

	userID := fmt.Sprintf("%s-%v", name, id)

	roomID, err := service.CreateRoom(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"message":     "房间创建成功",
		"data": dto.CreateRoomResponse{
			RoomID: roomID,
		},
	})
}

// func DeleteRoom(c *gin.Context) {
// 	var req dto.DeleteRoomRequest
// 	if err := c.ShouldBindJSON(&req); err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要字段"})
// 		return
// 	}
// 	err := service.DeleteRoom(req)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}
// 	c.JSON(http.StatusOK, gin.H{
// 		"status_code": http.StatusOK,
// 		"message":     "房间删除成功",
// 	})
// }

func GetRoomList(c *gin.Context) {
	rooms, err := service.GetRoomList()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取房间列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "获取成功",
		"status_code": http.StatusOK,
		"data": dto.GetRoomList{
			Rooms: rooms,
		},
	})
}
