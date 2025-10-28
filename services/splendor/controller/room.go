package controller

import (
	"context"
	"net/http"

	"splendor-service/client"
	"splendor-service/dto"
	roomproto "splendor-service/proto"
	"splendor-service/service"

	"github.com/gin-gonic/gin"
)

// 创建房间 - 直接转发到 room-service
func CreateRoom(c *gin.Context) {
	// Parse request parameters
	var req dto.CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request format"})
		return
	}

	ctx := context.Background()
	// 直接调用 room-service gRPC 接口
	resp, err := client.RoomServiceClient.CreateRoom(ctx, req.GameType, int32(req.MaxPlayers), int32(req.AiCount), req.UserID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, resp)
}

// 删除房间 - 直接转发到 room-service
func DeleteRoom(c *gin.Context) {
	roomID := c.Param("roomId")
	if roomID == "" {
		c.JSON(400, gin.H{"error": "Room ID is required"})
		return
	}

	ctx := context.Background()
	// 直接调用 room-service gRPC 接口
	resp, err := client.RoomServiceClient.DeleteRoom(ctx, roomID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, resp)
}

func GetRoomList(c *gin.Context) {
	// Use the service layer which handles the gRPC call
	gameType := c.Query("gameType")
	if gameType == "" {
		c.JSON(400, gin.H{"error": "Game type is required"})
		return
	}
	rooms, err := service.GetRoomList(gameType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// onlinePlayer, err := service.GetOnlinePlayer(gameType)
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	// 	return
	// }

	response := roomproto.ListRoomsResponse{
		Rooms:   rooms,
		Success: true,
		Message: "获取房间列表成功",
	}

	c.JSON(http.StatusOK, response)
}
