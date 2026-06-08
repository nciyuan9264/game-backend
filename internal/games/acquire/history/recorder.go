package history

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nciyuan9264/game-backend/pkg/logger"
	"gorm.io/datatypes"
)

// IsRecordableCmd 命令白名单：只有这些命令会进入 game_events，影响 GameState 的命令。
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

// ParseUserIDFromPlayerID 从 "name-id" 形态的 playerID 解析尾部数字 user_id。
// AI 玩家（"ai-1"）虽然也能解析出 1，因此调用方需结合 isAI 标志使用。
func ParseUserIDFromPlayerID(playerID string) (int64, bool) {
	idx := strings.LastIndex(playerID, "-")
	if idx < 0 || idx == len(playerID)-1 {
		return 0, false
	}
	v, err := strconv.ParseInt(playerID[idx+1:], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// Recorder 单局录制器。开局时构造，所有事件先在内存里缓冲，
// 只有调用 Commit（即终局）时才一次性事务写库；
// 调用 Discard 或 Close（未 Commit 即关闭）时直接丢弃，未结束的对局不入 DB。
type Recorder struct {
	repo *Repo

	// 开局时确定的元数据。
	game    *Game
	players []GamePlayer

	mu        sync.Mutex
	events    []*GameEvent
	committed bool
	discarded bool
}

// NewRecorder 创建录制器；调用方仍然需要在终局时调用 Commit / 在未结束时调用 Discard。
// game 与 players 描述开局快照；events 后续通过 OnEvent 追加。
func NewRecorder(repo *Repo, game *Game, players []GamePlayer) *Recorder {
	return &Recorder{
		repo:    repo,
		game:    game,
		players: players,
		events:  make([]*GameEvent, 0, 128),
	}
}

// OnEvent 把一条事件加入内存缓冲；不立即写库。
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

// CommitInput 终局时附带的字段。
type CommitInput struct {
	EndedAt         time.Time
	DurationSeconds int
	WinnerUserID    *int64
	WinnerPlayerID  string
	FinalResult     []byte
	Players         []GamePlayer // 终局快照（含 FinalScore / IsWinner 等）
}

// Commit 终局时同步落库：在一个事务里写 games + game_players + game_events。
// 返回 game_id。
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

	// 把 CommitInput 的终局信息合并到 game 与 players 上。
	r.game.EndedAt = &in.EndedAt
	r.game.DurationSeconds = in.DurationSeconds
	r.game.WinnerUserID = in.WinnerUserID
	r.game.WinnerPlayerID = in.WinnerPlayerID
	if len(in.FinalResult) > 0 {
		r.game.FinalResult = datatypes.JSON(in.FinalResult)
	}

	// 用入参 players 的 final 信息覆盖 r.players 中对应行（按 PlayerID 匹配）。
	finalByPID := make(map[string]GamePlayer, len(in.Players))
	for _, p := range in.Players {
		finalByPID[p.PlayerID] = p
	}
	for i := range r.players {
		if fp, ok := finalByPID[r.players[i].PlayerID]; ok {
			r.players[i].FinalScore = fp.FinalScore
			r.players[i].FinalMoney = fp.FinalMoney
			r.players[i].FinalStocks = fp.FinalStocks
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

// Discard 丢弃所有内存数据，不写库。
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

// MarshalToJSON helper：把任意结构序列化为 datatypes.JSON。
func MarshalToJSON(v interface{}) (datatypes.JSON, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(b), nil
}
