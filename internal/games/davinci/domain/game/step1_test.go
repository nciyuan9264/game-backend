package game

import (
	"encoding/json"
	"testing"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

type stubConn struct{}

func (stubConn) WriteMessage(int, []byte) error    { return nil }
func (stubConn) Close() error                      { return nil }
func (stubConn) ReadMessage() (int, []byte, error) { return 0, nil, nil }

func newGuessRoom(targetRevealed bool) *domain.Room {
	base := roomcore.NewBase("test", 8)
	r := &domain.Room{
		Base: base,
		State: &domain.GameState{
			RoomStatus:    domain.RoomStatusGuessCard,
			CurrentPlayer: "me",
			BoardCards:    map[string]*domain.Card{},
			Players: map[string]*domain.PlayerState{
				"me": {Cards: []*domain.Card{
					{ID: "M1", Num: domain.Num0, Color: domain.ColorWhite, IsRevealed: true, Index: 0},
				}},
				"op": {Cards: []*domain.Card{
					{ID: "A1", Num: domain.Num1, Color: domain.ColorWhite, IsRevealed: targetRevealed, Index: 0},
					{ID: "A2", Num: domain.Num3, Color: domain.ColorWhite, IsRevealed: false, Index: 1},
				}},
			},
		},
	}
	r.Connections["me"] = &domain.PlayerConn{PlayerID: "me", Online: false, Conn: stubConn{}}
	r.Connections["op"] = &domain.PlayerConn{PlayerID: "op", Online: false, Conn: stubConn{}}
	return r
}

func TestHandleGuessCardCorrectEntersSetCard(t *testing.T) {
	r := newGuessRoom(false)
	payload, _ := json.Marshal(map[string]interface{}{
		"id":  "A1",
		"num": domain.Num1,
	})
	cmd := domain.Command{Type: "game_guess_card", PlayerID: "me", Payload: payload}
	if err := HandleGuessCardMessage(r, cmd); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.State.RoomStatus != domain.RoomStatusSetCard {
		t.Fatalf("expected setCard after correct guess, got %s", r.State.RoomStatus)
	}
	if r.State.CurrentPlayer != "me" {
		t.Fatalf("expected current player unchanged after correct guess, got %s", r.State.CurrentPlayer)
	}
	if !r.State.Players["op"].Cards[0].IsRevealed {
		t.Fatalf("expected target revealed")
	}
	if r.State.LastData == nil {
		t.Fatalf("expected LastData set")
	}
	var got map[string]interface{}
	if err := json.Unmarshal(r.State.LastData.Payload, &got); err != nil {
		t.Fatalf("decode last data: %v", err)
	}
	if !got["correct"].(bool) {
		t.Fatalf("expected correct=true in payload")
	}
}

func TestHandleGuessCardCorrectAllRevealedEndsGame(t *testing.T) {
	r := newGuessRoom(true)
	r.State.Players["op"].Cards[0].IsRevealed = true
	payload, _ := json.Marshal(map[string]interface{}{
		"id":  "A2",
		"num": domain.Num3,
	})
	cmd := domain.Command{Type: "game_guess_card", PlayerID: "me", Payload: payload}
	if err := HandleGuessCardMessage(r, cmd); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.State.RoomStatus != domain.RoomStatusEnd {
		t.Fatalf("expected end, got %s", r.State.RoomStatus)
	}
}
