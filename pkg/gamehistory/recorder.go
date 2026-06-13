package gamehistory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nciyuan9264/game-backend/pkg/logger"
	"gorm.io/datatypes"
)

const (
	// commitMaxAttempts Commit 落库的最大尝试次数（含首次）。
	commitMaxAttempts = 3
	// commitBaseBackoff 首次重试前的等待时长，之后按指数退避（200ms / 400ms / 800ms ...）。
	commitBaseBackoff = 200 * time.Millisecond
	// failedDumpDir 落库最终失败时，整局事件兜底落盘目录。
	failedDumpDir = "./game_logs/history_failed"
)

// IsRecordableCmd returns whether an Acquire command should be stored for replay.
func IsRecordableCmd(cmdType string) bool {
	switch cmdType {
	case "game_place_tile",
		"game_create_company",
		"game_merging_selection",
		"game_merging_settle",
		"game_buy_stock",
		"turn_timeout":
		return true
	}
	return false
}

// Recorder buffers one game's events in memory and commits only on completion.
type Recorder struct {
	repo *Repo

	game    *Game
	players []GamePlayer

	mu        sync.Mutex
	events    []*GameEvent
	committed bool
	discarded bool
}

func NewRecorder(repo *Repo, game *Game, players []GamePlayer) *Recorder {
	return &Recorder{
		repo:    repo,
		game:    game,
		players: players,
		events:  make([]*GameEvent, 0, 128),
	}
}

func (r *Recorder) OnEvent(seq int, cmdType, playerID string, payload []byte) {
	r.OnEventWithState(seq, cmdType, playerID, payload, nil)
}

// OnEventWithState 在 OnEvent 基础上额外保存该命令处理完毕后的权威状态快照。
// stateBlob 为已编码（gzip 压缩 + 瘦身）的状态字节，写入 StateBlob 列；为 nil 时退化为不带快照。
func (r *Recorder) OnEventWithState(seq int, cmdType, playerID string, payload, stateBlob []byte) {
	if r == nil {
		return
	}
	pl := payload
	if len(pl) == 0 {
		pl = []byte("null")
	}
	e := &GameEvent{
		Seq:        seq,
		OccurredAt: time.Now(),
		PlayerID:   playerID,
		CmdType:    cmdType,
		Payload:    datatypes.JSON(pl),
	}
	if len(stateBlob) > 0 {
		e.StateBlob = stateBlob
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.committed || r.discarded {
		return
	}
	r.events = append(r.events, e)
}

type CommitInput struct {
	EndedAt         time.Time
	DurationSeconds int
	WinnerUserID    *int64
	WinnerPlayerID  string
	EndReason       string
	FinalResult     []byte
	Players         []GamePlayer
}

func (r *Recorder) Commit(ctx context.Context, in CommitInput) (int64, error) {
	r.mu.Lock()
	if r.committed || r.discarded {
		r.mu.Unlock()
		return 0, nil
	}
	// 注意：先不置 committed、不清空 events。只有落库成功后才置位，
	// 否则一旦失败，事件被清空且 committed=true，将永远无法重试。
	events := r.events
	r.mu.Unlock()

	r.game.EndedAt = &in.EndedAt
	r.game.DurationSeconds = in.DurationSeconds
	r.game.WinnerUserID = in.WinnerUserID
	r.game.WinnerPlayerID = in.WinnerPlayerID
	if in.EndReason != "" {
		r.game.EndReason = in.EndReason
	}
	if len(in.FinalResult) > 0 {
		r.game.FinalResult = datatypes.JSON(in.FinalResult)
	}

	finalByPID := make(map[string]GamePlayer, len(in.Players))
	for _, p := range in.Players {
		finalByPID[p.PlayerID] = p
	}
	for i := range r.players {
		if fp, ok := finalByPID[r.players[i].PlayerID]; ok {
			r.players[i].FinalScore = fp.FinalScore
			r.players[i].FinalMoney = fp.FinalMoney
			r.players[i].FinalStocks = fp.FinalStocks
			r.players[i].FinalRank = fp.FinalRank
			r.players[i].IsWinner = fp.IsWinner
		}
	}

	// 有限次指数退避重试：games + players + events 仍是单个原子事务（见 SaveCompletedGame）。
	var gameID int64
	var err error
	for attempt := 1; attempt <= commitMaxAttempts; attempt++ {
		gameID, err = r.repo.SaveCompletedGame(ctx, r.game, r.players, events)
		if err == nil {
			break
		}
		logger.Error("history commit failed",
			logger.F("room_id", r.game.RoomID),
			logger.F("attempt", attempt),
			logger.F("max_attempts", commitMaxAttempts),
			logger.F("error", err))
		if attempt < commitMaxAttempts {
			// 退避；若 ctx 已取消则立即放弃。
			backoff := commitBaseBackoff * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(backoff):
				// 重试前重置自增主键，避免上次失败事务残留的 ID 影响本次插入。
				r.game.ID = 0
				continue
			}
			break
		}
	}

	if err != nil {
		// 最终失败：兜底把整局事件（含 state_snapshot）落盘，便于事后补录。
		r.dumpFailedGame(events, err)
		// 标记 committed，避免上层反复重试浪费资源；事件已落盘，可离线补。
		r.mu.Lock()
		r.committed = true
		r.events = nil
		r.mu.Unlock()
		return 0, err
	}

	// 成功后才置位并释放事件缓冲。
	r.mu.Lock()
	r.committed = true
	r.events = nil
	r.mu.Unlock()

	logger.Info("history committed",
		logger.F("game_id", gameID),
		logger.F("room_id", r.game.RoomID),
		logger.F("events", len(events)))
	return gameID, nil
}

// dumpFailedGame 在落库最终失败时，把整局元数据 + 玩家 + 事件（含 state_snapshot）
// 以 JSON 落到 failedDumpDir，便于事后人工/脚本补录，避免历史彻底丢失。
func (r *Recorder) dumpFailedGame(events []*GameEvent, commitErr error) {
	if err := os.MkdirAll(failedDumpDir, 0o755); err != nil {
		logger.Error("history dump: 创建兜底目录失败",
			logger.F("room_id", r.game.RoomID), logger.F("error", err))
		return
	}
	payload := map[string]any{
		"commitError": commitErr.Error(),
		"dumpedAt":    time.Now().Format("2006-01-02 15:04:05.000"),
		"game":        r.game,
		"players":     r.players,
		"events":      events,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		logger.Error("history dump: 序列化失败",
			logger.F("room_id", r.game.RoomID), logger.F("error", err))
		return
	}
	fname := fmt.Sprintf("game_%s_%d.json", r.game.RoomID, time.Now().UnixNano())
	fpath := filepath.Join(failedDumpDir, fname)
	if err := os.WriteFile(fpath, b, 0o644); err != nil {
		logger.Error("history dump: 写文件失败",
			logger.F("room_id", r.game.RoomID), logger.F("path", fpath), logger.F("error", err))
		return
	}
	logger.Error("history commit 最终失败，已兜底落盘",
		logger.F("room_id", r.game.RoomID),
		logger.F("events", len(events)),
		logger.F("path", fpath))
}

func (r *Recorder) Discard() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.committed {
		return
	}
	r.discarded = true
	r.events = nil
}
