// Package replay 提供基于 game_events 的对局回放引擎。
//
// 接受 initial_state + 事件序列，构造一个内存离线 Room，依次将 events 应用到 Room 上，
// 复用 game 包里现有的命令处理函数（HandlePlaceTileMessage 等），最终返回某一步的 GameState 快照。
package replay

import (
	"encoding/json"
	"fmt"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	acgame "github.com/nciyuan9264/game-backend/internal/games/acquire/domain/game"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// Snapshot 单步快照，结构与 ROOM_SYNC 同形，方便前端复用渲染。
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

// ReplayTo 把 events[0..targetSeq] 应用到由 initialState 反序列化构造出的 Room 上，返回快照。
//
// targetSeq < 0 视为"只渲染初始状态，不应用任何事件"。
// targetSeq >= len(events) 视为应用全部事件。
func ReplayTo(g *gamehistory.Game, players []gamehistory.GamePlayer, events []gamehistory.GameEvent, targetSeq int) (*Snapshot, error) {
	if g == nil {
		return nil, fmt.Errorf("game is nil")
	}

	// 1. 反序列化 initial_state。
	state := &domain.GameState{}
	if len(g.InitialState) > 0 {
		if err := json.Unmarshal(g.InitialState, state); err != nil {
			return nil, fmt.Errorf("反序列化 initial_state 失败: %w", err)
		}
	}
	if state.BoardTiles == nil {
		state.BoardTiles = map[string]*domain.Tile{}
	}
	if state.Players == nil {
		state.Players = map[string]*domain.PlayerState{}
	}
	if state.Companies == nil {
		state.Companies = map[string]*domain.CompanyState{}
	}

	// 2. 构造离线 Room。Connections 用来让 handler 遍历玩家。
	base := roomcore.NewBase(g.RoomID, 0)
	for _, p := range players {
		base.Connections[p.PlayerID] = &roomcore.PlayerConn{
			PlayerID: p.PlayerID,
			Conn:     nil,
			Online:   false,
			AI:       p.IsAI,
		}
		base.PlayerSeq = append(base.PlayerSeq, p.PlayerID)
	}
	r := &domain.Room{Base: base, State: state}

	// 快照优先路径：新对局直接读权威 state_snapshot 组装目标帧，不重跑 handler。
	if allHaveSnapshot(events) {
		limit := targetSeq + 1
		if limit > len(events) {
			limit = len(events)
		}
		if limit < 0 {
			limit = 0
		}
		if limit == 0 {
			// 初始态：用 initial_state 重算 result。
			initResult, _ := acgame.RecomputeDerivedState(r)
			return buildSnapshotFromState(state, initResult, base, events, targetSeq, 0), nil
		}
		if st, res, ok := decodeSnapshot(events[limit-1]); ok {
			return buildSnapshotFromState(st, res, base, events, events[limit-1].Seq, limit), nil
		}
		// decode 失败兜底：落到下方重算路径。
	}

	// 3. 预扫描所有 events，按 playerID 收集未来要打出的 tile 序列；
	// 每次 placeTile 后据此把该玩家手牌覆盖为"接下来 5 步会打出的 tile"。
	// 这样可以在 schema 不存补牌信息的前提下，让 snapshot.tiles 显示出更贴近真实的 5 张。
	futureByPlayer := make(map[string][]string)
	for _, ev := range events {
		if ev.CmdType != "game_place_tile" {
			continue
		}
		var p struct {
			TileKey string `json:"tileKey"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil || p.TileKey == "" {
			continue
		}
		futureByPlayer[ev.PlayerID] = append(futureByPlayer[ev.PlayerID], p.TileKey)
	}
	cursors := make(map[string]int, len(futureByPlayer))

	// 4. 依次应用事件。
	limit := targetSeq + 1
	if limit > len(events) {
		limit = len(events)
	}
	if limit < 0 {
		limit = 0
	}

	for i := 0; i < limit; i++ {
		e := events[i]
		applyEvent(r, e)
		if e.CmdType == "game_place_tile" {
			cursors[e.PlayerID]++
		}
		// 与生产 BroadcastToRoom 行为对齐：每条命令处理后立即同步 derived state，
		// 让下一条 buy_stock / merging_settle 读到正确的 StockPrice / StockTotal / Tiles。
		acgame.RecomputeDerivedState(r)
	}

	return buildSnapshot(r, state, base, events, futureByPlayer, cursors, targetSeq, limit), nil
}

// ReplayAll 单趟重放整局，产出每一步的快照数组（含初始态 seq=-1）。
//
// 与逐帧调用 ReplayTo 等价，但只反序列化 / 构造 Room / 预扫描一次，避免 O(N²) 的重复重放。
// 数组首元素为初始态（targetSeq=-1，应用 0 个事件），其后每个事件一帧。
func ReplayAll(g *gamehistory.Game, players []gamehistory.GamePlayer, events []gamehistory.GameEvent) ([]*Snapshot, error) {
	if g == nil {
		return nil, fmt.Errorf("game is nil")
	}

	// 1. 反序列化 initial_state。
	state := &domain.GameState{}
	if len(g.InitialState) > 0 {
		if err := json.Unmarshal(g.InitialState, state); err != nil {
			return nil, fmt.Errorf("反序列化 initial_state 失败: %w", err)
		}
	}
	if state.BoardTiles == nil {
		state.BoardTiles = map[string]*domain.Tile{}
	}
	if state.Players == nil {
		state.Players = map[string]*domain.PlayerState{}
	}
	if state.Companies == nil {
		state.Companies = map[string]*domain.CompanyState{}
	}

	// 2. 构造离线 Room。
	base := roomcore.NewBase(g.RoomID, 0)
	for _, p := range players {
		base.Connections[p.PlayerID] = &roomcore.PlayerConn{
			PlayerID: p.PlayerID,
			Conn:     nil,
			Online:   false,
			AI:       p.IsAI,
		}
		base.PlayerSeq = append(base.PlayerSeq, p.PlayerID)
	}
	r := &domain.Room{Base: base, State: state}

	// 快照优先路径：若所有事件都带权威 state_snapshot（新对局），直接用快照逐帧组装，
	// 不重跑 handler，从根本上消除状态分叉。
	if allHaveSnapshot(events) {
		snaps := make([]*Snapshot, 0, len(events)+1)
		// 初始态（应用 0 个事件，seq=-1）：用 initial_state 重算 result。
		initResult, _ := acgame.RecomputeDerivedState(r)
		snaps = append(snaps, buildSnapshotFromState(state, initResult, base, events, -1, 0))
		for i := 0; i < len(events); i++ {
			st, res, ok := decodeSnapshot(events[i])
			if !ok {
				// 理论上不会发生（allHaveSnapshot 已校验非空），稳一手：回退重算路径。
				return replayAllByReapply(r, state, base, events)
			}
			snaps = append(snaps, buildSnapshotFromState(st, res, base, events, events[i].Seq, i+1))
		}
		return snaps, nil
	}

	// 回退路径：老对局无快照，重跑 handler 重建状态。
	return replayAllByReapply(r, state, base, events)
}

// replayAllByReapply 老对局回退路径：逐条重跑规则 handler 重建状态并产出每帧快照。
// 注意：该路径含非确定性（map 迭代/不稳定排序），可能与真实对局分叉，仅用于无快照的历史对局。
func replayAllByReapply(r *domain.Room, state *domain.GameState, base *roomcore.Base, events []gamehistory.GameEvent) ([]*Snapshot, error) {
	// 3. 预扫描 future placeTile 序列。
	futureByPlayer := make(map[string][]string)
	for _, ev := range events {
		if ev.CmdType != "game_place_tile" {
			continue
		}
		var p struct {
			TileKey string `json:"tileKey"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil || p.TileKey == "" {
			continue
		}
		futureByPlayer[ev.PlayerID] = append(futureByPlayer[ev.PlayerID], p.TileKey)
	}
	cursors := make(map[string]int, len(futureByPlayer))

	snaps := make([]*Snapshot, 0, len(events)+1)

	// 初始态（应用 0 个事件，seq=-1）。
	acgame.RecomputeDerivedState(r)
	snaps = append(snaps, buildSnapshot(r, state, base, events, futureByPlayer, cursors, -1, 0))

	// 逐步推进，每步产出一帧。
	for i := 0; i < len(events); i++ {
		e := events[i]
		// 分叉诊断：记录应用前的关键状态指纹。
		prevStatus := r.State.RoomStatus
		prevPlayer := r.State.CurrentPlayer
		prevLastTile := r.State.LastTileKey

		applyEvent(r, e)
		if e.CmdType == "game_place_tile" {
			cursors[e.PlayerID]++
		}
		acgame.RecomputeDerivedState(r)

		// 若白名单命令应用后三项关键状态均未变，疑似被守卫拒绝 → 分叉。
		if gamehistory.IsRecordableCmd(e.CmdType) &&
			r.State.RoomStatus == prevStatus &&
			r.State.CurrentPlayer == prevPlayer &&
			r.State.LastTileKey == prevLastTile {
			logger.Warn("replay 疑似分叉：事件未推进状态",
				logger.F("room_id", r.ID),
				logger.F("seq", e.Seq),
				logger.F("cmd_type", e.CmdType),
				logger.F("player_id", e.PlayerID),
				logger.F("status", string(r.State.RoomStatus)))
		}

		snaps = append(snaps, buildSnapshot(r, state, base, events, futureByPlayer, cursors, e.Seq, i+1))
	}

	return snaps, nil
}

// allHaveSnapshot 判定该局是否为"带权威快照"的对局：当且仅当所有事件都带非空快照
// （新对局的 StateBlob 或老对局的 StateSnapshot）。任一缺失则回退"重跑 handler"逻辑。
func allHaveSnapshot(events []gamehistory.GameEvent) bool {
	for i := range events {
		if len(events[i].StateBlob) == 0 && len(events[i].StateSnapshot) == 0 {
			return false
		}
	}
	return true
}

// decodeSnapshot 反序列化单条事件的权威快照：优先用新版压缩列 StateBlob，
// 回退到旧版未压缩列 StateSnapshot。两者都通过 acgame.DecodeStateSnapshot 统一解码 + 补齐棋盘。
func decodeSnapshot(e gamehistory.GameEvent) (*domain.GameState, map[string]interface{}, bool) {
	if len(e.StateBlob) > 0 {
		return acgame.DecodeStateSnapshot(e.StateBlob)
	}
	if len(e.StateSnapshot) > 0 {
		return acgame.DecodeStateSnapshot(e.StateSnapshot)
	}
	return nil, nil, false
}

// buildSnapshotFromState 直接用权威 state + result 组装一帧快照，
// 不重跑 handler、不做 future-tile 推断（快照里已是处理后的最终态），
// 输出结构与 buildSnapshot 完全一致，保证前端零改动。
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
			"companyInfo":   state.Companies,
			"currentPlayer": state.CurrentPlayer,
			"gameStatus":    state.RoomStatus,
			"tiles":         state.BoardTiles,
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

// buildSnapshot 用当前 Room 状态组装一帧快照。
//
// targetSeq 为该帧对应的事件 seq（初始态为 -1）；limit 为已应用的事件数量，用于定位 CurrentEvent。
// 同时会按 cursors 用 future 序列覆盖每个玩家的"未来 5 张牌"。
func buildSnapshot(r *domain.Room, state *domain.GameState, base *roomcore.Base, events []gamehistory.GameEvent, futureByPlayer map[string][]string, cursors map[string]int, targetSeq, limit int) *Snapshot {
	// 用 future 序列覆盖每个玩家的"未来 5 张牌"——已被打出的不算，
	// 已经在棋盘上的（避免重复显示）也跳过。
	const handSize = 5
	for pid, ps := range state.Players {
		if ps == nil {
			continue
		}
		future := futureByPlayer[pid]
		cur := cursors[pid]
		hand := make([]string, 0, handSize)
		for j := cur; j < len(future) && len(hand) < handSize; j++ {
			tk := future[j]
			// 跳过已经在棋盘上的 tile（理论上 future 之前的都已打出，但稳一手）。
			if t, ok := state.BoardTiles[tk]; ok && t != nil && t.Belong != "" {
				continue
			}
			hand = append(hand, tk)
		}
		ps.Tiles = hand
	}

	// 重算派生状态（公司股价、终局判定、result）。
	result, _ := acgame.RecomputeDerivedState(r)

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
			"companyInfo":   state.Companies,
			"currentPlayer": state.CurrentPlayer,
			"gameStatus":    state.RoomStatus,
			"tiles":         state.BoardTiles,
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

// cloneStateMap 通过 JSON 往返做一次深拷贝，切断快照与仍在被回放循环持续 mutate 的
// state（state.Companies / state.BoardTiles / state.Players 指针）之间的引用，
// 否则 ReplayAll 返回的所有帧会共享同一份 state，最终全部渲染成最后一回合的数据。
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

// applyEvent 把单条事件应用到 Room 上。直接复用 game 包内现有的命令处理函数。
func applyEvent(r *domain.Room, e gamehistory.GameEvent) {
	cmd := domain.Command{
		Type:     e.CmdType,
		PlayerID: e.PlayerID,
		Payload:  json.RawMessage(e.Payload),
	}

	// placeTile 之前显式补齐"该步要打出的 tile"，
	// 避免因为 replay 中玩家 tiles 与原局不一致导致 SafeSliceRemove 静默失败。
	// 注：玩家手牌的最终展示由 ReplayTo 在事件循环结束后用 future 序列统一覆盖，
	// 这里只是确保 placeTile handler 内部能正确执行 -1 的逻辑。
	if e.CmdType == "game_place_tile" {
		var p struct {
			TileKey string `json:"tileKey"`
		}
		if err := json.Unmarshal(cmd.Payload, &p); err == nil && p.TileKey != "" {
			if ps, ok := r.State.Players[e.PlayerID]; ok && ps != nil {
				found := false
				for _, t := range ps.Tiles {
					if t == p.TileKey {
						found = true
						break
					}
				}
				if !found {
					ps.Tiles = append(ps.Tiles, p.TileKey)
				}
			}
		}
	}

	switch e.CmdType {
	case "game_place_tile":
		acgame.HandlePlaceTileMessage(r, cmd)
	case "game_create_company":
		acgame.HandleCreateCompanyMessage(r, cmd)
	case "game_merging_selection":
		acgame.HandleMergingSelectionMessage(r, cmd)
	case "game_merging_settle":
		acgame.HandleMergingSettleMessage(r, cmd)
	case "game_buy_stock":
		acgame.HandleBuyStockMessage(r, cmd)
	case "turn_timeout":
		// 思考超时由 game 层在 BuildTimeoutCommand 时映射为具体动作；
		// 第一版 acquire 暂未启用超时映射，这里忽略。
	}
}
