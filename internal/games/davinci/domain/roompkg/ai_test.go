package roompkg

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func newTestRoom() *domain.Room {
	base := roomcore.NewBase("test", 8)
	return &domain.Room{
		Base: base,
		State: &domain.GameState{
			RoomStatus: domain.RoomStatusGuessCard,
			BoardCards: map[string]*domain.Card{},
			Players:    map[string]*domain.PlayerState{},
		},
	}
}

func revealedCard(id string, num domain.CardNumber, color domain.Color, idx int) *domain.Card {
	return &domain.Card{ID: id, Num: num, Color: color, IsRevealed: true, Index: idx}
}

func hiddenCard(id string, num domain.CardNumber, color domain.Color, idx int) *domain.Card {
	return &domain.Card{ID: id, Num: num, Color: color, IsRevealed: false, Index: idx}
}

func TestBestGuessOptionPrefersHighProb(t *testing.T) {
	r := newTestRoom()
	r.State.Players["me"] = &domain.PlayerState{Cards: []*domain.Card{
		revealedCard("M1", domain.Num0, domain.ColorWhite, 0),
		revealedCard("M2", domain.Num2, domain.ColorWhite, 1),
	}}
	// 对手 op1 黑色三张：黑1(揭) - 黑?(暗) - 黑5(揭)，中间暗牌只能是 {2,3,4}，每个概率 1/3。
	r.State.Players["op1"] = &domain.PlayerState{Cards: []*domain.Card{
		revealedCard("A1", domain.Num1, domain.ColorBlack, 0),
		hiddenCard("A2", domain.Num3, domain.ColorBlack, 1),
		revealedCard("A3", domain.Num5, domain.ColorBlack, 2),
	}}
	// 对手 op2 单张白暗牌，可能数字很多，概率较低。
	r.State.Players["op2"] = &domain.PlayerState{Cards: []*domain.Card{
		hiddenCard("B1", domain.Num7, domain.ColorWhite, 0),
	}}

	opt, ok := bestGuessOption(r, "me")
	if !ok {
		t.Fatalf("expected ok")
	}
	if opt.cardID != "A2" {
		t.Fatalf("expected cardID=A2, got %s", opt.cardID)
	}
	if opt.ownerID != "op1" {
		t.Fatalf("expected ownerID=op1, got %s", opt.ownerID)
	}
	if opt.hitProb < 0.32 || opt.hitProb > 0.35 {
		t.Fatalf("expected hitProb~1/3, got %f", opt.hitProb)
	}
	if opt.num < domain.Num2 || opt.num > domain.Num4 {
		t.Fatalf("expected num in {2,3,4}, got %d", opt.num)
	}
}

func TestBestGuessOptionUsesKnownNumbers(t *testing.T) {
	r := newTestRoom()
	// me 占用白0/白2/白3/白4 → 对手白暗牌可能数字 = {-1,1,5,6,7,8,9,10,11}
	r.State.Players["me"] = &domain.PlayerState{Cards: []*domain.Card{
		revealedCard("M1", domain.Num0, domain.ColorWhite, 0),
		revealedCard("M2", domain.Num2, domain.ColorWhite, 1),
		revealedCard("M3", domain.Num3, domain.ColorWhite, 2),
		revealedCard("M4", domain.Num4, domain.ColorWhite, 3),
	}}
	// 对手 op1 黑1(揭) - 黑?(暗) - 黑3(揭)，暗牌只能是 2，概率 100%。
	r.State.Players["op1"] = &domain.PlayerState{Cards: []*domain.Card{
		revealedCard("A1", domain.Num1, domain.ColorBlack, 0),
		hiddenCard("A2", domain.Num2, domain.ColorBlack, 1),
		revealedCard("A3", domain.Num3, domain.ColorBlack, 2),
	}}

	opt, ok := bestGuessOption(r, "me")
	if !ok {
		t.Fatalf("expected ok")
	}
	if opt.cardID != "A2" || opt.num != domain.Num2 {
		t.Fatalf("expected A2/2, got %s/%d", opt.cardID, opt.num)
	}
	if opt.hitProb < 0.999 {
		t.Fatalf("expected hitProb=1, got %f", opt.hitProb)
	}
}

func TestChooseCardToGetPrefersOpponentColor(t *testing.T) {
	r := newTestRoom()
	r.State.BoardCards["W0"] = &domain.Card{ID: "W0", Num: domain.Num1, Color: domain.ColorWhite, Index: -1}
	r.State.BoardCards["B0"] = &domain.Card{ID: "B0", Num: domain.Num2, Color: domain.ColorBlack, Index: -1}
	// 对手暗牌 3 张全是黑，应当选择黑色公共牌。
	r.State.Players["me"] = &domain.PlayerState{Cards: []*domain.Card{
		revealedCard("M1", domain.Num0, domain.ColorWhite, 0),
		revealedCard("M2", domain.Num1, domain.ColorBlack, 1),
	}}
	r.State.Players["op1"] = &domain.PlayerState{Cards: []*domain.Card{
		hiddenCard("H1", domain.Num4, domain.ColorBlack, 0),
		hiddenCard("H2", domain.Num5, domain.ColorBlack, 1),
		hiddenCard("H3", domain.Num6, domain.ColorBlack, 2),
	}}

	id := chooseCardToGet(r, "me")
	if id != "B0" {
		t.Fatalf("expected B0 (black), got %s", id)
	}
}

func TestChooseCardToGetSingleColorOnly(t *testing.T) {
	r := newTestRoom()
	r.State.BoardCards["W0"] = &domain.Card{ID: "W0", Num: domain.Num1, Color: domain.ColorWhite, Index: -1}
	r.State.Players["me"] = &domain.PlayerState{Cards: []*domain.Card{}}
	id := chooseCardToGet(r, "me")
	if id != "W0" {
		t.Fatalf("expected W0, got %s", id)
	}
}

func TestShouldGuessAgainInSetCardHighProb(t *testing.T) {
	r := newTestRoom()
	// me 已知大量黑色数字，op1 仅剩两张暗黑牌 → 下一手概率 100%。
	r.State.Players["me"] = &domain.PlayerState{Cards: []*domain.Card{
		revealedCard("M0", domain.Num0, domain.ColorBlack, 0),
		revealedCard("M2", domain.Num2, domain.ColorBlack, 1),
		revealedCard("M4", domain.Num4, domain.ColorBlack, 2),
		revealedCard("M5", domain.Num5, domain.ColorBlack, 3),
		revealedCard("M6", domain.Num6, domain.ColorBlack, 4),
		revealedCard("M7", domain.Num7, domain.ColorBlack, 5),
		revealedCard("M8", domain.Num8, domain.ColorBlack, 6),
		revealedCard("M9", domain.Num9, domain.ColorBlack, 7),
		revealedCard("M10", domain.Num10, domain.ColorBlack, 8),
		revealedCard("M11", domain.Num11, domain.ColorBlack, 9),
	}}
	r.State.Players["op1"] = &domain.PlayerState{Cards: []*domain.Card{
		hiddenCard("A1", domain.Num1, domain.ColorBlack, 0),
		hiddenCard("A2", domain.Num3, domain.ColorBlack, 1),
	}}
	if _, ok := shouldGuessAgainInSetCard(r, "me"); !ok {
		t.Fatalf("expected guess again when next prob is high")
	}
}

func TestShouldGuessAgainInSetCardLowProb(t *testing.T) {
	r := newTestRoom()
	r.State.Players["me"] = &domain.PlayerState{Cards: []*domain.Card{
		revealedCard("M1", domain.Num0, domain.ColorWhite, 0),
	}}
	// 对手 4 张全暗白牌，单张暗牌概率 ≪ 0.35。
	r.State.Players["op1"] = &domain.PlayerState{Cards: []*domain.Card{
		hiddenCard("A1", domain.Num1, domain.ColorWhite, 0),
		hiddenCard("A2", domain.Num3, domain.ColorWhite, 1),
		hiddenCard("A3", domain.Num5, domain.ColorWhite, 2),
		hiddenCard("A4", domain.Num7, domain.ColorWhite, 3),
	}}
	if _, ok := shouldGuessAgainInSetCard(r, "me"); ok {
		t.Fatalf("expected fall back to set card when next prob is low")
	}
}

func TestMaybeRunAIIfNeededStopsIfRoomEndsDuringThinkDelay(t *testing.T) {
	r := newTestRoom()
	r.State.RoomStatus = domain.RoomStatusGetCard
	r.State.CurrentPlayer = "ai_001"
	r.State.BoardCards["W0"] = &domain.Card{ID: "W0", Num: domain.Num1, Color: domain.ColorWhite, Index: -1}
	r.State.Players["ai_001"] = &domain.PlayerState{Cards: []*domain.Card{}}
	r.Connections["ai_001"] = &roomcore.PlayerConn{
		PlayerID: "ai_001",
		Online:   true,
		Ready:    true,
		AI:       true,
	}

	oldDelay := aiThinkDelay
	delayCalled := make(chan struct{})
	aiThinkDelay = func() time.Duration {
		r.State.RoomStatus = domain.RoomStatusEnd
		close(delayCalled)
		return 0
	}
	defer func() {
		aiThinkDelay = oldDelay
	}()

	msg := map[string]interface{}{
		"playerId": "ai_001",
		"roomData": map[string]interface{}{
			"currentPlayer": "ai_001",
			"gameStatus":    string(domain.RoomStatusGetCard),
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}

	if !MaybeRunAIIfNeeded(r, data) {
		t.Fatalf("MaybeRunAIIfNeeded returned false before room ended")
	}

	select {
	case <-delayCalled:
	case <-time.After(time.Second):
		t.Fatalf("AI delay was not reached")
	}

	select {
	case cmd := <-r.CmdCh:
		t.Fatalf("unexpected command after room ended during AI delay: %+v", cmd)
	case <-time.After(50 * time.Millisecond):
	}
}
