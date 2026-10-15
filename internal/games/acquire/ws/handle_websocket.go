package ws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
	"github.com/nciyuan9264/game-backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Message struct {
	Type         string          `json:"type"`
	Payload      json.RawMessage `json:"payload"`
	RequestID    string          `json:"requestId"`
	PhaseVersion *uint64         `json:"phaseVersion"`
}

type wsReadStats struct {
	messageCount int
	lastType     string
	startedAt    time.Time
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// 将 HTTP 请求升级为 WebSocket 连接
func upgradeConnection(c *gin.Context) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WebSocket 升级失败", logger.F("error", err))
	}
	return conn, err
}

func readLoop(conn *domain.RealConn, room *domain.Room, playerID string) {
	stats := wsReadStats{startedAt: time.Now()}
	defer func() {
		// 断线也是 Command
		logger.Info("WebSocket read loop exited",
			logger.F("room_id", room.ID),
			logger.F("player_id", playerID),
			logger.F("message_count", stats.messageCount),
			logger.F("last_type", stats.lastType),
			logger.F("duration_ms", time.Since(stats.startedAt).Milliseconds()),
		)
		select {
		case room.CmdCh <- domain.Command{
			PlayerID: playerID,
			Type:     "disconnect",
			Conn:     conn,
		}:
		case <-room.QuitCh:
		}
		conn.Close()
	}()

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			logger.Warn("WebSocket read failed",
				logger.F("room_id", room.ID),
				logger.F("player_id", playerID),
				logger.F("message_count", stats.messageCount),
				logger.F("last_type", stats.lastType),
				logger.F("duration_ms", time.Since(stats.startedAt).Milliseconds()),
				logger.F("close", closeDescription(err)),
				logger.F("error", err),
			)
			return
		}
		stats.messageCount++
		stats.lastType = msg.Type

		// WS → Command
		if !clientCommandAllowed(msg.Type) {
			data, _ := json.Marshal(map[string]string{
				"type": "error", "code": "invalid_command", "message": "不支持的操作", "requestId": msg.RequestID,
			})
			conn.WriteMessage(websocket.TextMessage, data)
			continue
		}
		select {
		case room.CmdCh <- domain.Command{
			Type:         msg.Type,
			Payload:      msg.Payload,
			PlayerID:     playerID,
			Conn:         conn,
			RequestID:    msg.RequestID,
			PhaseVersion: msg.PhaseVersion,
		}:
		case <-room.QuitCh:
			return
		}
	}
}

func HandleWebSocket(c *gin.Context) {
	id, authenticated := c.Get("user_id")
	if !authenticated {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "unauthorized"})
		return
	}
	playerID := fmt.Sprintf("%s-%v", c.GetString("name"), id)
	roomID := c.Query("roomID")
	conn, err := upgradeConnection(c)
	if err != nil {
		return
	}

	// 创建 RealConn 实例
	realConn := domain.NewRealConn(conn)

	// 1️⃣ 解析参数（保持不变）
	if roomID == "" || playerID == "" {
		logger.Warn("WebSocket missing query",
			logger.F("room_id", roomID),
			logger.F("player_id", playerID),
			logger.F("remote_addr", c.Request.RemoteAddr),
			logger.F("forwarded_for", c.GetHeader("X-Forwarded-For")),
		)
		realConn.Close()
		return
	}

	// 2️⃣ 拿房间（只读，不改）
	room, ok := roompkg.Rooms.Get(roomID)
	if !ok {
		logger.Warn("WebSocket room not found",
			logger.F("room_id", roomID),
			logger.F("player_id", playerID),
			logger.F("remote_addr", c.Request.RemoteAddr),
			logger.F("forwarded_for", c.GetHeader("X-Forwarded-For")),
		)
		realConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","code":"room_not_found","message":"房间不存在"}`))
		realConn.Close()
		return
	}

	logger.Info("WebSocket connected",
		logger.F("room_id", roomID),
		logger.F("player_id", playerID),
		logger.F("remote_addr", c.Request.RemoteAddr),
		logger.F("forwarded_for", c.GetHeader("X-Forwarded-For")),
		logger.F("user_agent", c.GetHeader("User-Agent")),
	)

	// 3️⃣ 通知房间：有人 join（Command）
	select {
	case room.Room.CmdCh <- domain.Command{
		Type:     "connect",
		PlayerID: playerID,
		Conn:     realConn,
		Name:     c.GetString("name"),
		Avatar:   c.GetString("avatar"),
	}:
	case <-room.Room.QuitCh:
		realConn.Close()
		return
	}

	// 4️⃣ 启动 WS 读循环（每个连接一个 goroutine）
	go readLoop(realConn, room.Room, playerID)
}

func clientCommandAllowed(command string) bool {
	switch command {
	case "match_ready", "match_begin", "match_add_ai", "match_remove_player", "room_update_settings",
		"game_ready", "game_place_tile", "game_create_company", "game_merging_settle",
		"game_buy_stock", "game_merging_selection", "game_end_vote", "game_play_audio", "game_restart_game":
		return true
	default:
		return false
	}
}

func closeDescription(err error) string {
	if closeErr, ok := err.(*websocket.CloseError); ok {
		return closeErr.Error()
	}
	return "none"
}
