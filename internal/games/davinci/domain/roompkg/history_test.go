package roompkg

import (
	"fmt"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func newDavinciHistoryTestRoom(remainingCards int) *RoomService {
	base := roomcore.NewBase("davinci-history-test", 1)
	base.Connections["alice-101"] = &roomcore.PlayerConn{PlayerID: "alice-101"}
	base.Connections["bot-1"] = &roomcore.PlayerConn{PlayerID: "bot-1", AI: true}

	boardCards := make(map[string]*domain.Card, remainingCards)
	for i := 0; i < remainingCards; i++ {
		id := fmt.Sprintf("C%d", i)
		boardCards[id] = &domain.Card{ID: id, Num: domain.Num1, Color: domain.ColorWhite}
	}

	return &RoomService{Room: &domain.Room{
		Base: base,
		State: &domain.GameState{
			GameStartTime: time.Now(),
			BoardCards:    boardCards,
		},
	}}
}

func TestDavinciShouldRecordAbandonedByDrawnBoardCards(t *testing.T) {
	if newDavinciHistoryTestRoom(14).shouldRecordAbandoned() {
		t.Fatal("14 remaining cards should not record abandoned game")
	}
	if !newDavinciHistoryTestRoom(13).shouldRecordAbandoned() {
		t.Fatal("13 remaining cards should record abandoned game")
	}

	rs := newDavinciHistoryTestRoom(13)
	rs.Room.State.GameStartTime = time.Time{}
	if rs.shouldRecordAbandoned() {
		t.Fatal("zero GameStartTime should not record abandoned game")
	}
}

func TestBuildDavinciHistoryPlayersMarksAbandonedPlayersAsLosses(t *testing.T) {
	players := buildDavinciHistoryPlayers(newDavinciHistoryTestRoom(13).Room, "")

	if len(players) != 2 {
		t.Fatalf("len(players) = %d, want 2", len(players))
	}
	for _, p := range players {
		if p.IsWinner {
			t.Fatalf("player %s is winner in abandoned game", p.PlayerID)
		}
		if p.PlayerID == "alice-101" {
			if p.UserID == nil || *p.UserID != 101 {
				t.Fatalf("alice user id = %v, want 101", p.UserID)
			}
		}
	}
}
