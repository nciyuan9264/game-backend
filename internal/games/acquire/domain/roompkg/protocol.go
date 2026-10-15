package roompkg

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/game"
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
	conn.WriteMessage(websocket.TextMessage, data)
}

// validateCommand runs within the actor, before any state mutation.
func (r *RoomService) validateCommand(cmd domain.Command) bool {
	state := r.Room.State
	reject := func(code, message string) bool {
		if cmd.Conn != nil {
			r.reply(cmd, code, message)
		}
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
			cmd.Conn.Close()
			return false
		}
		return true
	}
	pc := r.Room.Connections[cmd.PlayerID]
	if cmd.Type == "disconnect" {
		return pc != nil && (cmd.Conn == nil || pc.Conn == cmd.Conn)
	}
	_, virtual := cmd.Conn.(*VirtualConn)
	if pc == nil || (cmd.Conn != nil && pc.Conn != cmd.Conn && !(pc.AI && virtual)) {
		return reject("not_member", "连接已失效，请重新进入房间")
	}
	if cmd.PhaseVersion != nil && *cmd.PhaseVersion != state.PhaseVersion {
		return reject("phase_changed", "阶段已变化，请根据最新状态操作")
	}
	if cmd.Type == "acquire_timeout" || cmd.Type == "turn_timeout" {
		return cmd.Conn == nil || (pc.AI && virtual)
	}
	switch cmd.Type {
	case "room_update_settings", "match_begin", "match_add_ai", "match_remove_player":
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
	case "game_end_vote":
		if state.RoomStatus == domain.RoomStatusMatch || state.RoomStatus == domain.RoomStatusWaiting ||
			state.RoomStatus == domain.RoomStatusEnd {
			return reject("phase_changed", "当前不能确认终局")
		}
		if !state.EndGame.Eligible {
			return reject("end_not_available", "尚未达到终局条件")
		}
	case "game_ready":
		return state.RoomStatus == domain.RoomStatusWaiting
	}
	switch cmd.Type {
	case "room_update_settings":
		var p struct {
			Visibility string `json:"visibility"`
		}
		if json.Unmarshal(cmd.Payload, &p) != nil || (p.Visibility != "public" && p.Visibility != "private") {
			return reject("invalid_visibility", "请选择公开或私密")
		}
	case "match_ready":
		var p struct {
			Ready *bool `json:"ready"`
		}
		if json.Unmarshal(cmd.Payload, &p) != nil || p.Ready == nil {
			return reject("invalid_payload", "准备状态无效")
		}
		if cmd.PlayerID == state.OwnerID && !*p.Ready {
			return reject("forbidden", "房主自动准备")
		}
	case "match_begin":
		if len(r.Room.Connections) < 2 {
			return reject("not_ready", "至少需要两位玩家")
		}
		for _, player := range r.Room.Connections {
			if !player.Ready || !player.Online {
				return reject("not_ready", "请等待所有玩家准备")
			}
		}
	case "match_add_ai":
		if len(r.Room.Connections) >= MaxPlayers {
			return reject("room_full", "房间已满")
		}
		var p struct {
			SeatIndex *int `json:"seatIndex"`
		}
		if len(cmd.Payload) > 0 && json.Unmarshal(cmd.Payload, &p) != nil {
			return reject("invalid_payload", "席位无效")
		}
		if p.SeatIndex != nil {
			if *p.SeatIndex < 0 || *p.SeatIndex >= MaxPlayers {
				return reject("invalid_seat", "席位无效")
			}
			for _, player := range r.Room.Connections {
				if player != nil && player.SeatIndex == *p.SeatIndex {
					return reject("seat_occupied", "该席位已被占用")
				}
			}
		}
	case "match_remove_player":
		var p struct {
			PlayerID string `json:"playerID"`
		}
		if json.Unmarshal(cmd.Payload, &p) != nil || r.Room.Connections[p.PlayerID] == nil {
			return reject("invalid_player", "玩家已离开")
		}
		if p.PlayerID == state.OwnerID || p.PlayerID == cmd.PlayerID {
			return reject("forbidden", "不能移除房主或自己")
		}
	case "game_end_vote":
		if _, ok := game.ParseEndVotePayload(cmd.Payload); !ok {
			return reject("invalid_payload", "终局选择无效")
		}
	}
	phases := map[string]domain.RoomStatus{
		"game_place_tile": domain.RoomStatusSetTile, "game_create_company": domain.RoomStatusCreateCompany,
		"game_buy_stock": domain.RoomStatusBuyStock, "game_merging_selection": domain.RoomStatusMergingSelection,
		"game_merging_settle": domain.RoomStatusMergingSettle,
	}
	if phase, isAction := phases[cmd.Type]; isAction {
		if state.RoomStatus != phase {
			return reject("phase_changed", "当前阶段不允许此操作")
		}
		if cmd.Type != "game_merging_settle" && cmd.PlayerID != state.CurrentPlayer {
			return reject("not_your_turn", "请等待你的回合")
		}
		if state.Players[cmd.PlayerID] == nil {
			return reject("invalid_player", "玩家数据尚未就绪")
		}
	}
	switch cmd.Type {
	case "game_place_tile":
		var p game.PlaceTilePayload
		if json.Unmarshal(cmd.Payload, &p) != nil || state.BoardTiles[p.TileKey] == nil ||
			state.BoardTiles[p.TileKey].Belong != "" || !slices.Contains(state.Players[cmd.PlayerID].Tiles, p.TileKey) {
			return reject("invalid_tile", "这块地不能放置")
		}
	case "game_create_company":
		var p game.CreateCompanyPayload
		if json.Unmarshal(cmd.Payload, &p) != nil || state.Companies[p.Company] == nil || state.Companies[p.Company].Tiles != 0 {
			return reject("invalid_company", "请选择尚未成立的公司")
		}
	case "game_merging_selection":
		var p game.MergingSelectionPayload
		if json.Unmarshal(cmd.Payload, &p) != nil || !slices.Contains(state.MergingSelection.MainCompany, p.MainCompany) {
			return reject("invalid_company", "请选择可存续的公司")
		}
	case "game_buy_stock":
		var p game.BuyStockPayload
		if json.Unmarshal(cmd.Payload, &p) != nil {
			return reject("invalid_payload", "购股数据无效")
		}
		total, cost := 0, 0
		for name, count := range p.Stocks {
			company := state.Companies[name]
			if company == nil || count < 0 || count > 3 || count > company.StockTotal || (count > 0 && company.Tiles == 0) {
				return reject("invalid_stock", "股票数量或库存已变化")
			}
			total += count
			cost += count * company.StockPrice
		}
		if total > 3 || cost > state.Players[cmd.PlayerID].Money {
			return reject("invalid_stock", "最多购买三股，且不能超过现金余额")
		}
	case "game_merging_settle":
		var p game.MergingSettlePayload
		if json.Unmarshal(cmd.Payload, &p) != nil {
			return reject("invalid_payload", "合并数据无效")
		}
		pending := false
		for _, settlement := range state.MergeSettleData {
			pending = pending || slices.Contains(settlement.Hoders, cmd.PlayerID)
		}
		if !pending {
			return reject("already_settled", "你已完成本次结算")
		}
		exchanged := 0
		seen := map[string]bool{}
		for _, action := range p.Actions {
			settlement, exists := state.MergeSettleData[action.Company]
			holding := state.Players[cmd.PlayerID].Stocks[action.Company]
			if !exists || seen[action.Company] || !slices.Contains(settlement.Hoders, cmd.PlayerID) ||
				action.SellAmount < 0 || action.SellAmount > holding || action.ExchangeAmount < 0 ||
				action.ExchangeAmount > holding-action.SellAmount || action.ExchangeAmount%2 != 0 {
				return reject("invalid_settlement", "卖出与兑换数量超出持股，或兑换不是二换一")
			}
			seen[action.Company] = true
			exchanged += action.ExchangeAmount / 2
		}
		main := state.Companies[state.MergeMainCompany]
		if main == nil || exchanged > main.StockTotal {
			return reject("stock_unavailable", "存续公司库存已变化，请调整兑换数量")
		}
	}
	return true
}

func commandCompletedTurn(cmdType string, previous domain.RoomStatus, previousPlayer string, state *domain.GameState) bool {
	if state == nil {
		return false
	}
	if state.RoomStatus == domain.RoomStatusSetTile && previous != domain.RoomStatusSetTile {
		return true
	}
	if previousPlayer == "" {
		return false
	}
	if previousPlayer != state.CurrentPlayer {
		return true
	}
	if state.RoomStatus != domain.RoomStatusSetTile {
		return false
	}
	return cmdType == "turn_timeout" || cmdType == "game_place_tile"
}

func (r *RoomService) advancePhase(previous domain.RoomStatus, turnCompleted bool) {
	state := r.Room.State
	newTurn := state.RoomStatus == domain.RoomStatusSetTile && turnCompleted
	if previous != state.RoomStatus || newTurn {
		state.PhaseVersion++
	}
	if state.RoomStatus == domain.RoomStatusMatch {
		state.TurnNumber = 0
	} else if newTurn {
		state.TurnNumber++
	}
}

func (r *RoomService) resolveTimeout(cmd domain.Command) (domain.Command, bool) {
	if !r.validateCommand(cmd) || CurrentTimeoutPlayer(r.Room) != cmd.PlayerID ||
		r.Room.TurnDeadline.IsZero() || time.Now().Before(r.Room.TurnDeadline) {
		return domain.Command{}, false
	}
	action, ok := BuildTurnTimeoutCommand(r.Room, cmd.PlayerID)
	action.Conn = nil
	return action, ok
}

// Only immutable values cross into the timer goroutine. State and fallback
// actions are inspected/generated by the actor when the timeout arrives.
func (r *RoomService) armTimer() {
	roomcore.StopThinkTimer(r.svc)
	timeout, timed := turnTimeoutByStatus[r.Room.State.RoomStatus]
	playerID := CurrentTimeoutPlayer(r.Room)
	pc := r.Room.Connections[playerID]
	if !timed || pc == nil || pc.AI {
		return
	}
	version := r.Room.State.PhaseVersion
	r.Room.TurnTimeout = timeout
	r.Room.TurnDeadline = time.Now().Add(timeout)
	r.Room.ThinkTimer = time.AfterFunc(timeout, func() {
		select {
		case r.Room.CmdCh <- domain.Command{Type: "acquire_timeout", PlayerID: playerID, PhaseVersion: &version}:
		case <-r.Room.QuitCh:
		}
	})
}
