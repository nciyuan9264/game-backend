package roompkg

import (
	"fmt"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func newAcquireHistoryTestRoom(placedTiles int) *RoomService {
	base := roomcore.NewBase("acquire-history-test", 1)
	boardTiles := make(map[string]*domain.Tile, acquireMaxTiles)
	for i := 0; i < acquireMaxTiles; i++ {
		belong := ""
		if i < placedTiles {
			belong = "Sackson"
		}
		id := fmt.Sprintf("T%d", i)
		boardTiles[id] = &domain.Tile{ID: id, Belong: belong}
	}

	base.Connections["alice-101"] = &roomcore.PlayerConn{PlayerID: "alice-101"}
	base.Connections["bot-1"] = &roomcore.PlayerConn{PlayerID: "bot-1", AI: true}

	return &RoomService{Room: &domain.Room{
		Base: base,
		State: &domain.GameState{
			GameStartTime: time.Now(),
			BoardTiles:    boardTiles,
			Companies: map[string]*domain.CompanyState{
				"Sackson": {Name: "Sackson", StockPrice: 100},
			},
			Players: map[string]*domain.PlayerState{
				"alice-101": {Money: 1000, Stocks: map[string]int{"Sackson": 1}},
				"bot-1":     {Money: 500, Stocks: map[string]int{}},
			},
		},
	}}
}

func TestAcquireShouldRecordAbandonedByPlacedTiles(t *testing.T) {
	if newAcquireHistoryTestRoom(53).shouldRecordAbandoned() {
		t.Fatal("53 placed tiles should not record abandoned game")
	}
	if !newAcquireHistoryTestRoom(54).shouldRecordAbandoned() {
		t.Fatal("54 placed tiles should record abandoned game")
	}

	rs := newAcquireHistoryTestRoom(54)
	rs.Room.State.GameStartTime = time.Time{}
	if rs.shouldRecordAbandoned() {
		t.Fatal("zero GameStartTime should not record abandoned game")
	}
}

func TestBuildAcquireFinalPlayersMarksAbandonedPlayersAsLosses(t *testing.T) {
	players, winnerPlayerID, winnerUserID := buildAcquireFinalPlayers(newAcquireHistoryTestRoom(54).Room, true)

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
		if p.PlayerID == "alice-101" && (p.FinalScore == nil || *p.FinalScore != 1100) {
			t.Fatalf("alice final score = %v, want 1100", p.FinalScore)
		}
	}
}
