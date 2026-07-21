package roompkg

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

// ErrRoomSnapshotUnavailable indicates the room actor cannot serve a snapshot.
var ErrRoomSnapshotUnavailable = errors.New("room snapshot unavailable")

// RoomSnapshotPlayer is the copy-safe player view returned by a room actor.
type RoomSnapshotPlayer struct {
	PlayerID string
	Online   bool
	AI       bool
	Ready    bool
}

// RoomSnapshot is the copy-safe list view returned by a room actor.
type RoomSnapshot struct {
	RoomID         string
	OwnerID        string
	Status         string
	Players        []RoomSnapshotPlayer
	EmptyTileCount int
}

// RoomStatusSnapshot is a copy-safe full room status view for /room/game_status.
type RoomStatusSnapshot struct {
	Room             *domain.Room `json:"Room"`
	HistorySeq       int          `json:"HistorySeq"`
	HistoryStartedAt time.Time    `json:"HistoryStartedAt"`
	HistoryEnded     bool         `json:"HistoryEnded"`
}

type snapshotRequest struct {
	reply chan RoomSnapshot
}

type statusSnapshotRequest struct {
	reply chan RoomStatusSnapshot
}

// Snapshot asks the room actor to build a copy-safe room list view.
func (r *RoomService) Snapshot(ctx context.Context) (RoomSnapshot, error) {
	if r == nil || r.Room == nil || r.snapshotCh == nil {
		return RoomSnapshot{}, ErrRoomSnapshotUnavailable
	}

	reply := make(chan RoomSnapshot, 1)
	req := snapshotRequest{reply: reply}

	select {
	case r.snapshotCh <- req:
	case <-r.Room.QuitCh:
		return RoomSnapshot{}, ErrRoomSnapshotUnavailable
	case <-ctx.Done():
		return RoomSnapshot{}, ctx.Err()
	}

	select {
	case snapshot := <-reply:
		return snapshot, nil
	case <-r.Room.QuitCh:
		return RoomSnapshot{}, ErrRoomSnapshotUnavailable
	case <-ctx.Done():
		return RoomSnapshot{}, ctx.Err()
	}
}

// StatusSnapshot asks the room actor to build a copy-safe full room status view.
func (r *RoomService) StatusSnapshot(ctx context.Context) (RoomStatusSnapshot, error) {
	if r == nil || r.Room == nil || r.statusSnapshotCh == nil {
		return RoomStatusSnapshot{}, ErrRoomSnapshotUnavailable
	}

	reply := make(chan RoomStatusSnapshot, 1)
	req := statusSnapshotRequest{reply: reply}

	select {
	case r.statusSnapshotCh <- req:
	case <-r.Room.QuitCh:
		return RoomStatusSnapshot{}, ErrRoomSnapshotUnavailable
	case <-ctx.Done():
		return RoomStatusSnapshot{}, ctx.Err()
	}

	select {
	case snapshot := <-reply:
		return snapshot, nil
	case <-r.Room.QuitCh:
		return RoomStatusSnapshot{}, ErrRoomSnapshotUnavailable
	case <-ctx.Done():
		return RoomStatusSnapshot{}, ctx.Err()
	}
}

func (r *RoomService) buildSnapshot() RoomSnapshot {
	room := r.Room
	snapshot := RoomSnapshot{
		RoomID:  room.ID,
		Players: make([]RoomSnapshotPlayer, 0, len(room.Connections)),
	}

	if room.State != nil {
		snapshot.OwnerID = room.State.OwnerID
		snapshot.Status = string(room.State.RoomStatus)
		snapshot.EmptyTileCount = countEmptyTiles(room.State.BoardTiles)
	}

	for _, player := range room.Connections {
		if player == nil {
			continue
		}
		snapshot.Players = append(snapshot.Players, RoomSnapshotPlayer{
			PlayerID: player.PlayerID,
			Online:   player.Online,
			AI:       player.AI,
			Ready:    player.Ready,
		})
	}
	sort.Slice(snapshot.Players, func(i, j int) bool {
		return snapshot.Players[i].PlayerID < snapshot.Players[j].PlayerID
	})

	return snapshot
}

func (r *RoomService) buildStatusSnapshot() RoomStatusSnapshot {
	return RoomStatusSnapshot{
		Room:             cloneRoom(r.Room),
		HistorySeq:       r.HistorySeq,
		HistoryStartedAt: r.HistoryStartedAt,
		HistoryEnded:     r.HistoryEnded,
	}
}

func countEmptyTiles(tiles map[string]*domain.Tile) int {
	emptyTileCount := 0
	for _, tile := range tiles {
		if tile != nil && tile.Belong == "" {
			emptyTileCount++
		}
	}
	return emptyTileCount
}

func cloneRoom(room *domain.Room) *domain.Room {
	if room == nil {
		return nil
	}
	return &domain.Room{
		Base:  cloneBase(room.Base),
		State: cloneGameState(room.State),
	}
}

func cloneBase(base *roomcore.Base) *roomcore.Base {
	if base == nil {
		return nil
	}

	connections := make(map[string]*roomcore.PlayerConn, len(base.Connections))
	for playerID, pc := range base.Connections {
		if pc == nil {
			continue
		}
		connections[playerID] = &roomcore.PlayerConn{
			PlayerID: pc.PlayerID,
			Online:   pc.Online,
			Ready:    pc.Ready,
			AI:       pc.AI,
		}
	}

	return &roomcore.Base{
		ID:            base.ID,
		Connections:   connections,
		PlayerSeq:     append([]string(nil), base.PlayerSeq...),
		NoHumanChecks: base.NoHumanChecks,
		AIRunning:     base.AIRunning,
		TurnDeadline:  base.TurnDeadline,
		TurnTimeout:   base.TurnTimeout,
	}
}
