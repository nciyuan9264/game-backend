package roompkg

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
)

type protocolConn struct {
	messages [][]byte
	closed   bool
}

func (c *protocolConn) WriteMessage(_ int, data []byte) error {
	c.messages = append(c.messages, append([]byte(nil), data...))
	return nil
}
func (c *protocolConn) ReadMessage() (int, []byte, error) { return 0, nil, nil }
func (c *protocolConn) Close() error                      { c.closed = true; return nil }
func (c *protocolConn) lastCode(t *testing.T) string {
	t.Helper()
	if len(c.messages) == 0 {
		t.Fatal("expected protocol response")
	}
	var response struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(c.messages[len(c.messages)-1], &response); err != nil {
		t.Fatal(err)
	}
	return response.Code
}

func protocolRoom() (*RoomService, *protocolConn, *protocolConn) {
	room := NewRoomService("protocol-room", "owner")
	owner, guest := &protocolConn{}, &protocolConn{}
	room.handleCommand(domain.Command{Type: "connect", PlayerID: "owner", Conn: owner, Name: "Owner"})
	room.handleCommand(domain.Command{Type: "connect", PlayerID: "guest", Conn: guest, Name: "Guest"})
	return room, owner, guest
}

func TestOwnerCommandsRejectGuestAndStaleConnection(t *testing.T) {
	for _, command := range []string{"match_begin", "match_add_ai", "match_remove_player", "game_restart_game"} {
		t.Run(command, func(t *testing.T) {
			room, _, guest := protocolRoom()
			payload := []byte(`{"playerID":"owner"}`)
			if room.handleCommand(domain.Command{
				Type: command, PlayerID: "guest", Conn: guest, Payload: payload,
			}) {
				t.Fatal("guest command accepted")
			}
			if guest.lastCode(t) != "forbidden" {
				t.Fatalf("code = %q, want forbidden", guest.lastCode(t))
			}
		})
	}

	room, owner, _ := protocolRoom()
	duplicate := &protocolConn{}
	if room.handleCommand(domain.Command{Type: "connect", PlayerID: "owner", Conn: duplicate}) {
		t.Fatal("duplicate connection accepted")
	}
	if !duplicate.closed || duplicate.lastCode(t) != "identity_conflict" {
		t.Fatal("duplicate connection did not receive identity conflict")
	}
	room.handleCommand(domain.Command{Type: "disconnect", PlayerID: "owner", Conn: duplicate})
	if room.Room.Connections["owner"].Conn != owner || !room.Room.Connections["owner"].Online {
		t.Fatal("stale connection disconnected the active session")
	}
}

func TestGameActionsValidateTurnPhaseAndPayload(t *testing.T) {
	room, owner, guest := protocolRoom()
	room.Room.State.RoomStatus = domain.RoomStatusGetCard
	room.Room.State.CurrentPlayer = "owner"
	room.Room.State.Players["owner"] = &domain.PlayerState{Cards: []*domain.Card{}}
	room.Room.State.Players["guest"] = &domain.PlayerState{Cards: []*domain.Card{
		{ID: "guest-card", Num: domain.Num3, Index: 0},
	}}
	room.Room.State.BoardCards["pool-card"] = &domain.Card{ID: "pool-card", Num: domain.Num2, Index: -1}

	if room.handleCommand(domain.Command{
		Type: "game_get_card", PlayerID: "guest", Conn: guest, Payload: []byte(`{"id":"pool-card"}`),
	}) || guest.lastCode(t) != "not_your_turn" {
		t.Fatal("non-current player drew a card")
	}
	if room.handleCommand(domain.Command{
		Type: "game_get_card", PlayerID: "owner", Conn: owner, Payload: []byte(`{"id":"missing"}`),
	}) || owner.lastCode(t) != "invalid_card" {
		t.Fatal("missing pool card was accepted")
	}
	if !room.handleCommand(domain.Command{
		Type: "game_get_card", PlayerID: "owner", Conn: owner, Payload: []byte(`{"id":"pool-card"}`),
	}) {
		t.Fatal("valid draw was rejected")
	}
	if room.Room.State.RoomStatus != domain.RoomStatusGuessCard {
		t.Fatal("draw did not advance to guess phase")
	}
	if room.handleCommand(domain.Command{
		Type: "game_guess_card", PlayerID: "owner", Conn: owner,
		Payload: []byte(`{"id":"guest-card","num":12}`),
	}) || owner.lastCode(t) != "invalid_guess" {
		t.Fatal("out-of-range guess was accepted")
	}
}

func TestSetCardRequiresDrawnCardAndLegalPosition(t *testing.T) {
	room, owner, _ := protocolRoom()
	room.Room.State.RoomStatus = domain.RoomStatusSetCard
	room.Room.State.CurrentPlayer = "owner"
	drawn := &domain.Card{ID: "drawn", Num: domain.Num4, Color: domain.ColorWhite, Index: -1}
	room.Room.State.Players["owner"] = &domain.PlayerState{Cards: []*domain.Card{
		{ID: "one", Num: domain.Num1, Index: 0},
		{ID: "six", Num: domain.Num6, Index: 1},
		drawn,
	}}

	if room.handleCommand(domain.Command{
		Type: "game_set_card", PlayerID: "owner", Conn: owner,
		Payload: []byte(`{"id":"one","index":0}`),
	}) || owner.lastCode(t) != "invalid_card" {
		t.Fatal("existing hand card was accepted as newly drawn card")
	}
	if room.handleCommand(domain.Command{
		Type: "game_set_card", PlayerID: "owner", Conn: owner,
		Payload: []byte(`{"id":"drawn","index":0}`),
	}) || owner.lastCode(t) != "invalid_position" {
		t.Fatal("unsorted numeric-card position was accepted")
	}
	if !room.handleCommand(domain.Command{
		Type: "game_set_card", PlayerID: "owner", Conn: owner,
		Payload: []byte(`{"id":"drawn","index":1}`),
	}) {
		t.Fatal("legal numeric-card position was rejected")
	}
}

func TestPhaseVersionRejectsStaleCommand(t *testing.T) {
	room, owner, _ := protocolRoom()
	room.Room.State.RoomStatus = domain.RoomStatusGetCard
	room.Room.State.CurrentPlayer = "owner"
	room.Room.State.PhaseVersion = 7
	room.Room.State.Players["owner"] = &domain.PlayerState{}
	room.Room.State.BoardCards["pool-card"] = &domain.Card{ID: "pool-card"}
	stale := uint64(6)

	if room.handleCommand(domain.Command{
		Type: "game_get_card", PlayerID: "owner", Conn: owner,
		Payload: []byte(`{"id":"pool-card"}`), PhaseVersion: &stale,
	}) || owner.lastCode(t) != "phase_changed" {
		t.Fatal("stale command was accepted")
	}
	if room.Room.State.BoardCards["pool-card"] == nil {
		t.Fatal("stale command mutated room state")
	}
}

func TestResolveTimeoutRunsOnlyForCurrentExpiredPhase(t *testing.T) {
	room, _, _ := protocolRoom()
	room.Room.State.RoomStatus = domain.RoomStatusGetCard
	room.Room.State.CurrentPlayer = "owner"
	room.Room.State.PhaseVersion = 7
	room.Room.State.Players["owner"] = &domain.PlayerState{}
	room.Room.State.BoardCards["pool-card"] = &domain.Card{ID: "pool-card"}
	room.Room.TurnDeadline = time.Now().Add(-time.Millisecond)

	stale := uint64(6)
	if _, ok := room.resolveTimeout(domain.Command{
		Type: "davinci_timeout", PlayerID: "owner", PhaseVersion: &stale,
	}); ok {
		t.Fatal("stale timeout was accepted")
	}

	current := uint64(7)
	action, ok := room.resolveTimeout(domain.Command{
		Type: "davinci_timeout", PlayerID: "owner", PhaseVersion: &current,
	})
	if !ok || action.PlayerID != "owner" || action.Type != "game_get_card" {
		t.Fatalf("current timeout action = %+v, ok = %v", action, ok)
	}
}
