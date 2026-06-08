package service

import (
	"fmt"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/roompkg"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
	"github.com/nciyuan9264/game-backend/pkg/roomctl/dto"
)

const defaultMaxPlayers = 4

func CreateRoom(userID string) (string, error) {
	owned := roomctl.CountOwnedRooms(
		roompkg.Rooms.Snapshot(),
		func(rs *roompkg.RoomService) string { return rs.Room.State.OwnerID },
		userID,
	)
	if owned >= roomctl.MaxOwnedRooms {
		logger.Warn("玩家房间数已达上限",
			logger.F("userID", userID),
			logger.F("owned", owned),
			logger.F("limit", roomctl.MaxOwnedRooms))
		return "", roomctl.ErrTooManyRooms
	}

	timePrefix := time.Now().Format("0102_150405")
	roomID := fmt.Sprintf("%s_%s", timePrefix, RandString(4))

	newRoom := roompkg.NewRoomService(roomID, userID, defaultMaxPlayers)
	roompkg.Rooms.Set(roomID, newRoom)
	go newRoom.Run()
	return roomID, nil
}

func DeleteRoom(params dto.DeleteRoomRequest) error {
	rs, ok := roompkg.Rooms.Get(params.RoomID)
	if !ok {
		return fmt.Errorf("房间不存在")
	}
	select {
	case <-rs.Room.QuitCh:
	default:
		close(rs.Room.QuitCh)
	}
	roompkg.Rooms.Delete(params.RoomID)
	return nil
}

func GetRoomList() ([]dto.RoomInfo, error) {
	var rooms []dto.RoomInfo
	for roomID, rs := range roompkg.Rooms.Snapshot() {
		players := make([]dto.RoomPlayer, 0, len(rs.Room.Connections))
		for _, p := range rs.Room.Connections {
			players = append(players, dto.RoomPlayer{
				PlayerID: p.PlayerID,
				Online:   p.Online,
				AI:       p.AI,
				Ready:    p.Ready,
			})
		}
		rooms = append(rooms, dto.RoomInfo{
			RoomID:     roomID,
			OwnerID:    rs.Room.State.OwnerID,
			MaxPlayers: rs.Room.State.MaxPlayers,
			Status:     string(rs.Room.State.RoomStatus),
			RoomPlayer: players,
		})
	}
	return rooms, nil
}

func GetGameStatus(roomID string) *roompkg.RoomService {
	rs, ok := roompkg.Rooms.Get(roomID)
	if !ok {
		return nil
	}
	return rs
}
