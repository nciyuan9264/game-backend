package controller

import (
	"go-game/dto"
	"go-game/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func CreateRoom(c *gin.Context) {
	var req dto.CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要字段"})
		return
	}

	roomID, err := service.CreateRoom(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"msg":         "房间创建成功",
		"data": dto.CreateRoomResponse{
			Room_id: roomID,
		},
	})
}

func DeleteRoom(c *gin.Context) {
	var req dto.DeleteRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要字段"})
		return
	}
	err := service.DeleteRoom(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"msg":         "房间删除成功",
	})
}

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
