package ws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/roompkg"
	"github.com/nciyuan9264/game-backend/pkg/logger"
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

var upgrader = websocket.Upgrader{CheckOrigin: checkOrigin}

func checkOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	normalized := parsed.Scheme + "://" + parsed.Host
	allowed := map[string]bool{
		"https://board.gamebus.online": true,
		"http://localhost:5173":        true,
		"http://127.0.0.1:5173":        true,
	}
	for _, value := range strings.Split(os.Getenv("CORS_ALLOW_ORIGINS"), ",") {
		if candidate := strings.TrimSpace(value); candidate != "" {
			allowed[candidate] = true
		}
	}
	return allowed[normalized]
}

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
		logger.Info("WebSocket read loop exited",
			logger.F("room_id", room.ID),
			logger.F("player_id", playerID),
			logger.F("message_count", stats.messageCount),
			logger.F("last_type", stats.lastType),
			logger.F("duration_ms", time.Since(stats.startedAt).Milliseconds()),
		)
		select {
		case room.CmdCh <- domain.Command{Type: "disconnect", PlayerID: playerID, Conn: conn}:
		case <-room.QuitCh:
		}
		_ = conn.Close()
	}()

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			logger.Warn("WebSocket read failed",
				logger.F("room_id", room.ID),
				logger.F("player_id", playerID),
				logger.F("message_count", stats.messageCount),
				logger.F("last_type", stats.lastType),
				logger.F("close", closeDescription(err)),
				logger.F("error", err),
			)
			return
		}
		stats.messageCount++
		stats.lastType = msg.Type

		if !clientCommandAllowed(msg.Type) {
			data, _ := json.Marshal(map[string]string{
				"type": "error", "code": "invalid_command", "message": "不支持的操作", "requestId": msg.RequestID,
			})
			_ = conn.WriteMessage(websocket.TextMessage, data)
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
	roomID := strings.TrimSpace(c.Query("roomID"))
	if roomID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"code": "invalid_room_id"})
		return
	}
	playerID := fmt.Sprintf("%s-%v", c.GetString("name"), id)

	conn, err := upgradeConnection(c)
	if err != nil {
		return
	}
	realConn := domain.NewRealConn(conn)

	room, ok := roompkg.Rooms.Get(roomID)
	if !ok {
		_ = realConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","code":"room_not_found","message":"房间不存在"}`))
		_ = realConn.Close()
		return
	}

	logger.Info("WebSocket connected",
		logger.F("room_id", roomID),
		logger.F("player_id", playerID),
		logger.F("remote_addr", c.Request.RemoteAddr),
		logger.F("forwarded_for", c.GetHeader("X-Forwarded-For")),
		logger.F("user_agent", c.GetHeader("User-Agent")),
	)
	select {
	case room.Room.CmdCh <- domain.Command{
		Type:     "connect",
		PlayerID: playerID,
		Conn:     realConn,
		Name:     c.GetString("name"),
		Avatar:   c.GetString("avatar"),
	}:
	case <-room.Room.QuitCh:
		_ = realConn.Close()
		return
	}
	go readLoop(realConn, room.Room, playerID)
}

func clientCommandAllowed(command string) bool {
	switch command {
	case "match_ready", "match_begin", "match_add_ai", "match_remove_player",
		"game_ready", "game_get_card", "game_guess_card", "game_set_card",
		"game_play_audio", "game_restart_game":
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
