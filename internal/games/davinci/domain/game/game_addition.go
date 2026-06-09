package game

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/logger"

	"github.com/gorilla/websocket"
)

func SwitchToNextPlayer(r *domain.Room, currentPlayerID string) error {
	if r == nil {
		return fmt.Errorf("房间为空")
	}

	if len(r.Connections) == 0 {
		return fmt.Errorf("房间 %s 没有玩家", r.ID)
	}

	ids := make([]string, 0, len(r.Connections))
	for pid, pc := range r.Connections {
		if pc != nil {
			ids = append(ids, pid)
		}
	}

	if len(ids) == 0 {
		return fmt.Errorf("房间 %s 没有在线玩家", r.ID)
	}

	sort.Strings(ids)

	// 找当前索引
	curIdx := -1
	for i, pid := range ids {
		if pid == currentPlayerID {
			curIdx = i
			break
		}
	}

	// 计算下一个玩家
	var nextPlayerID string
	if curIdx >= 0 {
		nextPlayerID = ids[(curIdx+1)%len(ids)]
	} else {
		nextPlayerID = ids[0]
	}

	r.State.CurrentPlayer = nextPlayerID

	logger.Info(
		"房间当前玩家切换",
		logger.F("current_player_id", currentPlayerID),
		logger.F("room_id", r.ID),
		logger.F("next_player_id", nextPlayerID),
	)

	return nil
}

type PlayAudioPayload struct {
	AudioType string `json:"audioType"`
}

func HandlePlayAudioMessage(r *domain.Room, cmd domain.Command) {
	var p PlayAudioPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		logger.Error("消息格式错误", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("error", err))
		return
	}
	audioType := p.AudioType

	msg := map[string]interface{}{
		"type":    "audio",
		"message": audioType,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		logger.Error("编码 JSON 失败", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID), logger.F("error", err))
		return
	}

	for _, pc := range r.Connections {
		if pc.Online && pc.Conn != nil {
			err := pc.Conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				logger.Error("向玩家 %s 发送音频消息失败", logger.F("room_id", r.ID), logger.F("player_id", pc.PlayerID), logger.F("error", err))
			}
		}
	}
}

func HandleRestartGameMessage(r *domain.Room, cmd domain.Command) {
	r.State.CurrentPlayer = ""
	r.State.GameStartTime = time.Time{}
	r.State.RoomStatus = domain.RoomStatusMatch
	// 恢复 MaxPlayers 为房间初始上限，下一次 match_begin 时会按实际人数重写
	r.State.MaxPlayers = domain.DefaultMaxPlayers
	r.State.LastData = nil
	r.State.BoardCards = make(map[string]*domain.Card)
	r.State.Players = make(map[string]*domain.PlayerState)
	for _, pc := range r.Connections {
		// 房主和 AI 保持 Ready，其他真人玩家需要重新点准备
		if pc.AI || pc.PlayerID == r.State.OwnerID {
			pc.Ready = true
		} else {
			pc.Ready = false
		}
		r.State.Players[pc.PlayerID] = &domain.PlayerState{
			Cards: []*domain.Card{},
		}
	}
}

// HandleTurnTimeoutMessage 在 AI/超时无法生成有效动作时由系统投递，
// 用于强制把回合推进到下一个玩家，避免房间死锁。
// 只有当命令携带的 PlayerID 仍是当前玩家时才生效，否则忽略，避免新回合被旧 timeout 影响。
func HandleTurnTimeoutMessage(r *domain.Room, cmd domain.Command) {
	if r == nil || r.State == nil {
		return
	}
	if r.State.CurrentPlayer != cmd.PlayerID {
		logger.Info("turn_timeout 已过期，忽略",
			logger.F("room_id", r.ID),
			logger.F("cmd_player", cmd.PlayerID),
			logger.F("current_player", r.State.CurrentPlayer))
		return
	}
	logger.Warn("turn_timeout 兜底切人",
		logger.F("room_id", r.ID),
		logger.F("player_id", cmd.PlayerID),
		logger.F("status", r.State.RoomStatus))
	if err := SwitchToNextPlayer(r, cmd.PlayerID); err != nil {
		logger.Error("turn_timeout 切人失败", logger.F("room_id", r.ID), logger.F("error", err))
		return
	}
	if len(r.State.BoardCards) == 0 {
		r.State.RoomStatus = domain.RoomStatusGuessCard
	} else {
		r.State.RoomStatus = domain.RoomStatusGetCard
	}
}
