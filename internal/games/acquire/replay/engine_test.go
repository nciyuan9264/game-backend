package replay

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	acgame "github.com/nciyuan9264/game-backend/internal/games/acquire/domain/game"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
	"gorm.io/datatypes"
)

func TestApplyEventReplaysEndVote(t *testing.T) {
	base := roomcore.NewBase("replay-end-vote", 0)
	base.Connections["human"] = &roomcore.PlayerConn{PlayerID: "human"}
	r := &domain.Room{
		Base: base,
		State: &domain.GameState{
			RoomStatus: domain.RoomStatusSetTile,
			EndGame: domain.EndGameState{
				Eligible: true,
				Votes:    map[string]bool{"human": false},
			},
		},
	}

	applyEvent(r, gamehistory.GameEvent{
		CmdType:  "game_end_vote",
		PlayerID: "human",
		Payload:  datatypes.JSON([]byte(`{"allow":true}`)),
	})

	if !r.State.EndGame.Votes["human"] {
		t.Fatal("end vote event was not applied")
	}
}

func TestReplaySnapshotIncludesEndGameState(t *testing.T) {
	base := roomcore.NewBase("replay-end-state", 0)
	state := &domain.GameState{
		EndGame: domain.EndGameState{
			Eligible:  true,
			Condition: "company_size",
			Company:   "Tower",
			TileCount: 41,
			Votes:     map[string]bool{"human": true},
		},
		Players: map[string]*domain.PlayerState{},
	}

	snapshot := buildSnapshotFromState(state, nil, base, 0, -1, nil)
	endGame, ok := snapshot.RoomData["endGame"].(map[string]interface{})
	if !ok || endGame["eligible"] != true || endGame["company"] != "Tower" {
		t.Fatalf("end game state missing from replay snapshot: %#v", snapshot.RoomData["endGame"])
	}
}

func TestReplayTurnBoundaryHandlesSamePlayerAfterSkippedSeats(t *testing.T) {
	state := &domain.GameState{RoomStatus: domain.RoomStatusSetTile, CurrentPlayer: "human"}
	if !replayTurnCompleted("game_buy_stock", domain.RoomStatusBuyStock, "human", state) {
		t.Fatal("buy completion was not recognized")
	}
	if replayTurnCompleted("game_end_vote", domain.RoomStatusSetTile, "human", state) {
		t.Fatal("vote was treated as a turn boundary")
	}
}

func TestReplayPageAuthoritativeSnapshotOffsets(t *testing.T) {
	game, players, events := authoritativeReplayFixture(t, 4)

	first, err := ReplayPage(game, players, len(events), 0, 3, events[:2], nil)
	if err != nil {
		t.Fatalf("ReplayPage first page failed: %v", err)
	}
	assertSnapshotSeqs(t, first, []int{-1, 0, 1})
	for _, snapshot := range first {
		if snapshot.TotalEvents != 4 {
			t.Fatalf("unexpected totalEvents: got %d want 4", snapshot.TotalEvents)
		}
	}
	if first[0].CurrentEvent != nil {
		t.Fatalf("initial snapshot has current event: %#v", first[0].CurrentEvent)
	}
	if first[2].CurrentEvent == nil || first[2].CurrentEvent.Seq != 1 {
		t.Fatalf("wrong current event: %#v", first[2].CurrentEvent)
	}

	last, err := ReplayPage(game, players, len(events), 3, 20, events[2:], nil)
	if err != nil {
		t.Fatalf("ReplayPage last page failed: %v", err)
	}
	assertSnapshotSeqs(t, last, []int{2, 3})
}

func TestReplayPageLegacyMatchesReplayAll(t *testing.T) {
	state := replayTestState()
	initialState, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	game := &gamehistory.Game{RoomID: "legacy-page", InitialState: initialState}
	players := []gamehistory.GamePlayer{{PlayerID: "human"}}
	events := make([]gamehistory.GameEvent, 5)
	for i := range events {
		events[i] = gamehistory.GameEvent{
			Seq:      i,
			PlayerID: "human",
			CmdType:  "turn_timeout",
			Payload:  datatypes.JSON([]byte(`null`)),
		}
	}

	all, err := ReplayAll(game, players, events)
	if err != nil {
		t.Fatalf("ReplayAll failed: %v", err)
	}
	page, err := ReplayPage(game, players, len(events), 2, 3, events[1:4], events)
	if err != nil {
		t.Fatalf("ReplayPage failed: %v", err)
	}
	if !reflect.DeepEqual(page, all[2:5]) {
		pageJSON, _ := json.Marshal(page)
		allJSON, _ := json.Marshal(all[2:5])
		t.Fatalf("legacy page differs from ReplayAll:\npage=%s\nall=%s", pageJSON, allJSON)
	}
}

func TestReplayPageOutOfRangeIsEmpty(t *testing.T) {
	game, players, events := authoritativeReplayFixture(t, 2)
	page, err := ReplayPage(game, players, len(events), 3, 20, nil, nil)
	if err != nil {
		t.Fatalf("ReplayPage failed: %v", err)
	}
	if len(page) != 0 {
		t.Fatalf("out-of-range page returned %d snapshots", len(page))
	}
}

func authoritativeReplayFixture(t *testing.T, count int) (*gamehistory.Game, []gamehistory.GamePlayer, []gamehistory.GameEvent) {
	t.Helper()
	initial := replayTestState()
	initialState, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]gamehistory.GameEvent, count)
	for i := range events {
		state := replayTestState()
		state.LastTileKey = string(rune('A' + i))
		blob, encodeErr := acgame.EncodeStateSnapshot(state, map[string]interface{}{"step": i})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		events[i] = gamehistory.GameEvent{
			Seq:       i,
			PlayerID:  "human",
			CmdType:   "turn_timeout",
			Payload:   datatypes.JSON([]byte(`null`)),
			StateBlob: blob,
		}
	}
	return &gamehistory.Game{RoomID: "authoritative-page", InitialState: initialState},
		[]gamehistory.GamePlayer{{PlayerID: "human"}},
		events
}

func replayTestState() *domain.GameState {
	return &domain.GameState{
		CurrentPlayer: "human",
		RoomStatus:    domain.RoomStatusSetTile,
		BoardTiles:    map[string]*domain.Tile{},
		Players: map[string]*domain.PlayerState{
			"human": {
				Money:  6000,
				Stocks: map[string]int{},
				Tiles:  []string{},
			},
		},
		Companies: map[string]*domain.CompanyState{},
	}
}

func assertSnapshotSeqs(t *testing.T, snapshots []*Snapshot, want []int) {
	t.Helper()
	got := make([]int, len(snapshots))
	for i := range snapshots {
		got[i] = snapshots[i].Seq
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot seqs = %v, want %v", got, want)
	}
}
