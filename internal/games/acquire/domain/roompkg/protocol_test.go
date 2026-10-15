package roompkg

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

type protocolConn struct {
	messages [][]byte
	closed   bool
}

func (c *protocolConn) WriteMessage(_ int, b []byte) error {
	c.messages = append(c.messages, append([]byte(nil), b...))
	return nil
}
func (c *protocolConn) ReadMessage() (int, []byte, error) { return 0, nil, nil }
func (c *protocolConn) Close() error                      { c.closed = true; return nil }
func (c *protocolConn) lastCode(t *testing.T) string {
	t.Helper()
	var message struct {
		Code string `json:"code"`
	}
	if len(c.messages) == 0 {
		t.Fatal("expected response")
	}
	if err := json.Unmarshal(c.messages[len(c.messages)-1], &message); err != nil {
		t.Fatal(err)
	}
	return message.Code
}

func protocolRoom() (*RoomService, *protocolConn, *protocolConn) {
	r := NewRoomService("test", "owner")
	owner, guest := &protocolConn{}, &protocolConn{}
	r.handleCommand(domain.Command{Type: "connect", PlayerID: "owner", Conn: owner, Name: "Owner", Avatar: "avatar"})
	r.handleCommand(domain.Command{Type: "connect", PlayerID: "guest", Conn: guest, Name: "Guest"})
	return r, owner, guest
}

func TestSettingsAndStartAreOwnerOnly(t *testing.T) {
	for _, command := range []string{"room_update_settings", "match_add_ai", "match_remove_player", "match_begin"} {
		t.Run(command, func(t *testing.T) {
			r, _, guest := protocolRoom()
			if r.handleCommand(domain.Command{Type: command, PlayerID: "guest", Conn: guest, Payload: []byte(`{"visibility":"private","playerID":"owner"}`)}) {
				t.Fatal("guest command accepted")
			}
			if guest.lastCode(t) != "forbidden" || len(r.Room.Connections) != 2 || r.Room.State.Visibility != "public" {
				t.Fatal("permission invariant failed")
			}
		})
	}
	r, owner, _ := protocolRoom()
	cmd := domain.Command{Type: "room_update_settings", PlayerID: "owner", Conn: owner, RequestID: "setting-1", Payload: []byte(`{"visibility":"private"}`)}
	if !r.handleCommand(cmd) || r.Room.State.Visibility != "private" {
		t.Fatal("owner could not set visibility")
	}
	r.Room.State.RoomStatus = domain.RoomStatusWaiting
	if r.handleCommand(cmd) || owner.lastCode(t) != "phase_changed" {
		t.Fatal("settings accepted after start")
	}
}

func TestRejectedConnectionCannotDisconnectOrCommandAsExistingPlayer(t *testing.T) {
	r, owner, _ := protocolRoom()
	duplicate := &protocolConn{}
	if r.handleCommand(domain.Command{Type: "connect", PlayerID: "owner", Conn: duplicate}) {
		t.Fatal("duplicate accepted")
	}
	if !duplicate.closed || duplicate.lastCode(t) != "identity_conflict" {
		t.Fatal("missing conflict response")
	}
	r.handleCommand(domain.Command{Type: "disconnect", PlayerID: "owner", Conn: duplicate})
	r.handleCommand(domain.Command{Type: "room_update_settings", PlayerID: "owner", Conn: duplicate, Payload: []byte(`{"visibility":"private"}`)})
	if r.Room.Connections["owner"].Conn != owner || r.Room.State.Visibility != "public" {
		t.Fatal("old connection affected current session")
	}
	r.Room.State.RoomStatus = domain.RoomStatusBuyStock
	r.handleCommand(domain.Command{Type: "disconnect", PlayerID: "owner", Conn: owner})
	fresh := &protocolConn{}
	if !r.handleCommand(domain.Command{Type: "connect", PlayerID: "owner", Conn: fresh}) {
		t.Fatal("reconnect failed")
	}
	r.handleCommand(domain.Command{Type: "disconnect", PlayerID: "owner", Conn: owner})
	if r.Room.Connections["owner"].Conn != fresh || !r.Room.Connections["owner"].Online {
		t.Fatal("late disconnect cleared new session")
	}
}

func TestInvalidStocksCannotPartiallyMutateAssets(t *testing.T) {
	for _, payload := range []string{
		`{"stocks":{"Tower":4}}`, `{"stocks":{"Tower":-1}}`, `{"stocks":{"missing":1}}`,
		`{"stocks":{"Tower":2,"Sackson":2}}`, `{"stocks":{"Tower":3}}`,
	} {
		t.Run(payload, func(t *testing.T) {
			r, owner, _ := protocolRoom()
			r.Room.State.RoomStatus = domain.RoomStatusBuyStock
			r.Room.State.CurrentPlayer = "owner"
			r.Room.State.Players["owner"] = &domain.PlayerState{Money: 500, Stocks: map[string]int{}}
			r.Room.State.Companies["Tower"] = &domain.CompanyState{Tiles: 2, StockTotal: 25, StockPrice: 200}
			r.Room.State.Companies["Sackson"] = &domain.CompanyState{Tiles: 2, StockTotal: 25, StockPrice: 200}
			if r.handleCommand(domain.Command{Type: "game_buy_stock", PlayerID: "owner", Conn: owner, Payload: []byte(payload)}) {
				t.Fatal("invalid stocks accepted")
			}
			if r.Room.State.Players["owner"].Money != 500 || r.Room.State.Companies["Tower"].StockTotal != 25 {
				t.Fatal("partial mutation on rejection")
			}
		})
	}
}

func TestMergeExchangeUsesSharedInventoryAndEvenAmounts(t *testing.T) {
	for _, payload := range []string{
		`{"actions":[{"company":"Tower","exchangeAmount":3}]}`,
		`{"actions":[{"company":"Tower","exchangeAmount":4},{"company":"Sackson","exchangeAmount":4}]}`,
		`{"actions":[{"company":"Tower","sellAmount":3,"exchangeAmount":2}]}`,
		`{"actions":[{"company":"Tower","sellAmount":2},{"company":"Tower","sellAmount":2}]}`,
	} {
		r, owner, _ := protocolRoom()
		r.Room.State.RoomStatus = domain.RoomStatusMergingSettle
		r.Room.State.Players["owner"] = &domain.PlayerState{Money: 0, Stocks: map[string]int{"Tower": 4, "Sackson": 4}}
		r.Room.State.MergeMainCompany = "American"
		r.Room.State.Companies["American"] = &domain.CompanyState{StockTotal: 3}
		r.Room.State.MergeSettleData = map[string]domain.SettleData{"Tower": {Hoders: []string{"owner"}}, "Sackson": {Hoders: []string{"owner"}}}
		if r.handleCommand(domain.Command{Type: "game_merging_settle", PlayerID: "owner", Conn: owner, Payload: []byte(payload)}) {
			t.Fatalf("invalid exchange accepted: %s", payload)
		}
		if r.Room.State.Players["owner"].Stocks["Tower"] != 4 {
			t.Fatal("holdings changed on rejected exchange")
		}
	}
}

func TestPhaseVersionTracksTurnsButNotMergeHolderChanges(t *testing.T) {
	r, owner, _ := protocolRoom()
	r.Room.State.RoomStatus = domain.RoomStatusSetTile
	r.Room.State.CurrentPlayer = "owner"
	r.advancePhase(domain.RoomStatusWaiting, true)
	first := r.Room.State.PhaseVersion
	if r.Room.State.TurnNumber != 1 {
		t.Fatal("first turn must be one")
	}
	r.advancePhase(domain.RoomStatusSetTile, false)
	if r.Room.State.PhaseVersion != first {
		t.Fatal("duplicate snapshot changed phase")
	}
	r.Room.State.CurrentPlayer = "guest"
	r.advancePhase(domain.RoomStatusSetTile, true)
	if r.Room.State.TurnNumber != 2 || r.Room.State.PhaseVersion != first+1 {
		t.Fatal("next turn not counted")
	}
	r.Room.State.RoomStatus = domain.RoomStatusMergingSettle
	r.advancePhase(domain.RoomStatusSetTile, false)
	merge := r.Room.State.PhaseVersion
	r.Room.State.CurrentPlayer = "owner"
	r.advancePhase(domain.RoomStatusMergingSettle, false)
	if r.Room.State.PhaseVersion != merge {
		t.Fatal("holder change invalidated merge drafts")
	}
	if r.handleCommand(domain.Command{Type: "game_merging_settle", PlayerID: "owner", Conn: owner, PhaseVersion: &first}) || owner.lastCode(t) != "phase_changed" {
		t.Fatal("stale phase accepted")
	}
	r.Room.State.RoomStatus = domain.RoomStatusMatch
	r.advancePhase(domain.RoomStatusEnd, false)
	if r.Room.State.TurnNumber != 0 || r.Room.State.PhaseVersion <= merge {
		t.Fatal("restart reset phase backwards")
	}
}

func TestRoomCapacityAndStartConditions(t *testing.T) {
	r, owner, guest := protocolRoom()
	if r.handleCommand(domain.Command{Type: "match_begin", PlayerID: "owner", Conn: owner}) {
		t.Fatal("unready room started")
	}
	r.handleCommand(domain.Command{Type: "match_ready", PlayerID: "guest", Conn: guest, Payload: []byte(`{"ready":true}`)})
	for i := 0; i < 4; i++ {
		if !r.handleCommand(domain.Command{Type: "match_add_ai", PlayerID: "owner", Conn: owner}) {
			t.Fatal("AI add failed")
		}
	}
	extra := &protocolConn{}
	if r.handleCommand(domain.Command{Type: "connect", PlayerID: "extra", Conn: extra}) || extra.lastCode(t) != "room_full" {
		t.Fatal("seventh player accepted")
	}
	if !r.handleCommand(domain.Command{Type: "match_begin", PlayerID: "owner", Conn: owner}) || r.Room.State.RoomStatus != domain.RoomStatusWaiting {
		t.Fatal("ready room failed to start")
	}
}

func TestAddAITargetsRequestedSeat(t *testing.T) {
	r, owner, _ := protocolRoom()
	if !r.handleCommand(domain.Command{
		Type: "match_add_ai", PlayerID: "owner", Conn: owner, Payload: []byte(`{"seatIndex":4}`),
	}) {
		t.Fatal("targeted AI add failed")
	}
	var added *roomcore.PlayerConn
	for _, player := range r.Room.Connections {
		if player.AI {
			added = player
			break
		}
	}
	if added == nil || added.SeatIndex != 4 {
		t.Fatalf("AI seat = %v, want 4", added)
	}
	if r.handleCommand(domain.Command{
		Type: "match_add_ai", PlayerID: "owner", Conn: owner, Payload: []byte(`{"seatIndex":4}`),
	}) || owner.lastCode(t) != "seat_occupied" {
		t.Fatal("occupied seat accepted")
	}
}

func TestExpiredTimeoutDoesNotExecuteBeforeCurrentDeadline(t *testing.T) {
	r, _, _ := protocolRoom()
	r.Room.State.RoomStatus = domain.RoomStatusSetTile
	r.Room.State.CurrentPlayer = "owner"
	r.Room.TurnDeadline = time.Now().Add(time.Minute)
	version := r.Room.State.PhaseVersion
	if _, ok := r.resolveTimeout(domain.Command{Type: "acquire_timeout", PlayerID: "owner", PhaseVersion: &version}); ok {
		t.Fatal("obsolete timeout executed against new deadline")
	}
}

func TestDisconnectWithoutReferenceRemainsCompatible(t *testing.T) {
	r, _, _ := protocolRoom()
	roomcore.HandleDisconnect(r.svc, domain.Command{Type: "disconnect", PlayerID: "guest"})
	if r.Room.Connections["guest"] != nil {
		t.Fatal("legacy internal disconnect changed behavior")
	}
}

func TestEndVoteRequiresEligibilityAndExplicitBoolean(t *testing.T) {
	r, owner, _ := protocolRoom()
	r.Room.State.RoomStatus = domain.RoomStatusSetTile
	r.Room.State.EndGame = domain.EndGameState{Votes: map[string]bool{}}

	if r.handleCommand(domain.Command{
		Type: "game_end_vote", PlayerID: "owner", Conn: owner, Payload: []byte(`{"allow":true}`),
	}) || owner.lastCode(t) != "end_not_available" {
		t.Fatal("vote accepted before eligibility")
	}

	r.Room.State.EndGame.Eligible = true
	if r.handleCommand(domain.Command{
		Type: "game_end_vote", PlayerID: "owner", Conn: owner, Payload: []byte(`{}`),
	}) || owner.lastCode(t) != "invalid_payload" {
		t.Fatal("vote without explicit boolean accepted")
	}

	if !r.handleCommand(domain.Command{
		Type: "game_end_vote", PlayerID: "owner", Conn: owner, Payload: []byte(`{"allow":true}`),
	}) {
		t.Fatal("valid approval rejected")
	}
	if !r.Room.State.EndGame.Votes["owner"] || r.Room.State.RoomStatus != domain.RoomStatusSetTile {
		t.Fatal("approval did not remain a non-terminal state update")
	}

	if !r.handleCommand(domain.Command{
		Type: "game_end_vote", PlayerID: "owner", Conn: owner, Payload: []byte(`{"allow":false}`),
	}) || r.Room.State.EndGame.Votes["owner"] {
		t.Fatal("valid revocation rejected")
	}
}

func TestCommandCompletedTurnHandlesSamePlayablePlayer(t *testing.T) {
	state := &domain.GameState{RoomStatus: domain.RoomStatusSetTile, CurrentPlayer: "owner"}
	if !commandCompletedTurn("game_buy_stock", domain.RoomStatusBuyStock, "owner", state) {
		t.Fatal("buy completion was not recognized when empty players were skipped")
	}
	if !commandCompletedTurn("game_place_tile", domain.RoomStatusSetTile, "owner", state) {
		t.Fatal("no-company placement completion was not recognized")
	}
	if commandCompletedTurn("game_end_vote", domain.RoomStatusSetTile, "owner", state) {
		t.Fatal("vote was treated as a completed turn")
	}
}
