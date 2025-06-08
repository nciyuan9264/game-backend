package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetRoomInfo(c *gin.Context) {
	roomID := c.Param("roomID")
	// 这里可以从全局房间列表中获取状态
	c.JSON(http.StatusOK, gin.H{
		"roomID": roomID,
		"status": "waiting", // 示例返回状态，可扩展为: waiting/playing/finished
	})
}
