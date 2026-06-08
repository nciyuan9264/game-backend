package wsupgrade

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

// DefaultUpgrader 默认 WebSocket 升级器（允许任意 Origin）
var DefaultUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Upgrade 将 HTTP 请求升级为 WebSocket 连接
func Upgrade(c *gin.Context) (*websocket.Conn, error) {
	conn, err := DefaultUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WebSocket 升级失败", logger.F("error", err))
	}
	return conn, err
}
