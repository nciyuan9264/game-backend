package gamehistory

import (
	"context"
	"sync"
	"time"

	"github.com/nciyuan9264/game-backend/pkg/logger"
	"gorm.io/datatypes"
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
	FinalResult     []byte
	Players         []GamePlayer
}

func (r *Recorder) Commit(ctx context.Context, in CommitInput) (int64, error) {
	r.mu.Lock()
	if r.committed || r.discarded {
		r.mu.Unlock()
		return 0, nil
	}
	r.committed = true
	events := r.events
	r.events = nil
	r.mu.Unlock()

	r.game.EndedAt = &in.EndedAt
	r.game.DurationSeconds = in.DurationSeconds
	r.game.WinnerUserID = in.WinnerUserID
	r.game.WinnerPlayerID = in.WinnerPlayerID
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

	gameID, err := r.repo.SaveCompletedGame(ctx, r.game, r.players, events)
	if err != nil {
		logger.Error("history commit failed",
			logger.F("room_id", r.game.RoomID),
			logger.F("error", err))
		return 0, err
	}
	logger.Info("history committed",
		logger.F("game_id", gameID),
		logger.F("room_id", r.game.RoomID),
		logger.F("events", len(events)))
	return gameID, nil
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
