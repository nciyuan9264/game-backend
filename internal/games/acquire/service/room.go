package service

import (
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/roompkg"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/dto"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomctl"
)

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
	room_id := timePrefix

	newRoom := roompkg.NewRoomService(room_id, userID)

	roompkg.Rooms.Set(room_id, newRoom)

	go newRoom.Run()

	logger.Info("房间已创建", logger.F("room_id", room_id), logger.F("userID", userID))
	return room_id, nil
}

func GetRoomList() ([]dto.RoomInfo, error) {
	var rooms []dto.RoomInfo
	roomConnInfos := roompkg.Rooms.Snapshot()
	for roomID, roomConnInfo := range roomConnInfos {
		roomPlayers := make([]dto.RoomPlayer, 0, len(roomConnInfo.Room.Connections))
		for _, player := range roomConnInfo.Room.Connections {
			roomPlayers = append(roomPlayers, dto.RoomPlayer{
				PlayerID: player.PlayerID,
				Online:   player.Online,
				AI:       player.AI,
				Ready:    player.Ready,
			})
		}

		tiles := roomConnInfo.Room.State.BoardTiles
		emptyTileCount := 0
		for _, tile := range tiles {
			if tile.Belong == "" {
				emptyTileCount++
			}
		}

		room := dto.RoomInfo{
			RoomID:         roomID,
			OwnerID:        roomConnInfo.Room.State.OwnerID,
			Status:         roomConnInfo.Room.State.RoomStatus,
			RoomPlayer:     roomPlayers,
			EmptyTileCount: emptyTileCount,
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

func GetGameStatus(roomID string) *roompkg.RoomService {
	roomConnInfo, exists := roompkg.Rooms.Get(roomID)
	if !exists {
		logger.Error("房间不存在", logger.F("room_id", roomID))
	}

	return roomConnInfo
}
