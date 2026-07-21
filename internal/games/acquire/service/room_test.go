package service

import (
	"context"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
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
		AI:       false,
	}
	rs.Room.State.RoomStatus = domain.RoomStatusSetTile
	rs.Room.State.BoardTiles["1A"] = &domain.Tile{ID: "1A", Belong: ""}
	rs.Room.State.BoardTiles["1B"] = &domain.Tile{ID: "1B", Belong: "Tower"}

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
	if room.Status != string(domain.RoomStatusSetTile) {
		t.Fatalf("status = %q, want %q", room.Status, domain.RoomStatusSetTile)
	}
	if room.EmptyTileCount != 1 {
		t.Fatalf("empty tile count = %d, want 1", room.EmptyTileCount)
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
		AI:       false,
	}
	rs.Room.State.RoomStatus = domain.RoomStatusBuyStock
	rs.Room.State.CurrentPlayer = "owner-1"
	rs.Room.State.BoardTiles["1A"] = &domain.Tile{ID: "1A", Belong: ""}
	rs.Room.State.Players["owner-1"] = &domain.PlayerState{
		Money:  6000,
		Stocks: map[string]int{"Tower": 1},
		Tiles:  []string{"1A"},
	}
	rs.Room.State.Companies["Tower"] = &domain.CompanyState{
		Name:       "Tower",
		Tiles:      2,
		StockTotal: 24,
		StockPrice: 200,
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
	status.Room.State.Players["owner-1"].Money = 1
	status.Room.State.Players["owner-1"].Stocks["Tower"] = 99
	status.Room.State.Players["owner-1"].Tiles[0] = "9I"
	status.Room.State.BoardTiles["1A"].Belong = "Tower"
	status.Room.State.Companies["Tower"].StockTotal = 1
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
	if snapshot.Players[0].Online != true {
		t.Fatalf("actor player online = %v, want true", snapshot.Players[0].Online)
	}
}
