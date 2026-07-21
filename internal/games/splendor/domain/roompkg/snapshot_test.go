package roompkg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func TestRoomServiceSnapshotUsesActorOwnedState(t *testing.T) {
	rs := NewRoomService("room-1", "owner-1", 4)
	rs.Room.Connections["owner-1"] = &roomcore.PlayerConn{
		PlayerID: "owner-1",
		Online:   true,
		Ready:    true,
	}
	rs.Room.State.RoomStatus = domain.RoomStatusPlaying
	rs.Room.State.Players["owner-1"] = &domain.PlayerState{Score: 7}

	go rs.Run()
	defer close(rs.Room.QuitCh)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	snapshot, err := rs.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}

	if snapshot.RoomID != "room-1" {
		t.Fatalf("room id = %q, want room-1", snapshot.RoomID)
	}
	if snapshot.OwnerID != "owner-1" {
		t.Fatalf("owner id = %q, want owner-1", snapshot.OwnerID)
	}
	if snapshot.Status != string(domain.RoomStatusPlaying) {
		t.Fatalf("status = %q, want %q", snapshot.Status, domain.RoomStatusPlaying)
	}
	if snapshot.MaxPlayers != 4 {
		t.Fatalf("max players = %d, want 4", snapshot.MaxPlayers)
	}
	if snapshot.MaxScore != 7 {
		t.Fatalf("max score = %d, want 7", snapshot.MaxScore)
	}
	if len(snapshot.Players) != 1 || snapshot.Players[0].PlayerID != "owner-1" {
		t.Fatalf("players = %+v, want owner-1", snapshot.Players)
	}
}

func TestRoomServiceSnapshotRespectsContextWhenActorIsNotRunning(t *testing.T) {
	rs := NewRoomService("room-1", "owner-1", 4)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := rs.Snapshot(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Snapshot error = %v, want context deadline exceeded", err)
	}
}

func TestRoomServiceStatusSnapshotDeepCopiesRoomState(t *testing.T) {
	rs := NewRoomService("room-1", "owner-1", 4)
	rs.Room.Connections["owner-1"] = &roomcore.PlayerConn{
		PlayerID: "owner-1",
		Online:   true,
		Ready:    true,
	}
	rs.Room.State.CurrentPlayer = "owner-1"
	rs.Room.State.Players["owner-1"] = &domain.PlayerState{
		Gem:         map[string]int{"Red": 1},
		Score:       3,
		ReserveCard: []entities.NormalCard{{ID: 1, Cost: map[string]int{"Blue": 2}}},
	}
	rs.Room.State.NormalCards["1"] = &entities.NormalCard{ID: 1, Cost: map[string]int{"Red": 1}}
	rs.Room.State.NobleCards["N1"] = &entities.NobleCard{ID: "N1", Cost: map[string]int{"White": 4}}
	rs.Room.State.Gems["Gold"] = 5

	go rs.Run()
	defer close(rs.Room.QuitCh)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	status, err := rs.StatusSnapshot(ctx)
	if err != nil {
		t.Fatalf("StatusSnapshot returned error: %v", err)
	}

	status.Room.State.OwnerID = "mutated-owner"
	status.Room.State.Players["owner-1"].Gem["Red"] = 99
	status.Room.State.Players["owner-1"].ReserveCard[0].Cost["Blue"] = 99
	status.Room.State.NormalCards["1"].Cost["Red"] = 99
	status.Room.State.NobleCards["N1"].Cost["White"] = 99
	status.Room.State.Gems["Gold"] = 0
	status.Room.Connections["owner-1"].Online = false

	fresh, err := rs.StatusSnapshot(ctx)
	if err != nil {
		t.Fatalf("fresh StatusSnapshot returned error: %v", err)
	}
	if fresh.Room.State.OwnerID != "owner-1" {
		t.Fatalf("owner = %q, want owner-1", fresh.Room.State.OwnerID)
	}
	if fresh.Room.State.Players["owner-1"].Gem["Red"] != 1 {
		t.Fatalf("gem = %d, want 1", fresh.Room.State.Players["owner-1"].Gem["Red"])
	}
	if fresh.Room.State.Players["owner-1"].ReserveCard[0].Cost["Blue"] != 2 {
		t.Fatalf("reserve cost = %d, want 2", fresh.Room.State.Players["owner-1"].ReserveCard[0].Cost["Blue"])
	}
	if fresh.Room.State.NormalCards["1"].Cost["Red"] != 1 {
		t.Fatalf("normal card cost = %d, want 1", fresh.Room.State.NormalCards["1"].Cost["Red"])
	}
	if fresh.Room.State.NobleCards["N1"].Cost["White"] != 4 {
		t.Fatalf("noble card cost = %d, want 4", fresh.Room.State.NobleCards["N1"].Cost["White"])
	}
	if fresh.Room.State.Gems["Gold"] != 5 {
		t.Fatalf("room gem = %d, want 5", fresh.Room.State.Gems["Gold"])
	}
	if !fresh.Room.Connections["owner-1"].Online {
		t.Fatal("connection online = false, want true")
	}
}
