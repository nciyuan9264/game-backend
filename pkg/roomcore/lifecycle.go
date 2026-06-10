package roomcore

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

// startDelayedDelete 当房间无在线真人时，启动 10 秒后删除房间的定时器。
func startDelayedDelete[R any](s *Service[R]) {
	if s.Base.DeleteTimer != nil {
		return
	}
	s.Logger.Info("房间10s后将被删除", "room_id", s.Base.ID)
	s.Base.DeleteTimer = time.AfterFunc(10*time.Second, func() {
		for _, p := range s.Base.Connections {
			if !p.AI && p.Online {
				s.Logger.Info("房间有玩家重连，取消删除", "room_id", s.Base.ID)
				s.Base.DeleteTimer = nil
				return
			}
		}
		StopThinkTimer(s)
		close(s.Base.QuitCh)
		s.OnRoomDeleted(s.Game)
		s.Logger.Info("房间已被延迟删除", "room_id", s.Base.ID)
	})
}

// StartCreateGrace 房间创建后启动一个兜底删除定时器：
// 60 秒后若房间内仍无任何在线真人（说明创建者从未连 ws 或已离开），则删除房间。
// 用于兜底「创建房间后从未建立 WebSocket 连接」导致房间残留的场景。
func StartCreateGrace[R any](s *Service[R]) {
	if s.Base.DeleteTimer != nil {
		return
	}
	s.Logger.Info("房间创建宽限期开始，60s 内无人连接将被删除", "room_id", s.Base.ID)
	s.Base.DeleteTimer = time.AfterFunc(60*time.Second, func() {
		for _, p := range s.Base.Connections {
			if !p.AI && p.Online {
				s.Base.DeleteTimer = nil
				return
			}
		}
		StopThinkTimer(s)
		select {
		case <-s.Base.QuitCh:
			// 已被其他路径关闭（如 splendor /room/delete），避免重复 close panic。
		default:
			close(s.Base.QuitCh)
		}
		s.OnRoomDeleted(s.Game)
		s.Logger.Info("房间创建后无人连接，已被删除", "room_id", s.Base.ID)
	})
}

// transferOwnerOrDelete 把房主转给一个在线真人；若没有则启动延迟删除。
// 返回是否触发了房间删除。
func transferOwnerOrDelete[R any](s *Service[R]) bool {
	for _, p := range s.Base.Connections {
		if !p.AI && p.Online {
			s.SetOwner(s.Game, p.PlayerID)
			s.Logger.Info("房主已变更", "room_id", s.Base.ID, "player_id", p.PlayerID)
			return false
		}
	}

	if s.StatusOf(s.Game) == StatusMatch {
		startDelayedDelete(s)
		return true
	}
	return false
}

// MarkPlayerOffline 处理玩家离线。
// match 阶段直接踢出；对局阶段仅维护连接状态（不再触发 AI 接管，超时由思考超时机制处理）。
func MarkPlayerOffline[R any](s *Service[R], playerID string) (roomDeleted bool) {
	if s.StatusOf(s.Game) == StatusMatch {
		for i, pID := range s.Base.PlayerSeq {
			if pID == playerID {
				s.Base.PlayerSeq = append(s.Base.PlayerSeq[:i], s.Base.PlayerSeq[i+1:]...)
				break
			}
		}
		delete(s.Base.Connections, playerID)
		s.Logger.Info("玩家离开房间", "room_id", s.Base.ID, "player_id", playerID)
	} else {
		p, ok := s.Base.Connections[playerID]
		if !ok {
			return
		}

		if p.Conn != nil {
			p.Conn.Close()
		}

		p.Online = false
		p.Conn = nil

		s.Logger.Info("玩家离线（对局阶段保留连接记录）", "room_id", s.Base.ID, "player_id", playerID)
	}

	if playerID == s.GetOwner(s.Game) {
		return transferOwnerOrDelete(s)
	}

	return false
}

// HandleDisconnect 玩家断开连接命令。
func HandleDisconnect[R any](s *Service[R], cmd Command) {
	MarkPlayerOffline(s, cmd.PlayerID)
}

// roomPlayerCount 返回房间在线玩家数（含 AI）。
func roomPlayerCount[R any](s *Service[R]) int {
	onlineCount := 0
	for _, pc := range s.Base.Connections {
		if pc.Online {
			onlineCount++
		}
	}
	return onlineCount
}

// HandleConnect 处理玩家加入命令。
func HandleConnect[R any](s *Service[R], cmd Command) {
	playerID := cmd.PlayerID
	conn := cmd.Conn

	if p, ok := s.Base.Connections[playerID]; ok {
		s.Logger.Info("玩家尝试加入房间（已存在玩家）",
			"room_id", s.Base.ID, "player_id", playerID, "current_status", p.Online)
		if !p.Online {
			if s.OnAttachReader != nil {
				s.OnAttachReader(s.Game, conn)
			}
			p.Conn = conn
			p.Online = true

			s.Logger.Info("玩家重连成功", "room_id", s.Base.ID, "player_id", playerID)
			return
		}

		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"玩家已在房间内,请从其他设备退出"}`))
		conn.Close()
		return
	}

	if s.StatusOf(s.Game) != StatusMatch {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"游戏已开始，无法加入"}`))
		conn.Close()
		return
	}

	if len(s.Base.Connections) >= s.GetMaxPlayers(s.Game) {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"房间已满"}`))
		conn.Close()
		return
	}

	if s.OnAttachReader != nil {
		s.OnAttachReader(s.Game, conn)
	}

	s.Base.Connections[playerID] = &PlayerConn{
		PlayerID: playerID,
		Conn:     conn,
		Online:   true,
		Ready:    s.GetOwner(s.Game) == playerID,
		AI:       false,
	}
	s.Base.PlayerSeq = append(s.Base.PlayerSeq, playerID)

	s.Logger.Info("玩家加入房间", "room_id", s.Base.ID, "player_id", playerID)
}

// readyPayload match_ready / 局内 ready 共用的 payload 形态。
type readyPayload struct {
	Ready bool `json:"ready"`
}

// HandleMatchReady 处理 match 阶段的 ready 状态切换。
func HandleMatchReady[R any](s *Service[R], cmd Command) {
	var p readyPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		s.Logger.Error("无效的 payload",
			"room_id", s.Base.ID, "player_id", cmd.PlayerID, "error", err)
		return
	}
	ready := p.Ready

	for _, pc := range s.Base.Connections {
		if pc.PlayerID == cmd.PlayerID {
			pc.Ready = ready
			break
		}
	}
}

// JoinMatchAsAI 把一个 AI 玩家加入到当前匹配中。
func JoinMatchAsAI[R any](s *Service[R], playerID string) bool {
	s.Base.Connections[playerID] = &PlayerConn{
		PlayerID: playerID,
		Conn:     s.NewVirtualConn(s.Game),
		Online:   true,
		Ready:    true,
		AI:       true,
	}
	s.Base.PlayerSeq = append(s.Base.PlayerSeq, playerID)

	s.Logger.Info("AI 玩家加入房间", "room_id", s.Base.ID, "player_id", playerID)
	return true
}

func writeCommandError[R any](s *Service[R], cmd Command, message string) {
	conn := WriteOnlyConn(cmd.Conn)
	if conn == nil {
		if pc, ok := s.Base.Connections[cmd.PlayerID]; ok && pc != nil {
			conn = pc.Conn
		}
	}
	if conn == nil {
		return
	}

	msg := struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}{
		Type:    "error",
		Message: message,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		s.Logger.Error("编码错误消息失败", "room_id", s.Base.ID, "player_id", cmd.PlayerID, "error", err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		s.Logger.Error("发送错误消息失败", "room_id", s.Base.ID, "player_id", cmd.PlayerID, "error", err)
	}
}

// HandleAddAI 命令：往房间加一个 AI。
func HandleAddAI[R any](s *Service[R], cmd Command) {
	if len(s.Base.Connections) >= s.GetMaxPlayers(s.Game) {
		s.Logger.Error("房间已满，无法添加 AI", "room_id", s.Base.ID)
		writeCommandError(s, cmd, "房间已满")
		return
	}

	if s.StatusOf(s.Game) != StatusMatch {
		s.Logger.Error("房间状态不是等待加入，无法添加 AI",
			"room_id", s.Base.ID, "current_status", s.StatusOf(s.Game))
		writeCommandError(s, cmd, "游戏已开始，无法添加 AI")
		return
	}

	JoinMatchAsAI(s, s.GenAIPlayerID(s.Game))
}

// removePlayerPayload match_remove_player 命令的 payload。
type removePlayerPayload struct {
	PlayerID string `json:"playerID"`
}

// RemovePlayer 从房间中物理移除一个玩家（不发消息、不广播）。
func RemovePlayer[R any](s *Service[R], playerID string) bool {
	if _, ok := s.Base.Connections[playerID]; !ok {
		s.Logger.Error("房间中不存在玩家", "room_id", s.Base.ID, "player_id", playerID)
		return false
	}

	delete(s.Base.Connections, playerID)
	for i, pid := range s.Base.PlayerSeq {
		if pid == playerID {
			s.Base.PlayerSeq = append(s.Base.PlayerSeq[:i], s.Base.PlayerSeq[i+1:]...)
			break
		}
	}
	s.Logger.Info("玩家从房间移除", "room_id", s.Base.ID, "player_id", playerID)
	return true
}

// HandleRemovePlayer 命令：通知被移除的玩家并下线之。
func HandleRemovePlayer[R any](s *Service[R], cmd Command) {
	var p removePlayerPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		s.Logger.Error("无效的 payload",
			"room_id", s.Base.ID, "player_id", cmd.PlayerID, "error", err)
		return
	}
	removePlayerID := p.PlayerID

	if s.StatusOf(s.Game) != StatusMatch {
		s.Logger.Error("房间状态不是等待加入，无法移除玩家",
			"room_id", s.Base.ID, "current_status", s.StatusOf(s.Game))
		return
	}

	if removePlayerConn, ok := s.Base.Connections[removePlayerID]; ok {
		msg := struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}{
			Type:    "error",
			Message: "你已被移出房间",
		}

		if data, err := json.Marshal(msg); err == nil {
			removePlayerConn.Conn.WriteMessage(websocket.TextMessage, data)
			removePlayerConn.Conn.Close()
		}
	}

	RemovePlayer(s, removePlayerID)
}

// HandleReadyMessage 局内玩家点击 ready：当全员到齐时回调 OnAllReady 由 game 决定下一阶段。
func HandleReadyMessage[R any](s *Service[R], cmd Command) {
	maxPlayers := s.GetMaxPlayers(s.Game)
	playerCount := roomPlayerCount(s)

	if playerCount == maxPlayers {
		s.OnAllReady(s.Game, cmd)
	}
}

// RearmThinkTimer 在主循环每条命令处理完后调用：根据当前 RoomStatus 重新启动思考超时定时器。
//   - 若 GetTurnTimeout 返回 ok=false：清空 deadline，不计时。
//   - 若当前玩家不存在或为 AI：不计时（AI 由 MaybeRunAIIfNeeded 自驱）。
//   - 否则：启动 time.AfterFunc(timeout, ...)，到点把 BuildTimeoutCommand 产出的命令塞进 CmdCh。
func RearmThinkTimer[R any](s *Service[R]) {
	if s.Base.ThinkTimer != nil {
		s.Base.ThinkTimer.Stop()
		s.Base.ThinkTimer = nil
	}
	s.Base.TurnDeadline = time.Time{}
	s.Base.TurnTimeout = 0

	if s.GetTurnTimeout == nil || s.GetCurrentPlayer == nil || s.BuildTimeoutCommand == nil {
		return
	}

	timeout, ok := s.GetTurnTimeout(s.Game)
	if !ok || timeout <= 0 {
		return
	}

	currentPlayerID := s.GetCurrentPlayer(s.Game)
	if currentPlayerID == "" {
		return
	}
	pc, ok := s.Base.Connections[currentPlayerID]
	if !ok || pc == nil || pc.AI {
		return
	}

	s.Base.TurnDeadline = time.Now().Add(timeout)
	s.Base.TurnTimeout = timeout

	deadlineSnap := s.Base.TurnDeadline
	playerSnap := currentPlayerID

	s.Base.ThinkTimer = time.AfterFunc(timeout, func() {
		// 仅在 timer 仍然对应当前 deadline 时投递，避免被新 timer 替换后还触发。
		// 真正的"是否依然轮到该玩家"判断在 game 的 turn_timeout handler 内做。
		cmd, ok := s.BuildTimeoutCommand(s.Game, playerSnap)
		if !ok {
			s.Logger.Info("思考超时但未产生有效动作", "room_id", s.Base.ID, "player_id", playerSnap)
			return
		}
		_ = deadlineSnap
		select {
		case s.Base.CmdCh <- cmd:
		case <-s.Base.QuitCh:
		}
	})
}

// StopThinkTimer 取消当前思考超时定时器并清空截止时刻。
func StopThinkTimer[R any](s *Service[R]) {
	if s.Base.ThinkTimer != nil {
		s.Base.ThinkTimer.Stop()
		s.Base.ThinkTimer = nil
	}
	s.Base.TurnDeadline = time.Time{}
	s.Base.TurnTimeout = 0
}
