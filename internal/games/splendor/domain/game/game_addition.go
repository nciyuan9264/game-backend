package game

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
)

func HandlePlayAudioMessage(r *domain.Room, cmd domain.Command) {
	var audioType string
	if err := json.Unmarshal(cmd.Payload, &audioType); err != nil {
		log.Printf("❌ play_audio payload 解析失败 room=%s err=%v", r.ID, err)
		return
	}
	msg := map[string]interface{}{
		"type":    "audio",
		"message": audioType,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	for _, pc := range r.Connections {
		if pc.Online && pc.Conn != nil {
			if err := pc.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("❌ 发送音频消息失败 player=%s err=%v", pc.PlayerID, err)
			}
		}
	}
}

func HandleRestartGameMessage(r *domain.Room, cmd domain.Command) {
	r.State.LastData = nil
	r.State.CurrentPlayer = ""
	r.State.FirstPlayer = ""
	r.State.GameStartTime = time.Time{}
	r.State.RoomStatus = domain.RoomStatusMatch

	for _, pc := range r.Connections {
		// 房主和 AI 保持 Ready，其他真人玩家需要重新点准备。
		if pc.AI || pc.PlayerID == r.State.OwnerID {
			pc.Ready = true
		} else {
			pc.Ready = false
		}
	}
}

// HandleTurnTimeoutMessage 在思考超时时由系统投递，强制把回合推进到下一个玩家，避免房间死锁。
// 仅当命令携带的 PlayerID 仍是当前玩家、且游戏未结束时才生效。
func HandleTurnTimeoutMessage(r *domain.Room, cmd domain.Command) {
	if r == nil || r.State == nil {
		return
	}
	if r.State.RoomStatus == domain.RoomStatusEnd {
		return
	}
	if r.State.CurrentPlayer != cmd.PlayerID {
		log.Printf("turn_timeout 已过期，忽略 room=%s cmd_player=%s current=%s", r.ID, cmd.PlayerID, r.State.CurrentPlayer)
		return
	}
	log.Printf("⏰ turn_timeout 兜底切人 room=%s player=%s status=%s", r.ID, cmd.PlayerID, r.State.RoomStatus)

	r.State.LastData = &domain.LastAction{
		Action:   "turn_timeout",
		PlayerID: cmd.PlayerID,
		Payload:  json.RawMessage("null"),
	}

	SwitchToNextPlayer(r)

	// 超时跳过也要遵守最后一轮的终局判定。
	if r.State.RoomStatus == domain.RoomStatusLastTurn && r.State.CurrentPlayer == r.State.FirstPlayer {
		r.State.RoomStatus = domain.RoomStatusEnd
	}
}
