package game

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

type captureSync struct{ data []byte }

func (c *captureSync) WriteMessage(_ int, data []byte) error {
	c.data = append([]byte(nil), data...)
	return nil
}
func (*captureSync) Close() error { return nil }

func TestRoomSyncExposesOnlyRecipientsAssets(t *testing.T) {
	base := roomcore.NewBase("private-assets", 8)
	base.PlayerSeq = []string{"alice", "bob"}
	base.Connections["alice"] = &domain.PlayerConn{PlayerID: "alice", Name: "Alice", Avatar: "/alice.png"}
	base.Connections["bob"] = &domain.PlayerConn{PlayerID: "bob", Name: "Bob", Avatar: "/bob.png"}
	r := &domain.Room{Base: base, State: &domain.GameState{
		RoomStatus: domain.RoomStatusBuyStock, PhaseVersion: 9, TurnNumber: 3, Visibility: "private",
		EndGame: domain.EndGameState{Eligible: true, Condition: EndConditionCompanySize, Company: "Tower", TileCount: 41, Votes: map[string]bool{"alice": true, "bob": false}},
		Players: map[string]*domain.PlayerState{
			"alice": {Money: 500, Stocks: map[string]int{"Tower": 2}, Tiles: []string{"1A"}},
			"bob":   {Money: 1700, Stocks: map[string]int{"Tower": 7}, Tiles: []string{"9I"}},
		},
	}}
	conn := &captureSync{}
	if err := SyncRoomMessage(conn, r, base.Connections["alice"], nil); err != nil {
		t.Fatal(err)
	}
	var message struct {
		PlayerData   domain.PlayerState `json:"playerData"`
		PhaseVersion uint64             `json:"phaseVersion"`
		RoomData     struct {
			Players map[string]map[string]any `json:"players"`
			EndGame domain.EndGameState       `json:"endGame"`
		} `json:"roomData"`
	}
	if err := json.Unmarshal(conn.data, &message); err != nil {
		t.Fatal(err)
	}
	if message.PlayerData.Money != 500 || message.PhaseVersion != 9 {
		t.Fatal("recipient data/phase missing")
	}
	if !message.RoomData.EndGame.Eligible || message.RoomData.EndGame.Votes["bob"] {
		t.Fatal("end game state missing")
	}
	bob := message.RoomData.Players["bob"]
	if bob["name"] != "Bob" || bob["avatar"] != "/bob.png" {
		t.Fatal("public display profile missing")
	}
	for _, key := range []string{"email", "money", "stocks", "tiles", "playerData"} {
		if _, exists := bob[key]; exists {
			t.Fatalf("private opponent field: %s", key)
		}
	}
}

func TestBroadcastToRoomClearsThinkTimerWhenRoomEnds(t *testing.T) {
	base := roomcore.NewBase("end-room", 8)
	base.TurnDeadline = time.Now().Add(time.Minute)
	base.TurnTimeout = time.Minute
	base.ThinkTimer = time.NewTimer(time.Hour)

	r := &domain.Room{
		Base: base,
		State: &domain.GameState{
			RoomStatus:    domain.RoomStatusEnd,
			BoardTiles:    map[string]*domain.Tile{},
			Players:       map[string]*domain.PlayerState{},
			Companies:     map[string]*domain.CompanyState{},
			CurrentPlayer: "alice",
		},
	}
	companies := []string{"Sackson", "Tower", "American", "Festival", "Worldwide", "Continental", "Imperial"}
	for i := 0; i < 91; i++ {
		company := companies[i%len(companies)]
		r.State.BoardTiles[fmt.Sprintf("tile-%d", i)] = &domain.Tile{
			ID:     fmt.Sprintf("tile-%d", i),
			Belong: company,
		}
	}
	for _, company := range companies {
		r.State.Companies[company] = &domain.CompanyState{Name: company}
	}

	BroadcastToRoom(r)

	if r.State.RoomStatus != domain.RoomStatusEnd {
		t.Fatalf("room status = %q, want end", r.State.RoomStatus)
	}
	if r.Base.ThinkTimer != nil {
		t.Fatalf("think timer was not cleared")
	}
	if !r.Base.TurnDeadline.IsZero() {
		t.Fatalf("turn deadline = %v, want zero", r.Base.TurnDeadline)
	}
	if r.Base.TurnTimeout != 0 {
		t.Fatalf("turn timeout = %v, want zero", r.Base.TurnTimeout)
	}
}
