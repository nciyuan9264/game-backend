package service

import (
	"context"
	"fmt"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
	"github.com/nciyuan9264/game-backend/pkg/roomctl/dto"
	"github.com/nciyuan9264/game-backend/pkg/timeutil"
)

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
	room_id := timePrefix

	newRoom := roompkg.NewRoomService(room_id, userID)

	roompkg.Rooms.Set(room_id, newRoom)

	go newRoom.Run()

	logger.Info("房间已创建", logger.F("room_id", room_id), logger.F("userID", userID))
	return room_id, nil
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

	roomConnInfos := roompkg.Rooms.Snapshot()
	for roomID, roomConnInfo := range roomConnInfos {
		snapshot, err := roomConnInfo.Snapshot(ctx)
		if err != nil {
			return nil, fmt.Errorf("snapshot room %s: %w", roomID, err)
		}

		roomPlayers := make([]dto.RoomPlayer, 0, len(snapshot.Players))
		for _, player := range snapshot.Players {
			roomPlayers = append(roomPlayers, dto.RoomPlayer{
				PlayerID: player.PlayerID,
				Online:   player.Online,
				AI:       player.AI,
				Ready:    player.Ready,
			})
		}

		room := dto.RoomInfo{
			RoomID:         snapshot.RoomID,
			OwnerID:        snapshot.OwnerID,
			Status:         snapshot.Status,
			RoomPlayer:     roomPlayers,
			EmptyTileCount: snapshot.EmptyTileCount,
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

func GetGameStatus(roomID string) *roompkg.RoomStatusSnapshot {
	roomConnInfo, exists := roompkg.Rooms.Get(roomID)
	if !exists {
		logger.Error("房间不存在", logger.F("room_id", roomID))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), roomSnapshotTimeout)
	defer cancel()

	snapshot, err := roomConnInfo.StatusSnapshot(ctx)
	if err != nil {
		logger.Error("获取房间状态快照失败", logger.F("room_id", roomID), logger.F("error", err))
		return nil
	}

	return &snapshot
}
