package roompkg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

func TestRoomServiceSnapshotUsesActorOwnedState(t *testing.T) {
	rs := NewRoomService("room-1", "owner-1")
	rs.Room.Connections["owner-1"] = &roomcore.PlayerConn{
		PlayerID: "owner-1",
		Online:   true,
		Ready:    true,
		AI:       false,
	}
	rs.Room.Connections["ai-1"] = &roomcore.PlayerConn{
		PlayerID: "ai-1",
		Online:   true,
		Ready:    true,
		AI:       true,
	}
	rs.Room.State.RoomStatus = domain.RoomStatusSetTile
	rs.Room.State.BoardTiles["1A"] = &domain.Tile{ID: "1A", Belong: ""}
	rs.Room.State.BoardTiles["1B"] = &domain.Tile{ID: "1B", Belong: "Tower"}

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
	if snapshot.Status != string(domain.RoomStatusSetTile) {
		t.Fatalf("status = %q, want %q", snapshot.Status, domain.RoomStatusSetTile)
	}
	if snapshot.EmptyTileCount != 1 {
		t.Fatalf("empty tile count = %d, want 1", snapshot.EmptyTileCount)
	}
	if len(snapshot.Players) != 2 {
		t.Fatalf("players len = %d, want 2", len(snapshot.Players))
	}
	if snapshot.Players[0].PlayerID == snapshot.Players[1].PlayerID {
		t.Fatalf("players should contain distinct players: %+v", snapshot.Players)
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
	rs.Room.State.MergeSettleData = map[string]domain.SettleData{
		"Tower": {
			Hoders:    []string{"owner-1"},
			Dividends: map[string]int{"owner-1": 1000},
		},
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
	status.Room.State.BoardTiles["1A"].Belong = "Tower"
	status.Room.State.Players["owner-1"].Money = 1
	status.Room.State.Players["owner-1"].Stocks["Tower"] = 99
	status.Room.State.Players["owner-1"].Tiles[0] = "9I"
	status.Room.State.Companies["Tower"].StockTotal = 1
	settle := status.Room.State.MergeSettleData["Tower"]
	settle.Hoders[0] = "mutated-holder"
	settle.Dividends["owner-1"] = 1
	status.Room.State.MergeSettleData["Tower"] = settle
	status.Room.Connections["owner-1"].Online = false

	fresh, err := rs.StatusSnapshot(ctx)
	if err != nil {
		t.Fatalf("fresh StatusSnapshot returned error: %v", err)
	}
	if fresh.Room.State.OwnerID != "owner-1" {
		t.Fatalf("owner = %q, want owner-1", fresh.Room.State.OwnerID)
	}
	if fresh.Room.State.BoardTiles["1A"].Belong != "" {
		t.Fatalf("tile belong = %q, want empty", fresh.Room.State.BoardTiles["1A"].Belong)
	}
	if fresh.Room.State.Players["owner-1"].Money != 6000 {
		t.Fatalf("money = %d, want 6000", fresh.Room.State.Players["owner-1"].Money)
	}
	if fresh.Room.State.Players["owner-1"].Stocks["Tower"] != 1 {
		t.Fatalf("stock = %d, want 1", fresh.Room.State.Players["owner-1"].Stocks["Tower"])
	}
	if fresh.Room.State.Players["owner-1"].Tiles[0] != "1A" {
		t.Fatalf("tile = %q, want 1A", fresh.Room.State.Players["owner-1"].Tiles[0])
	}
	if fresh.Room.State.Companies["Tower"].StockTotal != 24 {
		t.Fatalf("stock total = %d, want 24", fresh.Room.State.Companies["Tower"].StockTotal)
	}
	if fresh.Room.State.MergeSettleData["Tower"].Hoders[0] != "owner-1" {
		t.Fatalf("holder = %q, want owner-1", fresh.Room.State.MergeSettleData["Tower"].Hoders[0])
	}
	if fresh.Room.State.MergeSettleData["Tower"].Dividends["owner-1"] != 1000 {
		t.Fatalf("dividend = %d, want 1000", fresh.Room.State.MergeSettleData["Tower"].Dividends["owner-1"])
	}
	if !fresh.Room.Connections["owner-1"].Online {
		t.Fatal("connection online = false, want true")
	}
}
