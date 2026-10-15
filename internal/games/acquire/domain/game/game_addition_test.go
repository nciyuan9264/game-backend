package game

import (
	"testing"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func TestHandleTurnTimeoutMessageIgnoresEndedRoom(t *testing.T) {
	base := roomcore.NewBase("ended-room", 8)
	base.Connections["alice"] = &roomcore.PlayerConn{PlayerID: "alice", Online: true}
	base.Connections["bob"] = &roomcore.PlayerConn{PlayerID: "bob", Online: true}
	r := &domain.Room{
		Base: base,
		State: &domain.GameState{
			CurrentPlayer:    "alice",
			RoomStatus:       domain.RoomStatusEnd,
			MergeMainCompany: "Tower",
			MergeSettleData: map[string]domain.SettleData{
				"Tower": {Hoders: []string{"alice"}},
			},
			MergingSelection: domain.MergingSelection{
				MainCompany: []string{"Tower"},
			},
		},
	}

	HandleTurnTimeoutMessage(r, domain.Command{Type: "turn_timeout", PlayerID: "alice"})

	if r.State.CurrentPlayer != "alice" {
		t.Fatalf("current player = %q, want alice", r.State.CurrentPlayer)
	}
	if r.State.RoomStatus != domain.RoomStatusEnd {
		t.Fatalf("room status = %q, want end", r.State.RoomStatus)
	}
	if r.State.MergeMainCompany != "Tower" {
		t.Fatalf("merge main company was reset")
	}
	if _, ok := r.State.MergeSettleData["Tower"]; !ok {
		t.Fatalf("merge settle data was reset")
	}
	if len(r.State.MergingSelection.MainCompany) != 1 || r.State.MergingSelection.MainCompany[0] != "Tower" {
		t.Fatalf("merging selection was reset: %+v", r.State.MergingSelection)
	}
}

func TestSwitchToNextPlayerSkipsEmptyHands(t *testing.T) {
	base := roomcore.NewBase("skip-empty", 8)
	for _, playerID := range []string{"alice", "bob", "carol"} {
		base.Connections[playerID] = &roomcore.PlayerConn{PlayerID: playerID}
	}
	r := &domain.Room{
		Base: base,
		State: &domain.GameState{
			CurrentPlayer: "alice",
			Players: map[string]*domain.PlayerState{
				"alice": {Tiles: []string{"1A"}},
				"bob":   {Tiles: nil},
				"carol": {Tiles: []string{"2A"}},
			},
		},
	}

	if err := SwitchToNextPlayer(r, "alice"); err != nil {
		t.Fatal(err)
	}
	if r.State.CurrentPlayer != "carol" {
		t.Fatalf("current player = %q, want carol", r.State.CurrentPlayer)
	}

	r.State.Players["alice"].Tiles = nil
	r.State.Players["carol"].Tiles = nil
	if err := SwitchToNextPlayer(r, "carol"); err != nil {
		t.Fatal(err)
	}
	if r.State.CurrentPlayer != "alice" {
		t.Fatalf("all-empty fallback player = %q, want alice", r.State.CurrentPlayer)
	}
}

func TestRestartClearsEndGameState(t *testing.T) {
	base := roomcore.NewBase("restart-end", 8)
	base.Connections["owner"] = &roomcore.PlayerConn{PlayerID: "owner"}
	r := &domain.Room{
		Base: base,
		State: &domain.GameState{
			RoomStatus: domain.RoomStatusEnd,
			BoardTiles: map[string]*domain.Tile{"1A": {ID: "1A", Belong: "Tower"}},
			Players:    map[string]*domain.PlayerState{"owner": {}},
			EndGame: domain.EndGameState{
				Eligible:  true,
				Condition: EndConditionCompanySize,
				Company:   "Tower",
				TileCount: 41,
				Votes:     map[string]bool{"owner": true},
			},
		},
	}

	HandleRestartGameMessage(r, domain.Command{PlayerID: "owner"})

	if r.State.EndGame.Eligible || len(r.State.EndGame.Votes) != 0 {
		t.Fatalf("end game state survived restart: %+v", r.State.EndGame)
	}
}
