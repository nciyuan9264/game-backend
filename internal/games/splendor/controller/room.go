package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/service"
	"github.com/nciyuan9264/game-backend/pkg/roomctl/dto"
)

// DeleteRoom splendor 特有：清理 Redis 中的房间相关 key。
func DeleteRoom(c *gin.Context) {
	var req dto.DeleteRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要字段"})
		return
	}
	if err := service.DeleteRoom(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status_code": http.StatusOK,
		"message":     "房间删除成功",
	})
}
