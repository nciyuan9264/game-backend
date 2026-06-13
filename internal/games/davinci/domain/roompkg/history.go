package roompkg

import (
	"context"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/database"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

var HistoryRepo *gamehistory.Repo

const davinciMaxBoardCards = 26

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

func (r *RoomService) resetHistoryState() {
	r.HistoryStartedAt = time.Time{}
	r.HistoryEnded = false
}

func (r *RoomService) shouldRecordAbandoned() bool {
	if r == nil || r.Room == nil || r.Room.State == nil {
		return false
	}
	if r.Room.State.GameStartTime.IsZero() {
		return false
	}
	remaining := len(r.Room.State.BoardCards)
	drawnCards := davinciMaxBoardCards - remaining
	return drawnCards*2 >= davinciMaxBoardCards
}

func buildDavinciHistoryPlayers(r *domain.Room, winnerPlayerID string) []gamehistory.GamePlayer {
	if r == nil {
		return nil
	}
	playerIDs := make([]string, 0, len(r.Connections))
	for pid, pc := range r.Connections {
		if pc != nil {
			playerIDs = append(playerIDs, pid)
		}
	}
	sort.Strings(playerIDs)

	players := make([]gamehistory.GamePlayer, 0, len(playerIDs))
	for i, pid := range playerIDs {
		pc := r.Connections[pid]
		gp := gamehistory.GamePlayer{
			PlayerID:  pid,
			SeatIndex: i,
			IsAI:      pc.AI,
			IsWinner:  winnerPlayerID != "" && pid == winnerPlayerID,
		}
		if !pc.AI {
			if uid, ok := gamehistory.ParseUserIDFromPlayerID(pid); ok {
				gp.UserID = &uid
			}
		}
		players = append(players, gp)
	}
	return players
}

func (r *RoomService) finishHistory() {
	if HistoryRepo == nil {
		r.HistoryEnded = true
		return
	}

	endedAt := time.Now()
	startedAt := r.Room.State.GameStartTime
	if startedAt.IsZero() {
		startedAt = endedAt
	}
	duration := int(endedAt.Sub(startedAt).Seconds())

	winnerPlayerID := findDavinciWinner(r.Room)
	var winnerUserID *int64
	if winnerPlayerID != "" {
		if pc := r.Room.Connections[winnerPlayerID]; pc != nil && !pc.AI {
			if uid, ok := gamehistory.ParseUserIDFromPlayerID(winnerPlayerID); ok {
				winnerUserID = &uid
			}
		}
	} else {
		logger.Error("达芬奇历史记录未找到赢家", logger.F("room_id", r.Room.ID))
	}

	g := &gamehistory.Game{
		RoomID:          r.Room.ID,
		GameType:        "davinci",
		StartedAt:       startedAt,
		EndedAt:         &endedAt,
		DurationSeconds: duration,
		WinnerUserID:    winnerUserID,
		WinnerPlayerID:  winnerPlayerID,
		EndReason:       gamehistory.EndReasonCompleted,
		MaxPlayers:      r.Room.State.MaxPlayers,
	}

	players := buildDavinciHistoryPlayers(r.Room, winnerPlayerID)

	gameID, err := HistoryRepo.SaveCompletedGame(context.Background(), g, players, nil)
	if err != nil {
		logger.Error("davinci history finish failed",
			logger.F("room_id", r.Room.ID),
			logger.F("error", err))
		return
	}

	r.HistoryEnded = true
	logger.Info("davinci history recorded",
		logger.F("game_id", gameID),
		logger.F("room_id", r.Room.ID),
		logger.F("winner", winnerPlayerID),
		logger.F("duration_s", duration))
}

func (r *RoomService) finishAbandonedHistory(now time.Time) {
	if r.HistoryEnded {
		return
	}
	if HistoryRepo == nil || !r.shouldRecordAbandoned() {
		return
	}
	startedAt := r.Room.State.GameStartTime
	g := &gamehistory.Game{
		RoomID:          r.Room.ID,
		GameType:        "davinci",
		StartedAt:       startedAt,
		EndedAt:         &now,
		DurationSeconds: int(now.Sub(startedAt).Seconds()),
		WinnerUserID:    nil,
		WinnerPlayerID:  "",
		EndReason:       gamehistory.EndReasonAbandoned,
		MaxPlayers:      r.Room.State.MaxPlayers,
	}
	players := buildDavinciHistoryPlayers(r.Room, "")

	gameID, err := HistoryRepo.SaveCompletedGame(context.Background(), g, players, nil)
	if err != nil {
		logger.Error("davinci abandoned history finish failed",
			logger.F("room_id", r.Room.ID),
			logger.F("error", err))
		return
	}

	r.HistoryEnded = true
	logger.Info("davinci abandoned history recorded",
		logger.F("game_id", gameID),
		logger.F("room_id", r.Room.ID),
		logger.F("duration_s", g.DurationSeconds))
}

func findDavinciWinner(r *domain.Room) string {
	winners := make([]string, 0, 1)
	for pid, ps := range r.State.Players {
		if ps == nil {
			continue
		}
		allRevealed := true
		for _, c := range ps.Cards {
			if c != nil && !c.IsRevealed {
				allRevealed = false
				break
			}
		}
		if !allRevealed {
			winners = append(winners, pid)
		}
	}
	sort.Strings(winners)
	if len(winners) == 0 {
		return ""
	}
	return winners[0]
}
