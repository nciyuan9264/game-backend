package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/roompkg"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
	"github.com/nciyuan9264/game-backend/pkg/roomctl/dto"
	"github.com/nciyuan9264/game-backend/pkg/timeutil"
)

const roomSnapshotTimeout = time.Second

var createMu sync.Mutex

func CreateRoom(userID string) (string, error) {
	return CreateRoomWithProfile(userID, "", "")
}

func CreateRoomWithProfile(userID, name, avatar string) (string, error) {
	createMu.Lock()
	defer createMu.Unlock()

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
	for suffix := 1; ; suffix++ {
		if _, exists := roompkg.Rooms.Get(room_id); !exists {
			break
		}
		room_id = fmt.Sprintf("%s_%d", timePrefix, suffix)
	}

	newRoom := roompkg.NewRoomService(room_id, userID)
	newRoom.Room.State.OwnerName = name
	newRoom.Room.State.OwnerAvatar = avatar

	roompkg.Rooms.Set(room_id, newRoom)

	go newRoom.Run()

	logger.Info("房间已创建", logger.F("room_id", room_id))
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

func GetRoomList(viewerID string) ([]dto.RoomInfo, error) {
	rooms := make([]dto.RoomInfo, 0)
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
				PlayerID:  player.PlayerID,
				Online:    player.Online,
				AI:        player.AI,
				Ready:     player.Ready,
				SeatIndex: player.SeatIndex,
				Name:      player.Name,
				Avatar:    player.Avatar,
			})
		}

		room := dto.RoomInfo{
			RoomID:         snapshot.RoomID,
			OwnerID:        snapshot.OwnerID,
			OwnerName:      snapshot.OwnerName,
			OwnerAvatar:    snapshot.OwnerAvatar,
			Status:         snapshot.Status,
			RoomPlayer:     roomPlayers,
			PlayerOrder:    snapshot.PlayerOrder,
			MaxPlayers:     snapshot.MaxPlayers,
			BoardCardCount: snapshot.BoardCardCount,
		}
		room.IsMember = sameUserOrPlayer(snapshot.OwnerID, viewerID)
		if !room.IsMember {
			for _, player := range snapshot.Players {
				if !sameUserOrPlayer(player.PlayerID, viewerID) {
					continue
				}
				room.IsMember = true
				break
			}
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

func sameUserOrPlayer(playerID, userID string) bool {
	if playerID == userID {
		return true
	}
	left, leftOK := numericUserID(playerID)
	right, rightOK := numericUserID(userID)
	return leftOK && rightOK && left == right
}

func numericUserID(value string) (int64, bool) {
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed, true
	}
	return gamehistory.ParseUserIDFromPlayerID(value)
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
