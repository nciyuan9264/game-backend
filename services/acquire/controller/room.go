package controller

import (
	"acquire-service/client"          // Add this import
	roomproto "acquire-service/proto" // 导入生成的 proto 包
	"acquire-service/service"
	"acquire-service/ws"
	"context" // Add this import
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 统一API响应结构
type APIResponse struct {
	StatusCode int         `json:"status_code"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data,omitempty"`
}

// 成功响应辅助函数
func SuccessResponse(data interface{}, message string) APIResponse {
	return APIResponse{
		StatusCode: 200,
		Message:    message,
		Data:       data,
	}
}

// 错误响应辅助函数
func ErrorResponse(statusCode int, message string) APIResponse {
	return APIResponse{
		StatusCode: statusCode,
		Message:    message,
	}
}

// 创建房间 - 直接转发到 room-service
func CreateRoom(c *gin.Context) {
	// Parse request parameters
	var req roomproto.CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response := ErrorResponse(http.StatusBadRequest, "Invalid request format")
		c.JSON(http.StatusBadRequest, response)
		return
	}

	ctx := context.Background()
	log.Printf("创建房间请求: gameType=%s, maxPlayers=%d, aiCount=%d, userId=%s", req.GameType, req.MaxPlayers, req.AiCount, req.UserId)
	// 直接调用 room-service gRPC 接口
	resp, err := client.RoomServiceClient.CreateRoom(ctx, req.GameType, int32(req.MaxPlayers), int32(req.AiCount), req.UserId)
	if err != nil {
		response := ErrorResponse(http.StatusInternalServerError, err.Error())
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	// 初始化房间
	_, err = service.InitRoom(resp.RoomId, int(req.AiCount))
	if err != nil {
		response := ErrorResponse(http.StatusInternalServerError, err.Error())
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	response := SuccessResponse(resp, "房间创建成功")
	c.JSON(http.StatusOK, response)
}

// 删除房间 - 直接转发到 room-service
func DeleteRoom(c *gin.Context) {
	var req roomproto.DeleteRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request format"})
		return
	}

	ctx := context.Background()
	// 直接调用 room-service gRPC 接口
	resp, err := client.RoomServiceClient.DeleteRoom(ctx, req.RoomId)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, resp)
}

// GetRoomListFromHub 从Hub获取房间列表
func GetRoomListFromHub(c *gin.Context, hub *ws.Hub) {
	gameType := c.Query("game_type")
	if gameType == "" {
		gameType = "acquire"
	}

	roomList, err := service.GetRoomListFromHub(hub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.Response{
			Code:    500,
			Message: fmt.Sprintf("获取房间列表失败: %v", err),
			Data:    nil,
		})
		return
	}

	c.JSON(http.StatusOK, model.Response{
		Code:    200,
		Message: "获取房间列表成功",
		Data:    roomList,
	})
}
