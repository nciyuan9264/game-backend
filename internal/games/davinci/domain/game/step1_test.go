package game

import (
	"encoding/json"
	"sort"
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

// TestComputeInsertIndexJokerNotLeftmost 复现用户场景：王牌不在最左时，
// 插入更小的数字牌不应被王牌占位顶偏。
func TestComputeInsertIndexJokerNotLeftmost(t *testing.T) {
	// 现有手牌：num1@idx0, num3@idx1, num5@idx2, joker@idx3
	hand := []*domain.Card{
		{ID: "c1", Num: domain.Num1, Color: domain.ColorBlack, Index: 0},
		{ID: "c3", Num: domain.Num3, Color: domain.ColorBlack, Index: 1},
		{ID: "c5", Num: domain.Num5, Color: domain.ColorBlack, Index: 2},
		{ID: "jk", Num: domain.NumMinus1, Color: domain.ColorBlack, Index: 3},
	}
	newCard := &domain.Card{ID: "c0", Num: domain.Num0, Color: domain.ColorBlack, Index: -1}
	hand = append(hand, newCard)

	pos := ComputeInsertIndex(hand, newCard)
	if pos != 0 {
		t.Fatalf("expected insert index 0 for num0, got %d", pos)
	}

	// 应用位移并断言：数字牌按 Index 升序后严格递增，王牌仅在 >= pos 时 +1。
	for _, c := range hand {
		if c.ID == newCard.ID {
			continue
		}
		if c.Index >= pos {
			c.Index++
		}
	}
	newCard.Index = pos

	if c0 := findCard(hand, "c0"); c0.Index != 0 {
		t.Fatalf("expected num0 at index 0, got %d", c0.Index)
	}
	if c1 := findCard(hand, "c1"); c1.Index != 1 {
		t.Fatalf("expected num1 at index 1, got %d", c1.Index)
	}
	if jk := findCard(hand, "jk"); jk.Index != 4 {
		t.Fatalf("expected joker at index 4, got %d", jk.Index)
	}

	assertNumbersAscendingByIndex(t, hand)
}

func findCard(cards []*domain.Card, id string) *domain.Card {
	for _, c := range cards {
		if c != nil && c.ID == id {
			return c
		}
	}
	return nil
}

// assertNumbersAscendingByIndex 校验：按 Index 升序排列后，
// 非王牌的 num 序列单调不降（同 num 黑在白前）。
func assertNumbersAscendingByIndex(t *testing.T, cards []*domain.Card) {
	t.Helper()
	ordered := make([]*domain.Card, len(cards))
	copy(ordered, cards)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Index < ordered[j].Index })
	var prev *domain.Card
	for _, c := range ordered {
		if c.Num == domain.NumMinus1 {
			continue
		}
		if prev != nil && cardOrderLess(c.Num, c.Color, prev.Num, prev.Color) {
			t.Fatalf("numbers not ascending by index: %d(after) < %d(before)", c.Num, prev.Num)
		}
		prev = c
	}
}
