package game

import (
	"fmt"
	"testing"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func newEndGameRoom() *domain.Room {
	base := roomcore.NewBase("end-game", 8)
	base.Connections["human-1"] = &roomcore.PlayerConn{PlayerID: "human-1", Online: true}
	base.Connections["human-2"] = &roomcore.PlayerConn{PlayerID: "human-2", Online: false}
	base.Connections["ai-1"] = &roomcore.PlayerConn{PlayerID: "ai-1", Online: true, AI: true}
	return &domain.Room{
		Base: base,
		State: &domain.GameState{
			RoomStatus: domain.RoomStatusSetTile,
			BoardTiles: make(map[string]*domain.Tile),
			EndGame:    domain.EndGameState{Votes: make(map[string]bool)},
		},
	}
}

func TestRefreshEndGameEligibilityRequiresEstablishedCompanyAndLatches(t *testing.T) {
	r := newEndGameRoom()
	RefreshEndGameEligibility(r, map[string]int{})
	if r.State.EndGame.Eligible {
		t.Fatal("empty company set became eligible")
	}

	RefreshEndGameEligibility(r, map[string]int{"Tower": 11, "Sackson": 12})
	if !r.State.EndGame.Eligible || r.State.EndGame.Condition != EndConditionAllCompaniesSafe {
		t.Fatalf("safe companies did not unlock end game: %+v", r.State.EndGame)
	}
	if r.State.EndGame.Votes["human-1"] || r.State.EndGame.Votes["human-2"] {
		t.Fatal("human votes must default to false")
	}
	if !r.State.EndGame.Votes["ai-1"] {
		t.Fatal("AI vote must default to true")
	}

	RefreshEndGameEligibility(r, map[string]int{"Tower": 11, "Sackson": 12, "American": 2})
	if !r.State.EndGame.Eligible || r.State.EndGame.Condition != EndConditionAllCompaniesSafe {
		t.Fatal("latched eligibility regressed after unsafe company appeared")
	}
}

func TestRefreshEndGameEligibilityChoosesStableLargestCompany(t *testing.T) {
	r := newEndGameRoom()
	RefreshEndGameEligibility(r, map[string]int{"Tower": 41, "American": 42, "Sackson": 42})
	if r.State.EndGame.Condition != EndConditionCompanySize ||
		r.State.EndGame.Company != "American" || r.State.EndGame.TileCount != 42 {
		t.Fatalf("unexpected company trigger: %+v", r.State.EndGame)
	}
}

func TestEndVotesIncludeOfflineHumansAndAreReversible(t *testing.T) {
	r := newEndGameRoom()
	RefreshEndGameEligibility(r, map[string]int{"Tower": 41})

	HandleEndVoteMessage(r, domain.Command{PlayerID: "human-1", Payload: []byte(`{"allow":true}`)})
	if AllEndVotesApproved(r) {
		t.Fatal("offline human default false was ignored")
	}
	HandleEndVoteMessage(r, domain.Command{PlayerID: "human-2", Payload: []byte(`{"allow":true}`)})
	if !AllEndVotesApproved(r) {
		t.Fatal("all human approvals were not recognized")
	}
	if ShouldFinalizeEnd(r, false) {
		t.Fatal("consensus ended the game before the turn boundary")
	}
	if !ShouldFinalizeEnd(r, true) {
		t.Fatal("consensus did not end the game at the turn boundary")
	}

	HandleEndVoteMessage(r, domain.Command{PlayerID: "human-1", Payload: []byte(`{"allow":false}`)})
	if AllEndVotesApproved(r) {
		t.Fatal("revoked vote remained approved")
	}
}

func TestBoardFullForcesEndOnlyAtTurnBoundary(t *testing.T) {
	r := newEndGameRoom()
	for index := 0; index < acquireBoardTileCount; index++ {
		id := fmt.Sprintf("tile-%d", index)
		r.State.BoardTiles[id] = &domain.Tile{ID: id, Belong: "Blank"}
	}
	if !IsBoardFull(r) {
		t.Fatal("108 placed tiles were not recognized as a full board")
	}
	if ShouldFinalizeEnd(r, false) {
		t.Fatal("full board ended before the current turn completed")
	}
	if !ShouldFinalizeEnd(r, true) {
		t.Fatal("full board did not force end at the turn boundary")
	}
	r.State.BoardTiles["tile-107"].Belong = ""
	if IsBoardFull(r) {
		t.Fatal("107 placed tiles were treated as a full board")
	}
}

func TestParseEndVotePayloadRequiresExplicitBoolean(t *testing.T) {
	if allow, ok := ParseEndVotePayload([]byte(`{"allow":false}`)); !ok || allow {
		t.Fatal("explicit false vote was not parsed")
	}
	for _, payload := range [][]byte{[]byte(`{}`), []byte(`{"allow":"yes"}`), []byte(`null`)} {
		if _, ok := ParseEndVotePayload(payload); ok {
			t.Fatalf("invalid payload accepted: %s", payload)
		}
	}
}
