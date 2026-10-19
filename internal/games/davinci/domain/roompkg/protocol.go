package roompkg

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/game"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func (r *RoomService) reply(cmd domain.Command, code, message string) {
	conn := domain.WriteOnlyConn(cmd.Conn)
	if conn == nil {
		if pc := r.Room.Connections[cmd.PlayerID]; pc != nil {
			conn = pc.Conn
		}
	}
	if conn == nil {
		return
	}
	kind := "command_result"
	if code != "" {
		kind = "error"
	}
	data, _ := json.Marshal(map[string]any{
		"type": kind, "code": code, "message": message, "requestId": cmd.RequestID,
		"phaseVersion": r.Room.State.PhaseVersion,
	})
	_ = conn.WriteMessage(websocket.TextMessage, data)
}

// validateCommand runs in the room actor before any state mutation.
func (r *RoomService) validateCommand(cmd domain.Command) bool {
	state := r.Room.State
	reject := func(code, message string) bool {
		r.reply(cmd, code, message)
		return false
	}

	if cmd.Type == "connect" {
		if cmd.Conn == nil {
			return false
		}
		pc := r.Room.Connections[cmd.PlayerID]
		code, message := "", ""
		switch {
		case pc != nil && pc.Online:
			code, message = "identity_conflict", "此身份已在其他窗口连接，请先退出"
		case pc == nil && state.RoomStatus != domain.RoomStatusMatch:
			code, message = "game_started", "对局已开始，仅原玩家可以重连"
		case pc == nil && len(r.Room.Connections) >= MaxPlayers:
			code, message = "room_full", "房间已满"
		}
		if code != "" {
			reject(code, message)
			_ = cmd.Conn.Close()
			return false
		}
		return true
	}

	pc := r.Room.Connections[cmd.PlayerID]
	if cmd.Type == "disconnect" {
		return pc != nil && (cmd.Conn == nil || pc.Conn == cmd.Conn)
	}
	_, internal := cmd.Conn.(*VirtualConn)
	if pc == nil {
		return reject("not_member", "连接已失效，请重新进入房间")
	}
	if cmd.Conn != nil && pc.Conn != cmd.Conn && !internal {
		return reject("not_member", "连接已失效，请重新进入房间")
	}
	if cmd.PhaseVersion != nil && *cmd.PhaseVersion != state.PhaseVersion {
		return reject("phase_changed", "阶段已变化，请根据最新状态操作")
	}
	if cmd.Type == "turn_timeout" && internal {
		return state.CurrentPlayer == cmd.PlayerID
	}

	switch cmd.Type {
	case "match_begin", "match_add_ai", "match_remove_player":
		if cmd.PlayerID != state.OwnerID {
			return reject("forbidden", "只有房主可以操作")
		}
		if state.RoomStatus != domain.RoomStatusMatch {
			return reject("phase_changed", "只能在准备阶段操作")
		}
	case "match_ready":
		if state.RoomStatus != domain.RoomStatusMatch {
			return reject("phase_changed", "准备阶段已结束")
		}
	case "game_restart_game":
		if cmd.PlayerID != state.OwnerID {
			return reject("forbidden", "只有房主可以重新开局")
		}
		if state.RoomStatus != domain.RoomStatusEnd {
			return reject("phase_changed", "本局尚未结束")
		}
	case "game_ready":
		return state.RoomStatus == domain.RoomStatusWaiting
	}

	switch cmd.Type {
	case "match_ready":
		var payload struct {
			Ready *bool `json:"ready"`
		}
		if json.Unmarshal(cmd.Payload, &payload) != nil || payload.Ready == nil {
			return reject("invalid_payload", "准备状态无效")
		}
		if cmd.PlayerID == state.OwnerID && !*payload.Ready {
			return reject("forbidden", "房主自动准备")
		}
	case "match_begin":
		if len(r.Room.Connections) != MaxPlayers {
			return reject("not_ready", "需要两位玩家才能开始")
		}
		for _, player := range r.Room.Connections {
			if player == nil || !player.Ready || !player.Online {
				return reject("not_ready", "请等待所有玩家准备")
			}
		}
	case "match_add_ai":
		if len(r.Room.Connections) >= MaxPlayers {
			return reject("room_full", "房间已满")
		}
		var payload struct {
			SeatIndex *int `json:"seatIndex"`
		}
		if len(cmd.Payload) > 0 && json.Unmarshal(cmd.Payload, &payload) != nil {
			return reject("invalid_payload", "席位无效")
		}
		if payload.SeatIndex != nil {
			if *payload.SeatIndex < 0 || *payload.SeatIndex >= MaxPlayers {
				return reject("invalid_seat", "席位无效")
			}
			for _, player := range r.Room.Connections {
				if player != nil && player.SeatIndex == *payload.SeatIndex {
					return reject("seat_occupied", "该席位已被占用")
				}
			}
		}
	case "match_remove_player":
		var payload struct {
			PlayerID string `json:"playerID"`
		}
		if json.Unmarshal(cmd.Payload, &payload) != nil || r.Room.Connections[payload.PlayerID] == nil {
			return reject("invalid_player", "玩家已离开")
		}
		if payload.PlayerID == state.OwnerID || payload.PlayerID == cmd.PlayerID {
			return reject("forbidden", "不能移除房主或自己")
		}
	case "game_play_audio":
		var payload game.PlayAudioPayload
		if json.Unmarshal(cmd.Payload, &payload) != nil || !allowedAudioType(payload.AudioType) {
			return reject("invalid_payload", "音效类型无效")
		}
	}

	actionPhases := map[string][]domain.RoomStatus{
		"game_get_card":   {domain.RoomStatusGetCard},
		"game_guess_card": {domain.RoomStatusGuessCard, domain.RoomStatusSetCard},
		"game_set_card":   {domain.RoomStatusSetCard},
	}
	if phases, isAction := actionPhases[cmd.Type]; isAction {
		validPhase := false
		for _, phase := range phases {
			validPhase = validPhase || state.RoomStatus == phase
		}
		if !validPhase {
			return reject("phase_changed", "当前阶段不允许此操作")
		}
		if state.CurrentPlayer != cmd.PlayerID {
			return reject("not_your_turn", "请等待你的回合")
		}
		if state.Players[cmd.PlayerID] == nil {
			return reject("invalid_player", "玩家数据尚未就绪")
		}
	}

	switch cmd.Type {
	case "game_get_card":
		var payload game.HandleGetCardPayload
		if json.Unmarshal(cmd.Payload, &payload) != nil || state.BoardCards[payload.ID] == nil {
			return reject("invalid_card", "这张牌已不可抽取")
		}
		for _, card := range state.Players[cmd.PlayerID].Cards {
			if card != nil && card.Index == -1 {
				return reject("phase_changed", "请先处理已抽取的牌")
			}
		}
	case "game_guess_card":
		var payload game.HandleGuessCardPayload
		if json.Unmarshal(cmd.Payload, &payload) != nil ||
			payload.Num < domain.NumMinus1 || payload.Num > domain.Num11 {
			return reject("invalid_guess", "猜测数字无效")
		}
		var target *domain.Card
		for playerID, player := range state.Players {
			if playerID == cmd.PlayerID || player == nil {
				continue
			}
			for _, card := range player.Cards {
				if card != nil && card.ID == payload.ID {
					target = card
					break
				}
			}
		}
		if target == nil || target.IsRevealed {
			return reject("invalid_card", "请选择对手尚未公开的牌")
		}
	case "game_set_card":
		var payload game.HandleSetCardPayload
		if json.Unmarshal(cmd.Payload, &payload) != nil {
			return reject("invalid_payload", "放回位置无效")
		}
		player := state.Players[cmd.PlayerID]
		var target *domain.Card
		for _, card := range player.Cards {
			if card != nil && card.ID == payload.ID && card.Index == -1 {
				target = card
				break
			}
		}
		if target == nil {
			return reject("invalid_card", "只能放回本回合抽取的牌")
		}
		maxIndex := len(player.Cards) - 1
		if payload.Index < 0 || payload.Index > maxIndex {
			return reject("invalid_position", "放回位置超出范围")
		}
		if target.Num != domain.NumMinus1 && payload.Index != game.ComputeInsertIndex(player.Cards, target) {
			return reject("invalid_position", "数字牌必须按顺序放回")
		}
	}
	return true
}

func allowedAudioType(value string) bool {
	switch value {
	case "quickily", "quickily1", "quickily2", "your-turn", "create-company",
		"buy-stock", "get-noble-card", "hover-button":
		return true
	default:
		return false
	}
}

func commandMutatesState(command string) bool {
	return command != "game_play_audio"
}

func (r *RoomService) advanceVersion(previousPlayer string, command string) {
	state := r.Room.State
	if !commandMutatesState(command) {
		return
	}
	state.PhaseVersion++
	if state.RoomStatus == domain.RoomStatusMatch {
		state.TurnNumber = 0
		return
	}
	if state.CurrentPlayer != "" && previousPlayer != state.CurrentPlayer {
		state.TurnNumber++
	}
}

func (r *RoomService) resolveTimeout(cmd domain.Command) (domain.Command, bool) {
	state := r.Room.State
	if cmd.Conn != nil || cmd.PhaseVersion == nil || *cmd.PhaseVersion != state.PhaseVersion ||
		state.CurrentPlayer != cmd.PlayerID || r.Room.TurnDeadline.IsZero() ||
		time.Now().Before(r.Room.TurnDeadline) {
		return domain.Command{}, false
	}
	return BuildTurnTimeoutCommand(r.Room, cmd.PlayerID)
}

// The timer only captures immutable values. Room state is inspected by the
// actor after the timeout command is received.
func (r *RoomService) armTimer() {
	roomcore.StopThinkTimer(r.svc)
	timeout, timed := turnTimeoutByStatus[r.Room.State.RoomStatus]
	playerID := r.Room.State.CurrentPlayer
	pc := r.Room.Connections[playerID]
	if !timed || pc == nil || pc.AI {
		return
	}

	version := r.Room.State.PhaseVersion
	r.Room.TurnTimeout = timeout
	r.Room.TurnDeadline = time.Now().Add(timeout)
	r.Room.ThinkTimer = time.AfterFunc(timeout, func() {
		select {
		case r.Room.CmdCh <- domain.Command{
			Type:         "davinci_timeout",
			PlayerID:     playerID,
			PhaseVersion: &version,
		}:
		case <-r.Room.QuitCh:
		}
	})
}
