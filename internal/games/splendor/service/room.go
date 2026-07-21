package service

import (
	"context"
	"fmt"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/roompkg"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
	"github.com/nciyuan9264/game-backend/pkg/roomctl/dto"
	"github.com/nciyuan9264/game-backend/pkg/timeutil"
)

const defaultMaxPlayers = 4
const roomSnapshotTimeout = time.Second

func CreateRoom(userID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), roomSnapshotTimeout)
	defer cancel()

	owned, err := countOwnedRooms(ctx, userID)
	if err != nil {
		return "", err
	}
	if owned >= roomctl.MaxOwnedRooms {
		logger.Warn("玩家房间数已达上限",
			logger.F("userID", userID),
			logger.F("owned", owned),
			logger.F("limit", roomctl.MaxOwnedRooms))
		return "", roomctl.ErrTooManyRooms
	}

	timePrefix := timeutil.Now().Format("0102_150405")
	roomID := fmt.Sprintf("%s_%s", timePrefix, RandString(4))

	newRoom := roompkg.NewRoomService(roomID, userID, defaultMaxPlayers)
	roompkg.Rooms.Set(roomID, newRoom)
	go newRoom.Run()
	return roomID, nil
}

func countOwnedRooms(ctx context.Context, userID string) (int, error) {
	owned := 0
	for roomID, rs := range roompkg.Rooms.Snapshot() {
		snapshot, err := rs.Snapshot(ctx)
		if err != nil {
			return 0, fmt.Errorf("snapshot room %s: %w", roomID, err)
		}
		if snapshot.OwnerID == userID {
			owned++
		}
	}
	return owned, nil
}

func GetRoomList() ([]dto.RoomInfo, error) {
	var rooms []dto.RoomInfo
	ctx, cancel := context.WithTimeout(context.Background(), roomSnapshotTimeout)
	defer cancel()

	for roomID, rs := range roompkg.Rooms.Snapshot() {
		snapshot, err := rs.Snapshot(ctx)
		if err != nil {
			return nil, fmt.Errorf("snapshot room %s: %w", roomID, err)
		}

		players := make([]dto.RoomPlayer, 0, len(snapshot.Players))
		for _, p := range snapshot.Players {
			players = append(players, dto.RoomPlayer{
				PlayerID: p.PlayerID,
				Online:   p.Online,
				AI:       p.AI,
				Ready:    p.Ready,
			})
		}
		rooms = append(rooms, dto.RoomInfo{
			RoomID:     snapshot.RoomID,
			OwnerID:    snapshot.OwnerID,
			MaxPlayers: snapshot.MaxPlayers,
			Status:     snapshot.Status,
			RoomPlayer: players,
			MaxScore:   snapshot.MaxScore,
		})
	}
	return rooms, nil
}

func GetGameStatus(roomID string) *roompkg.RoomStatusSnapshot {
	rs, ok := roompkg.Rooms.Get(roomID)
	if !ok {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), roomSnapshotTimeout)
	defer cancel()

	snapshot, err := rs.StatusSnapshot(ctx)
	if err != nil {
		logger.Error("获取房间状态快照失败", logger.F("room_id", roomID), logger.F("error", err))
		return nil
	}
	return &snapshot
}
