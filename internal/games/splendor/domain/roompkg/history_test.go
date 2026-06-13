package roompkg

import (
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func newSplendorHistoryTestRoom(maxScore int) *RoomService {
	base := roomcore.NewBase("splendor-history-test", 1)
	base.Connections["alice-101"] = &roomcore.PlayerConn{PlayerID: "alice-101"}
	base.Connections["bot-1"] = &roomcore.PlayerConn{PlayerID: "bot-1", AI: true}

	return &RoomService{Room: &domain.Room{
		Base: base,
		State: &domain.GameState{
			GameStartTime: time.Now(),
			Players: map[string]*domain.PlayerState{
				"alice-101": {Score: maxScore},
				"bot-1":     {Score: 3},
			},
		},
	}}
}

func TestSplendorShouldRecordAbandonedByMaxScore(t *testing.T) {
	if newSplendorHistoryTestRoom(7).shouldRecordAbandoned() {
		t.Fatal("max score 7 should not record abandoned game")
	}
	if !newSplendorHistoryTestRoom(8).shouldRecordAbandoned() {
		t.Fatal("max score 8 should record abandoned game")
	}

	rs := newSplendorHistoryTestRoom(8)
	rs.Room.State.GameStartTime = time.Time{}
	if rs.shouldRecordAbandoned() {
		t.Fatal("zero GameStartTime should not record abandoned game")
	}
}

func TestBuildSplendorFinalPlayersMarksAbandonedPlayersAsLosses(t *testing.T) {
	players, winnerPlayerID, winnerUserID := buildSplendorFinalPlayers(newSplendorHistoryTestRoom(8).Room, true)

	if winnerPlayerID != "" {
		t.Fatalf("winnerPlayerID = %q, want empty for abandoned game", winnerPlayerID)
	}
	if winnerUserID != nil {
		t.Fatalf("winnerUserID = %v, want nil for abandoned game", *winnerUserID)
	}
	if len(players) != 2 {
		t.Fatalf("len(players) = %d, want 2", len(players))
	}
	for _, p := range players {
		if p.IsWinner {
			t.Fatalf("player %s is winner in abandoned game", p.PlayerID)
		}
		if p.FinalRank == nil || *p.FinalRank != 2 {
			t.Fatalf("player %s final rank = %v, want 2", p.PlayerID, p.FinalRank)
		}
		if p.PlayerID == "alice-101" && (p.FinalScore == nil || *p.FinalScore != 8) {
			t.Fatalf("alice final score = %v, want 8", p.FinalScore)
		}
	}
}
