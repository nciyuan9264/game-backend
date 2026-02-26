package service

import (
	"go-game/domain/roompkg"
	"go-game/dto"
	"go-game/utils"
	"time"
)

func CreateRoom(userID string) (string, error) {
	timePrefix := time.Now().Format("0102_150405")
	room_id := timePrefix

	newRoom := roompkg.NewRoomService(room_id, userID)

	roompkg.RoomLock.Lock()
	roompkg.Rooms[room_id] = newRoom
	roompkg.RoomLock.Unlock()

	go newRoom.Run()

	utils.Info("房间已创建", utils.F("room_id", room_id))
	return room_id, nil
}

func GetRoomList() ([]dto.RoomInfo, error) {
	var rooms []dto.RoomInfo
	roomConnInfos := roompkg.GetAllRoomsSnapshot()
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
	roomConnInfo, exists := roompkg.Rooms[roomID]
	if !exists {
		utils.Error("房间不存在", utils.F("room_id", roomID))
	}

	return roomConnInfo
}
