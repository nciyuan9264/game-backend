package roompkg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func TestRoomServiceSnapshotUsesActorOwnedState(t *testing.T) {
	rs := NewRoomService("room-1", "owner-1")
	rs.Room.Connections["owner-1"] = &roomcore.PlayerConn{
		PlayerID: "owner-1",
		Online:   true,
		Ready:    true,
	}
	rs.Room.State.RoomStatus = domain.RoomStatusGuessCard
	rs.Room.State.BoardCards["1A"] = &domain.Card{ID: "1A"}

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
	if snapshot.Status != string(domain.RoomStatusGuessCard) {
		t.Fatalf("status = %q, want %q", snapshot.Status, domain.RoomStatusGuessCard)
	}
	if snapshot.BoardCardCount != 1 {
		t.Fatalf("board card count = %d, want 1", snapshot.BoardCardCount)
	}
	if len(snapshot.Players) != 1 || snapshot.Players[0].PlayerID != "owner-1" {
		t.Fatalf("players = %+v, want owner-1", snapshot.Players)
	}
}

func TestRoomServiceSnapshotRespectsContextWhenActorIsNotRunning(t *testing.T) {
	rs := NewRoomService("room-1", "owner-1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := rs.Snapshot(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Snapshot error = %v, want context deadline exceeded", err)
	}
}

func TestRoomServiceStatusSnapshotDeepCopiesRoomState(t *testing.T) {
	rs := NewRoomService("room-1", "owner-1")
	rs.Room.Connections["owner-1"] = &roomcore.PlayerConn{
		PlayerID: "owner-1",
		Online:   true,
		Ready:    true,
	}
	rs.Room.State.CurrentPlayer = "owner-1"
	rs.Room.State.BoardCards["1A"] = &domain.Card{ID: "1A", Color: domain.ColorWhite, Num: domain.Num1}
	rs.Room.State.Players["owner-1"] = &domain.PlayerState{
		Cards: []*domain.Card{{ID: "2B", Color: domain.ColorBlack, Num: domain.Num2}},
	}

	go rs.Run()
	defer close(rs.Room.QuitCh)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	status, err := rs.StatusSnapshot(ctx)
	if err != nil {
		t.Fatalf("StatusSnapshot returned error: %v", err)
	}

	status.Room.State.OwnerID = "mutated-owner"
	status.Room.State.BoardCards["1A"].Num = domain.Num9
	status.Room.State.Players["owner-1"].Cards[0].Num = domain.Num10
	status.Room.Connections["owner-1"].Online = false

	fresh, err := rs.StatusSnapshot(ctx)
	if err != nil {
		t.Fatalf("fresh StatusSnapshot returned error: %v", err)
	}
	if fresh.Room.State.OwnerID != "owner-1" {
		t.Fatalf("owner = %q, want owner-1", fresh.Room.State.OwnerID)
	}
	if fresh.Room.State.BoardCards["1A"].Num != domain.Num1 {
		t.Fatalf("board card num = %d, want %d", fresh.Room.State.BoardCards["1A"].Num, domain.Num1)
	}
	if fresh.Room.State.Players["owner-1"].Cards[0].Num != domain.Num2 {
		t.Fatalf("player card num = %d, want %d", fresh.Room.State.Players["owner-1"].Cards[0].Num, domain.Num2)
	}
	if !fresh.Room.Connections["owner-1"].Online {
		t.Fatal("connection online = false, want true")
	}
}
