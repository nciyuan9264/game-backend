package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
	"github.com/nciyuan9264/game-backend/pkg/roomctl/dto"
	"github.com/nciyuan9264/game-backend/pkg/timeutil"
)

const roomSnapshotTimeout = time.Second

var createMu sync.Mutex
var ErrRoomNotFound = errors.New("room not found")
var ErrInvalidVisibility = errors.New("invalid visibility")

func CreateRoom(userID string) (string, error) {
	return CreateRoomWithSettings(userID, "public", "", "")
}

func CreateRoomWithSettings(userID, visibility, name, avatar string) (string, error) {
	if visibility != "public" && visibility != "private" {
		return "", ErrInvalidVisibility
	}
	// Keep the ownership limit and ID allocation atomic across concurrent creates.
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
	newRoom.Room.State.Visibility = visibility
	newRoom.Room.State.OwnerName = name
	newRoom.Room.State.OwnerAvatar = avatar

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
		if sameUserOrPlayer(snapshot.OwnerID, userID) {
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

		if snapshot.Visibility == "private" && !sameUserOrPlayer(snapshot.OwnerID, viewerID) {
			continue
		}
		rooms = append(rooms, roomInfo(snapshot))
	}

	return rooms, nil
}

func LookupRoom(ctx context.Context, roomID, viewerID string) (dto.RoomInfo, error) {
	rs, ok := roompkg.Rooms.Get(roomID)
	if !ok {
		return dto.RoomInfo{}, ErrRoomNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, roomSnapshotTimeout)
	defer cancel()
	snapshot, err := rs.Snapshot(ctx)
	if err != nil {
		return dto.RoomInfo{}, err
	}
	info := roomInfo(snapshot)
	for _, player := range info.RoomPlayer {
		if sameUserOrPlayer(player.PlayerID, viewerID) {
			info.IsMember = true
		}
	}
	return info, nil
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

func roomInfo(snapshot roompkg.RoomSnapshot) dto.RoomInfo {
	players := make([]dto.RoomPlayer, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, dto.RoomPlayer{
			PlayerID: player.PlayerID, Online: player.Online, AI: player.AI,
			Ready: player.Ready, SeatIndex: player.SeatIndex, Name: player.Name, Avatar: player.Avatar,
		})
	}
	return dto.RoomInfo{
		RoomID: snapshot.RoomID, OwnerID: snapshot.OwnerID, Status: snapshot.Status,
		RoomPlayer: players, EmptyTileCount: snapshot.EmptyTileCount,
		Visibility: snapshot.Visibility, MaxPlayers: snapshot.MaxPlayers,
		PlayerOrder: snapshot.PlayerOrder, OwnerName: snapshot.OwnerName, OwnerAvatar: snapshot.OwnerAvatar,
	}
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
