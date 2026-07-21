package game

import (
	"fmt"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func TestBroadcastToRoomClearsThinkTimerWhenRoomEnds(t *testing.T) {
	base := roomcore.NewBase("end-room", 8)
	base.TurnDeadline = time.Now().Add(time.Minute)
	base.TurnTimeout = time.Minute
	base.ThinkTimer = time.NewTimer(time.Hour)

	r := &domain.Room{
		Base: base,
		State: &domain.GameState{
			RoomStatus:    domain.RoomStatusSetTile,
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
