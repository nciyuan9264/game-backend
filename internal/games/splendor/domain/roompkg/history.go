package roompkg

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	spgame "github.com/nciyuan9264/game-backend/internal/games/splendor/domain/game"
	"github.com/nciyuan9264/game-backend/pkg/database"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

// HistoryRepo 全局单例，由 cmd/splendor 在 InitPostgres 后注入。
var HistoryRepo *gamehistory.Repo

const splendorWinScore = 15

// InitHistoryRepo 由 cmd/splendor 在 database.InitPostgres 后调用。
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

// isRecordableCmd 判断 splendor 命令是否需要落库用于回放。
func isRecordableCmd(cmdType string) bool {
	switch cmdType {
	case "game_get_gem", "game_buy_card", "game_preserve_card", "turn_timeout":
		return true
	}
	return false
}

// startRecording 在开局时调用：构造内存录制器，缓存元信息与初始状态；
// 不写库。仅当 finishRecording（终局）时才事务落库。
func (r *RoomService) startRecording() {
	if HistoryRepo == nil {
		return
	}
	if r.Recorder != nil {
		return
	}

	startedAt := r.Room.State.GameStartTime
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	initialJSON, err := gamehistory.MarshalToJSON(r.Room.State)
	if err != nil {
		logger.Error("序列化 initial_state 失败", logger.F("room_id", r.Room.ID), logger.F("error", err))
		return
	}

	g := &gamehistory.Game{
		RoomID:       r.Room.ID,
		GameType:     "splendor",
		StartedAt:    startedAt,
		MaxPlayers:   r.Room.State.MaxPlayers,
		InitialState: initialJSON,
	}

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
	logger.Info("history recording started (in-memory)", logger.F("room_id", r.Room.ID))
}

// recordEvent 把命令写入内存事件缓冲（白名单过滤），并附带该命令处理完毕后的权威状态快照。
func (r *RoomService) recordEvent(cmd domain.Command) {
	if r.Recorder == nil {
		return
	}
	if !isRecordableCmd(cmd.Type) {
		return
	}
	result := spgame.RecomputeDerivedState(r.Room)
	blob, err := spgame.EncodeStateSnapshot(r.Room.State, result)
	if err != nil {
		logger.Error("编码 state_blob 失败", logger.F("room_id", r.Room.ID), logger.F("error", err))
		blob = nil
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
	return splendorMaxScore(r.Room)*2 >= splendorWinScore
}

func splendorMaxScore(r *domain.Room) int {
	if r == nil || r.State == nil {
		return 0
	}
	maxScore := 0
	for _, ps := range r.State.Players {
		if ps != nil && ps.Score > maxScore {
			maxScore = ps.Score
		}
	}
	return maxScore
}

type splendorScoreItem struct {
	playerID string
	score    int
	cards    int
}

func splendorScoreItems(r *domain.Room) []splendorScoreItem {
	if r == nil || r.State == nil {
		return nil
	}
	scores := make([]splendorScoreItem, 0, len(r.Connections))
	for pid, pc := range r.Connections {
		if pc == nil {
			continue
		}
		ps, ok := r.State.Players[pid]
		if !ok || ps == nil {
			continue
		}
		scores = append(scores, splendorScoreItem{
			playerID: pid,
			score:    ps.Score,
			cards:    len(ps.NormalCard),
		})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		if scores[i].cards != scores[j].cards {
			return scores[i].cards < scores[j].cards
		}
		return scores[i].playerID < scores[j].playerID
	})
	return scores
}

func buildSplendorFinalPlayers(r *domain.Room, abandoned bool) ([]gamehistory.GamePlayer, string, *int64) {
	scores := splendorScoreItems(r)
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
	prevScore, prevCards := -1, -1
	abandonedRank := len(scores)
	for i, s := range scores {
		if abandoned {
			rank = abandonedRank
		} else if i == 0 || s.score != prevScore || s.cards != prevCards {
			rank = i + 1
			prevScore = s.score
			prevCards = s.cards
		}
		score := s.score
		finalRank := rank
		playersIn = append(playersIn, gamehistory.GamePlayer{
			PlayerID:   s.playerID,
			FinalScore: &score,
			FinalRank:  &finalRank,
			IsWinner:   !abandoned && s.playerID == winnerPlayerID,
		})
	}
	return playersIn, winnerPlayerID, winnerUserID
}

// finishRecording 终局时调用：算 winner / final_score（荣誉分），并把整局事务性落库。
func (r *RoomService) finishRecording() {
	if r.Recorder == nil {
		return
	}

	result := spgame.RecomputeDerivedState(r.Room)
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

	playersIn, winnerPlayerID, winnerUserID := buildSplendorFinalPlayers(r.Room, false)

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
		logger.Error("history finish failed", logger.F("room_id", r.Room.ID), logger.F("error", err))
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

	result := spgame.RecomputeDerivedState(r.Room)
	maxScore := splendorMaxScore(r.Room)
	finalResult := map[string]any{
		"abandoned": true,
		"reason":    "no_human_after_progress_half",
		"progress": map[string]int{
			"maxScore": maxScore,
			"winScore": splendorWinScore,
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
	playersIn, winnerPlayerID, winnerUserID := buildSplendorFinalPlayers(r.Room, true)

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
		logger.Error("history abandoned finish failed", logger.F("room_id", r.Room.ID), logger.F("error", err))
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
