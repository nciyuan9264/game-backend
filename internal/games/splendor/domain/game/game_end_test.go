package game

import (
	"encoding/json"
	"testing"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// newEndTestRoom 构造一个两人局：p1 已达终局分数线（15+），p2 落后。
// PlayerSeq = [p1, p2]，FirstPlayer = p1，当前轮到 p2（即 p1 已完成触发最后一轮的那手）。
// 这样 p2 出手结束后，回合应循环回到 FirstPlayer(p1)，游戏应立即结束。
func newEndTestRoom(currentPlayer string, p1Score int) *domain.Room {
	base := roomcore.NewBase("splendor-end-test", 8)
	base.Connections["p1"] = &roomcore.PlayerConn{PlayerID: "p1"}
	base.Connections["p2"] = &roomcore.PlayerConn{PlayerID: "p2"}
	base.PlayerSeq = []string{"p1", "p2"}

	return &domain.Room{
		Base: base,
		State: &domain.GameState{
			RoomStatus:    domain.RoomStatusLastTurn,
			CurrentPlayer: currentPlayer,
			FirstPlayer:   "p1",
			Gems:          map[string]int{"Red": 5, "Green": 5, "Blue": 5, "White": 5, "Black": 5, "Gold": 5},
			Players: map[string]*domain.PlayerState{
				"p1": {Score: p1Score, Gem: map[string]int{}},
				"p2": {Score: 11, Gem: map[string]int{}},
			},
			NormalCards: map[string]*entities.NormalCard{},
		},
	}
}

func TestGetGemEndsGameOnLastTurnWrap(t *testing.T) {
	r := newEndTestRoom("p2", 18)

	cmd := domain.Command{PlayerID: "p2", Payload: json.RawMessage(`{"Red":1}`)}
	HandleGetGemMessage(r, cmd)

	if r.State.RoomStatus != domain.RoomStatusEnd {
		t.Fatalf("拿宝石结束最后一轮后应立即结束，got status=%s current=%s", r.State.RoomStatus, r.State.CurrentPlayer)
	}
}

func TestTurnTimeoutEndsGameOnLastTurnWrap(t *testing.T) {
	r := newEndTestRoom("p2", 18)

	cmd := domain.Command{PlayerID: "p2", Payload: json.RawMessage(`{}`)}
	HandleTurnTimeoutMessage(r, cmd)

	if r.State.RoomStatus != domain.RoomStatusEnd {
		t.Fatalf("超时结束最后一轮后应立即结束，got status=%s current=%s", r.State.RoomStatus, r.State.CurrentPlayer)
	}
}

func TestGetGemEntersLastTurnWhenPlayerCrossesThreshold(t *testing.T) {
	// p1 刚满 15 分、仍是 playing，且轮到 p1 出手：出手后应进入 last_turn 并切到 p2。
	r := newEndTestRoom("p1", 15)
	r.State.RoomStatus = domain.RoomStatusPlaying

	cmd := domain.Command{PlayerID: "p1", Payload: json.RawMessage(`{"Red":1}`)}
	HandleGetGemMessage(r, cmd)

	if r.State.RoomStatus != domain.RoomStatusLastTurn {
		t.Fatalf("有玩家达到 15 分后应进入最后一轮，got status=%s", r.State.RoomStatus)
	}
	if r.State.CurrentPlayer != "p2" {
		t.Fatalf("应切换到 p2，got current=%s", r.State.CurrentPlayer)
	}
}

func TestActionRejectedAfterGameEnd(t *testing.T) {
	r := newEndTestRoom("p2", 18)
	r.State.RoomStatus = domain.RoomStatusEnd
	r.State.CurrentPlayer = "p2"

	before := r.State.Players["p2"].Gem["Red"]
	cmd := domain.Command{PlayerID: "p2", Payload: json.RawMessage(`{"Red":1}`)}
	HandleGetGemMessage(r, cmd)

	if got := r.State.Players["p2"].Gem["Red"]; got != before {
		t.Fatalf("游戏结束后不应再改动状态，Red gem before=%d after=%d", before, got)
	}
}
