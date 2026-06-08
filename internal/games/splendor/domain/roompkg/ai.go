package roompkg

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// VirtualConn 用作 AI 玩家的写连接占位。它不连接真实客户端，
// 只是在 BroadcastToRoom 给 AI 投递 sync 消息时被调用，从而触发 AI 决策。
type VirtualConn struct {
	Room *domain.Room
}

// 编译期断言实现 WriteOnlyConn
var _ domain.WriteOnlyConn = (*VirtualConn)(nil)

func (v *VirtualConn) WriteMessage(messageType int, data []byte) error {
	MaybeRunAIIfNeeded(v.Room, data)
	return nil
}

func (v *VirtualConn) ReadMessage() (messageType int, p []byte, err error) {
	return 0, nil, fmt.Errorf("virtual connection cannot read")
}

func (v *VirtualConn) Close() error {
	return nil
}

// IsAIPlayerID 判断 playerID 是否为 AI 玩家（基于命名约定）。
func IsAIPlayerID(playerID string) bool {
	return strings.HasPrefix(playerID, "ai_")
}

// chooseActionForAI splendor AI 决策的占位（与旧 ws/ai.go 行为一致：返回空）。
func chooseActionForAI(r *domain.Room, playerID string) string {
	return ""
}

// buildAIActionMsg 根据当前 RoomStatus 选出"AI/超时"应当投递的命令 type+payload。
// 返回 ok=false 表示该状态下没有可投递的动作。
func buildAIActionMsg(r *domain.Room, playerID string, status domain.RoomStatus) (cmdType string, payload []byte, ok bool) {
	switch status {
	case domain.RoomStatusPlaying, domain.RoomStatusLastTurn:
		action := chooseActionForAI(r, playerID)
		if action == "" {
			return "", nil, false
		}
		data, err := json.Marshal(action)
		if err != nil {
			return "", nil, false
		}
		return "game_get_gem", data, true
	case domain.RoomStatusEnd:
		data, err := json.Marshal(map[string]interface{}{})
		if err != nil {
			return "", nil, false
		}
		return "game_restart_game", data, true
	default:
		return "", nil, false
	}
}

// BuildTurnTimeoutCommand 基于当前状态生成"代真人玩家"的命令。
// 与 MaybeRunAIIfNeeded 共用 buildAIActionMsg，但 PlayerID 用真实玩家 ID，且不修改身份。
func BuildTurnTimeoutCommand(r *domain.Room, playerID string) (roomcore.Command, bool) {
	cmdType, payload, ok := buildAIActionMsg(r, playerID, r.State.RoomStatus)
	if !ok {
		return roomcore.Command{}, false
	}
	return roomcore.Command{
		Type:     cmdType,
		PlayerID: playerID,
		Payload:  payload,
		Conn:     &VirtualConn{Room: r},
	}, true
}

// MaybeRunAIIfNeeded 在 AI 玩家收到 sync 消息时，按需在新 goroutine 里
// 给房间 CmdCh 投递一条游戏命令。当前 chooseActionForAI 返回空，等同空操作。
func MaybeRunAIIfNeeded(r *domain.Room, data []byte) bool {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("AI 消息格式错误 room=%s err=%v", r.ID, err)
		return false
	}

	roomData, ok := msg["roomData"].(map[string]interface{})
	if !ok {
		return false
	}
	currentPlayerID, ok := roomData["currentPlayer"].(string)
	if !ok || currentPlayerID == "" {
		return false
	}

	roomInfo, ok := roomData["roomInfo"].(map[string]interface{})
	if !ok {
		return false
	}
	gameStatusStr, ok := roomInfo["gameStatus"].(string)
	if !ok || gameStatusStr == "" {
		return false
	}
	gameStatus := domain.RoomStatus(gameStatusStr)

	if !IsAIPlayerID(currentPlayerID) {
		return false
	}

	if r.AIRunning {
		return false
	}
	r.AIRunning = true

	go func() {
		defer func() { r.AIRunning = false }()
		time.Sleep(3 * time.Second)

		cmdType, payload, ok := buildAIActionMsg(r, currentPlayerID, gameStatus)
		if !ok {
			log.Printf("AI 未选择有效动作 room=%s player=%s", r.ID, currentPlayerID)
			return
		}

		r.CmdCh <- domain.Command{
			Type:     cmdType,
			PlayerID: currentPlayerID,
			Payload:  payload,
			Conn:     &VirtualConn{Room: r},
		}
	}()

	return true
}
