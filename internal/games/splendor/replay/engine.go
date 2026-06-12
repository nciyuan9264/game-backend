// Package replay 提供基于 game_events 的对局回放引擎。
//
// 接受 initial_state + 事件序列，构造一个内存离线 Room，依次将 events 应用到 Room 上，
// 复用 game 包里现有的命令处理函数，最终返回某一步的 GameState 快照。
// 新对局每条事件都带权威快照（StateBlob），回放时直接解码快照，从根本上消除状态分叉。
package replay

import (
	"encoding/json"
	"fmt"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	spgame "github.com/nciyuan9264/game-backend/internal/games/splendor/domain/game"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// Snapshot 单步快照，结构与 sync 消息同形，方便前端复用渲染。
type Snapshot struct {
	Seq          int                    `json:"seq"`
	TotalEvents  int                    `json:"totalEvents"`
	CurrentEvent *EventInfo             `json:"currentEvent,omitempty"`
	RoomData     map[string]interface{} `json:"roomData"`
	PlayersData  map[string]interface{} `json:"playersData"`
	Result       map[string]interface{} `json:"result"`
}

// EventInfo 简短事件信息。
type EventInfo struct {
	Seq      int             `json:"seq"`
	PlayerID string          `json:"playerID"`
	CmdType  string          `json:"cmdType"`
	Payload  json.RawMessage `json:"payload"`
}

// ReplayTo 把 events[0..targetSeq] 应用后返回快照。targetSeq < 0 表示只渲染初始状态。
func ReplayTo(g *gamehistory.Game, players []gamehistory.GamePlayer, events []gamehistory.GameEvent, targetSeq int) (*Snapshot, error) {
	r, state, base, err := buildOfflineRoom(g, players)
	if err != nil {
		return nil, err
	}

	// 快照优先路径：新对局直接读权威 state_blob 组装目标帧，不重跑 handler。
	if allHaveSnapshot(events) {
		limit := clampLimit(targetSeq, len(events))
		if limit == 0 {
			initResult := spgame.RecomputeDerivedState(r)
			return buildSnapshotFromState(state, initResult, base, events, targetSeq, 0), nil
		}
		if st, res, ok := decodeSnapshot(events[limit-1]); ok {
			return buildSnapshotFromState(st, res, base, events, events[limit-1].Seq, limit), nil
		}
		// decode 失败：落到下方重算路径。
	}

	limit := clampLimit(targetSeq, len(events))
	for i := 0; i < limit; i++ {
		applyEvent(r, events[i])
		spgame.RecomputeDerivedState(r)
	}
	result := spgame.RecomputeDerivedState(r)
	return buildSnapshotFromState(state, result, base, events, targetSeq, limit), nil
}

// ReplayAll 单趟重放整局，产出每一步的快照数组（含初始态 seq=-1）。
func ReplayAll(g *gamehistory.Game, players []gamehistory.GamePlayer, events []gamehistory.GameEvent) ([]*Snapshot, error) {
	r, state, base, err := buildOfflineRoom(g, players)
	if err != nil {
		return nil, err
	}

	if allHaveSnapshot(events) {
		snaps := make([]*Snapshot, 0, len(events)+1)
		initResult := spgame.RecomputeDerivedState(r)
		snaps = append(snaps, buildSnapshotFromState(state, initResult, base, events, -1, 0))
		for i := 0; i < len(events); i++ {
			st, res, ok := decodeSnapshot(events[i])
			if !ok {
				return replayAllByReapply(g, players, events)
			}
			snaps = append(snaps, buildSnapshotFromState(st, res, base, events, events[i].Seq, i+1))
		}
		return snaps, nil
	}

	return replayAllByReapply(g, players, events)
}

// replayAllByReapply 老对局回退路径：重跑 handler 重建状态并产出每帧快照。
func replayAllByReapply(g *gamehistory.Game, players []gamehistory.GamePlayer, events []gamehistory.GameEvent) ([]*Snapshot, error) {
	r, state, base, err := buildOfflineRoom(g, players)
	if err != nil {
		return nil, err
	}
	snaps := make([]*Snapshot, 0, len(events)+1)
	initResult := spgame.RecomputeDerivedState(r)
	snaps = append(snaps, buildSnapshotFromState(cloneState(state), initResult, base, events, -1, 0))
	for i := 0; i < len(events); i++ {
		applyEvent(r, events[i])
		result := spgame.RecomputeDerivedState(r)
		snaps = append(snaps, buildSnapshotFromState(cloneState(state), result, base, events, events[i].Seq, i+1))
	}
	return snaps, nil
}

// buildOfflineRoom 反序列化 initial_state 并构造一个离线 Room。
func buildOfflineRoom(g *gamehistory.Game, players []gamehistory.GamePlayer) (*domain.Room, *domain.GameState, *roomcore.Base, error) {
	if g == nil {
		return nil, nil, nil, fmt.Errorf("game is nil")
	}
	state := &domain.GameState{}
	if len(g.InitialState) > 0 {
		if err := json.Unmarshal(g.InitialState, state); err != nil {
			return nil, nil, nil, fmt.Errorf("反序列化 initial_state 失败: %w", err)
		}
	}
	ensureMaps(state)

	base := roomcore.NewBase(g.RoomID, 0)
	for _, p := range players {
		base.Connections[p.PlayerID] = &roomcore.PlayerConn{
			PlayerID: p.PlayerID,
			Online:   false,
			AI:       p.IsAI,
		}
		base.PlayerSeq = append(base.PlayerSeq, p.PlayerID)
	}
	r := &domain.Room{Base: base, State: state}
	return r, state, base, nil
}

func ensureMaps(state *domain.GameState) {
	if state.NormalCards == nil {
		state.NormalCards = map[string]*entities.NormalCard{}
	}
	if state.NobleCards == nil {
		state.NobleCards = map[string]*entities.NobleCard{}
	}
	if state.Gems == nil {
		state.Gems = map[string]int{}
	}
	if state.Players == nil {
		state.Players = map[string]*domain.PlayerState{}
	}
}

func clampLimit(targetSeq, n int) int {
	limit := targetSeq + 1
	if limit > n {
		limit = n
	}
	if limit < 0 {
		limit = 0
	}
	return limit
}

func allHaveSnapshot(events []gamehistory.GameEvent) bool {
	for i := range events {
		if len(events[i].StateBlob) == 0 && len(events[i].StateSnapshot) == 0 {
			return false
		}
	}
	return true
}

func decodeSnapshot(e gamehistory.GameEvent) (*domain.GameState, map[string]interface{}, bool) {
	if len(e.StateBlob) > 0 {
		return spgame.DecodeStateSnapshot(e.StateBlob)
	}
	if len(e.StateSnapshot) > 0 {
		return spgame.DecodeStateSnapshot(e.StateSnapshot)
	}
	return nil, nil, false
}

// buildSnapshotFromState 用权威 state + result 组装一帧快照。
func buildSnapshotFromState(state *domain.GameState, result map[string]interface{}, base *roomcore.Base, events []gamehistory.GameEvent, targetSeq, limit int) *Snapshot {
	playersInfo := make(map[string]interface{}, len(base.Connections))
	for _, p := range base.Connections {
		playersInfo[p.PlayerID] = map[string]interface{}{
			"playerID": p.PlayerID,
			"online":   p.Online,
			"ai":       p.AI,
		}
	}

	playersData := make(map[string]interface{}, len(state.Players))
	for pid, ps := range state.Players {
		playersData[pid] = ps
	}

	snap := &Snapshot{
		Seq:         targetSeq,
		TotalEvents: len(events),
		RoomData: cloneStateMap(map[string]interface{}{
			"normalCards":   state.NormalCards,
			"nobleCards":    state.NobleCards,
			"gems":          state.Gems,
			"currentPlayer": state.CurrentPlayer,
			"gameStatus":    state.RoomStatus,
			"lastData":      state.LastData,
			"players":       playersInfo,
		}),
		PlayersData: cloneStateMap(playersData),
		Result:      result,
	}

	if limit > 0 && limit-1 < len(events) {
		ce := events[limit-1]
		snap.CurrentEvent = &EventInfo{
			Seq:      ce.Seq,
			PlayerID: ce.PlayerID,
			CmdType:  ce.CmdType,
			Payload:  json.RawMessage(ce.Payload),
		}
	}

	return snap
}

// cloneStateMap 通过 JSON 往返做一次深拷贝，切断快照与仍在被回放循环 mutate 的 state 之间的引用。
func cloneStateMap(m map[string]interface{}) map[string]interface{} {
	b, err := json.Marshal(m)
	if err != nil {
		return m
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(b, &out); err != nil {
		return m
	}
	return out
}

// cloneState 深拷贝整个 GameState（用于重算路径逐帧产出独立快照）。
func cloneState(s *domain.GameState) *domain.GameState {
	b, err := json.Marshal(s)
	if err != nil {
		return s
	}
	out := &domain.GameState{}
	if err := json.Unmarshal(b, out); err != nil {
		return s
	}
	return out
}

// applyEvent 把单条事件应用到 Room 上，复用 game 包内现有的命令处理函数。
func applyEvent(r *domain.Room, e gamehistory.GameEvent) {
	cmd := domain.Command{
		Type:     e.CmdType,
		PlayerID: e.PlayerID,
		Payload:  json.RawMessage(e.Payload),
	}
	switch e.CmdType {
	case "game_get_gem":
		spgame.HandleGetGemMessage(r, cmd)
	case "game_buy_card":
		spgame.HandleBuyCardMessage(r, cmd)
	case "game_preserve_card":
		spgame.HandleReserveCardMessage(r, cmd)
	case "turn_timeout":
		spgame.HandleTurnTimeoutMessage(r, cmd)
	}
}
