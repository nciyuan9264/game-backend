package roompkg

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	acgame "github.com/nciyuan9264/game-backend/internal/games/acquire/domain/game"
	"github.com/nciyuan9264/game-backend/pkg/database"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

// HistoryRepo 全局单例，由 init code 在 InitPostgres 后注入。
var HistoryRepo *gamehistory.Repo

const acquireMaxTiles = 108

// InitHistoryRepo 由 cmd/acquire 在 database.InitPostgres 后调用。
func InitHistoryRepo() {
	if database.DB == nil {
		logger.Error("InitHistoryRepo: database.DB is nil, history disabled")
		return
	}
	HistoryRepo = gamehistory.NewRepo(database.DB)
	if err := HistoryRepo.AutoMigrate(); err != nil {
		logger.Error("history AutoMigrate 失败", logger.F("error", err))
	} else {
		logger.Info("history AutoMigrate 成功")
	}
}

// startRecording 在开局时调用：构造内存录制器，缓存元信息与初始状态；
// 不写库。仅当 finishRecording（终局）时才事务落库。
func (r *RoomService) startRecording() {
	if HistoryRepo == nil {
		return
	}
	if r.Recorder != nil {
		// 已在录制中（防御）
		return
	}

	startedAt := r.Room.State.GameStartTime
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	// 序列化 initial_state 作为回放种子。
	initialJSON, err := gamehistory.MarshalToJSON(r.Room.State)
	if err != nil {
		logger.Error("序列化 initial_state 失败", logger.F("room_id", r.Room.ID), logger.F("error", err))
		return
	}

	g := &gamehistory.Game{
		RoomID:       r.Room.ID,
		GameType:     "acquire",
		StartedAt:    startedAt,
		MaxPlayers:   r.Room.State.MaxPlayers,
		InitialState: initialJSON,
	}

	// 收集玩家（按 PlayerID 排序，使 seat_index 稳定）。
	playerIDs := make([]string, 0, len(r.Room.Connections))
	for pid, pc := range r.Room.Connections {
		if pc != nil {
			playerIDs = append(playerIDs, pid)
		}
	}
	sort.Strings(playerIDs)

	players := make([]gamehistory.GamePlayer, 0, len(playerIDs))
	for i, pid := range playerIDs {
		pc := r.Room.Connections[pid]
		gp := gamehistory.GamePlayer{
			PlayerID:  pid,
			SeatIndex: i,
			IsAI:      pc.AI,
		}
		if !pc.AI {
			if uid, ok := gamehistory.ParseUserIDFromPlayerID(pid); ok {
				gp.UserID = &uid
			}
		}
		players = append(players, gp)
	}

	r.Recorder = gamehistory.NewRecorder(HistoryRepo, g, players)
	r.HistoryStartedAt = startedAt
	r.HistorySeq = 0
	logger.Info("history recording started (in-memory)",
		logger.F("room_id", r.Room.ID))
}

// recordEvent 把命令写入内存事件缓冲（白名单过滤），并附带该命令处理完毕后的权威状态快照。
// 快照（state + result）用于回放时直接渲染，避免重跑规则 handler 导致状态分叉。
func (r *RoomService) recordEvent(cmd domain.Command) {
	if r.Recorder == nil {
		return
	}
	if !gamehistory.IsRecordableCmd(cmd.Type) {
		return
	}
	// RecomputeDerivedState 为纯计算、幂等，可安全重复调用；取与 ROOM_SYNC 一致的 result。
	result, _ := acgame.RecomputeDerivedState(r.Room)
	// 编码为压缩 + 瘦身后的状态字节（BoardTiles 仅存非空地格子，回放时补齐）。
	blob, err := acgame.EncodeStateSnapshot(r.Room.State, result)
	if err != nil {
		logger.Error("编码 state_blob 失败", logger.F("room_id", r.Room.ID), logger.F("error", err))
		blob = nil // 退化为无快照，回放回退重算
	}
	r.Recorder.OnEventWithState(r.HistorySeq, cmd.Type, cmd.PlayerID, []byte(cmd.Payload), blob)
	r.HistorySeq++
}

func (r *RoomService) shouldRecordAbandoned() bool {
	if r == nil || r.Room == nil || r.Room.State == nil {
		return false
	}
	if r.Room.State.GameStartTime.IsZero() {
		return false
	}
	placedTiles := 0
	for _, tile := range r.Room.State.BoardTiles {
		if tile != nil && tile.Belong != "" {
			placedTiles++
		}
	}
	return placedTiles*2 >= acquireMaxTiles
}

func acquirePlacedTiles(r *domain.Room) int {
	if r == nil || r.State == nil {
		return 0
	}
	placedTiles := 0
	for _, tile := range r.State.BoardTiles {
		if tile != nil && tile.Belong != "" {
			placedTiles++
		}
	}
	return placedTiles
}

type acquireScoreItem struct {
	playerID string
	money    int
	stocks   int
	total    int
}

func acquireScoreItems(r *domain.Room) []acquireScoreItem {
	if r == nil || r.State == nil {
		return nil
	}
	companyInfoMap := r.State.Companies
	scores := make([]acquireScoreItem, 0, len(r.Connections))
	for pid, pc := range r.Connections {
		if pc == nil {
			continue
		}
		ps, ok := r.State.Players[pid]
		if !ok || ps == nil {
			continue
		}
		money := ps.Money
		stocks := 0
		for company, count := range ps.Stocks {
			if c, ok := companyInfoMap[company]; ok && c != nil {
				stocks += count * c.StockPrice
			}
		}
		scores = append(scores, acquireScoreItem{
			playerID: pid,
			money:    money,
			stocks:   stocks,
			total:    money + stocks,
		})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].total != scores[j].total {
			return scores[i].total > scores[j].total
		}
		return scores[i].playerID < scores[j].playerID
	})
	return scores
}

func buildAcquireFinalPlayers(r *domain.Room, abandoned bool) ([]gamehistory.GamePlayer, string, *int64) {
	scores := acquireScoreItems(r)
	winnerPlayerID := ""
	var winnerUserID *int64
	if !abandoned && len(scores) > 0 {
		winnerPlayerID = scores[0].playerID
		if pc := r.Connections[winnerPlayerID]; pc != nil && !pc.AI {
			if uid, ok := gamehistory.ParseUserIDFromPlayerID(winnerPlayerID); ok {
				winnerUserID = &uid
			}
		}
	}

	playersIn := make([]gamehistory.GamePlayer, 0, len(scores))
	rank := 0
	prevTotal := 0
	abandonedRank := len(scores)
	for i, s := range scores {
		if abandoned {
			rank = abandonedRank
		} else if i == 0 || s.total != prevTotal {
			rank = i + 1
			prevTotal = s.total
		}
		money := s.money
		score := s.total
		stocks := s.stocks
		finalRank := rank
		playersIn = append(playersIn, gamehistory.GamePlayer{
			PlayerID:    s.playerID,
			FinalMoney:  &money,
			FinalScore:  &score,
			FinalStocks: stocks,
			FinalRank:   &finalRank,
			IsWinner:    !abandoned && s.playerID == winnerPlayerID,
		})
	}
	return playersIn, winnerPlayerID, winnerUserID
}

// finishRecording 终局时调用：算 winner / final_score，并把整局事务性落库。
func (r *RoomService) finishRecording() {
	if r.Recorder == nil {
		return
	}

	// 复用 RecomputeDerivedState 拿到与 ROOM_SYNC 一致的 result（playerID -> {money, stocks, total}）。
	result, _ := acgame.RecomputeDerivedState(r.Room)

	finalResultJSON, err := json.Marshal(result)
	if err != nil {
		logger.Error("序列化 final_result 失败", logger.F("room_id", r.Room.ID), logger.F("error", err))
		finalResultJSON = nil
	}

	endedAt := time.Now()
	startedAt := r.HistoryStartedAt
	if startedAt.IsZero() {
		startedAt = endedAt
	}
	duration := int(endedAt.Sub(startedAt).Seconds())

	playersIn, winnerPlayerID, winnerUserID := buildAcquireFinalPlayers(r.Room, false)

	gameID, err := r.Recorder.Commit(context.Background(), gamehistory.CommitInput{
		EndedAt:         endedAt,
		DurationSeconds: duration,
		WinnerUserID:    winnerUserID,
		WinnerPlayerID:  winnerPlayerID,
		EndReason:       gamehistory.EndReasonCompleted,
		FinalResult:     finalResultJSON,
		Players:         playersIn,
	})
	if err != nil {
		logger.Error("history finish failed",
			logger.F("room_id", r.Room.ID),
			logger.F("error", err))
	}

	r.Recorder = nil
	r.HistorySeq = 0
	r.HistoryEnded = true
	logger.Info("history recording finished",
		logger.F("game_id", gameID),
		logger.F("room_id", r.Room.ID),
		logger.F("winner", winnerPlayerID),
		logger.F("duration_s", duration))
}

func (r *RoomService) finishAbandonedRecording(now time.Time) {
	if r.Recorder == nil {
		return
	}
	if r.HistoryEnded {
		return
	}
	if !r.shouldRecordAbandoned() {
		r.stopRecording()
		return
	}

	result, _ := acgame.RecomputeDerivedState(r.Room)
	finalResult := map[string]any{
		"abandoned": true,
		"reason":    "no_human_after_progress_half",
		"progress": map[string]int{
			"placedTiles": acquirePlacedTiles(r.Room),
			"maxTiles":    acquireMaxTiles,
		},
		"snapshot": result,
	}
	finalResultJSON, err := json.Marshal(finalResult)
	if err != nil {
		logger.Error("序列化 abandoned final_result 失败", logger.F("room_id", r.Room.ID), logger.F("error", err))
		finalResultJSON = nil
	}

	startedAt := r.HistoryStartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	duration := int(now.Sub(startedAt).Seconds())
	playersIn, winnerPlayerID, winnerUserID := buildAcquireFinalPlayers(r.Room, true)

	gameID, err := r.Recorder.Commit(context.Background(), gamehistory.CommitInput{
		EndedAt:         now,
		DurationSeconds: duration,
		WinnerUserID:    winnerUserID,
		WinnerPlayerID:  winnerPlayerID,
		EndReason:       gamehistory.EndReasonAbandoned,
		FinalResult:     finalResultJSON,
		Players:         playersIn,
	})
	if err != nil {
		logger.Error("history abandoned finish failed",
			logger.F("room_id", r.Room.ID),
			logger.F("error", err))
	}

	r.Recorder = nil
	r.HistorySeq = 0
	r.HistoryEnded = true
	logger.Info("history abandoned recording finished",
		logger.F("game_id", gameID),
		logger.F("room_id", r.Room.ID),
		logger.F("duration_s", duration))
}

// stopRecording 房间销毁/重开时调用：丢弃内存中的未终局对局，不写库。
func (r *RoomService) stopRecording() {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Discard()
	r.Recorder = nil
}
