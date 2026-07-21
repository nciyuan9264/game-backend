package service

import (
	"context"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/roompkg"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func TestGetRoomListReturnsActorSnapshots(t *testing.T) {
	roompkg.Rooms.Clear()
	t.Cleanup(roompkg.Rooms.Clear)

	rs := roompkg.NewRoomService("room-list-1", "owner-1")
	rs.Room.Connections["owner-1"] = &roomcore.PlayerConn{
		PlayerID: "owner-1",
		Online:   true,
		Ready:    true,
	}
	rs.Room.State.RoomStatus = domain.RoomStatusGuessCard
	rs.Room.State.BoardCards["1A"] = &domain.Card{ID: "1A"}

	roompkg.Rooms.Set(rs.Room.ID, rs)
	go rs.Run()
	t.Cleanup(func() {
		close(rs.Room.QuitCh)
	})

	rooms, err := GetRoomList()
	if err != nil {
		t.Fatalf("GetRoomList returned error: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("rooms len = %d, want 1", len(rooms))
	}
	room := rooms[0]
	if room.RoomID != "room-list-1" {
		t.Fatalf("room id = %q, want room-list-1", room.RoomID)
	}
	if room.OwnerID != "owner-1" {
		t.Fatalf("owner id = %q, want owner-1", room.OwnerID)
	}
	if room.Status != string(domain.RoomStatusGuessCard) {
		t.Fatalf("status = %q, want %q", room.Status, domain.RoomStatusGuessCard)
	}
	if room.BoardCardCount != 1 {
		t.Fatalf("board card count = %d, want 1", room.BoardCardCount)
	}
	if len(room.RoomPlayer) != 1 || room.RoomPlayer[0].PlayerID != "owner-1" {
		t.Fatalf("room players = %+v, want owner-1", room.RoomPlayer)
	}
}

func TestGetGameStatusReturnsActorOwnedDeepSnapshot(t *testing.T) {
	roompkg.Rooms.Clear()
	t.Cleanup(roompkg.Rooms.Clear)

	rs := roompkg.NewRoomService("room-status-1", "owner-1")
	rs.Room.Connections["owner-1"] = &roomcore.PlayerConn{
		PlayerID: "owner-1",
		Online:   true,
		Ready:    true,
	}
	rs.Room.State.CurrentPlayer = "owner-1"
	rs.Room.State.BoardCards["1A"] = &domain.Card{ID: "1A", Num: domain.Num1}
	rs.Room.State.Players["owner-1"] = &domain.PlayerState{
		Cards: []*domain.Card{{ID: "2B", Num: domain.Num2}},
	}

	roompkg.Rooms.Set(rs.Room.ID, rs)
	go rs.Run()
	t.Cleanup(func() {
		close(rs.Room.QuitCh)
	})

	status := GetGameStatus("room-status-1")
	if status == nil {
		t.Fatal("GetGameStatus returned nil")
	}

	status.Room.State.OwnerID = "mutated-owner"
	status.Room.State.BoardCards["1A"].Num = domain.Num9
	status.Room.State.Players["owner-1"].Cards[0].Num = domain.Num10
	status.Room.Connections["owner-1"].Online = false

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snapshot, err := rs.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.OwnerID != "owner-1" {
		t.Fatalf("actor owner = %q, want owner-1", snapshot.OwnerID)
	}
	if !snapshot.Players[0].Online {
		t.Fatal("actor player online = false, want true")
	}
}
