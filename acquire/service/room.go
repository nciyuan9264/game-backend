package service

import (
	"go-game/domain/data"
	"go-game/domain/roompkg"
	"go-game/dto"
	"go-game/repository"
	"go-game/utils"
	"time"
)

func CreateRoom(userID string) (string, error) {
	timePrefix := time.Now().Format("0102_150405")
	roomID := timePrefix

	newRoom := roompkg.NewRoomService(roomID, userID)

	roompkg.RoomLock.Lock()
	roompkg.Rooms[roomID] = newRoom
	roompkg.RoomLock.Unlock()

	go newRoom.Run()

	utils.Info("房间已创建", utils.F("roomID", roomID))
	return roomID, nil
}

func GetRoomList() ([]dto.RoomInfo, error) {
	var rooms []dto.RoomInfo
	roomConnInfos := roompkg.GetAllRoomsSnapshot()
	for roomID, roomConnInfo := range roomConnInfos {
		roomPlayers := make([]dto.RoomPlayer, 0, len(roomConnInfo.Room.Players))
		for _, player := range roomConnInfo.Room.Players {
			roomPlayers = append(roomPlayers, dto.RoomPlayer{
				PlayerID: player.PlayerID,
				Online:   player.Online,
				AI:       player.AI,
				Ready:    player.Ready,
			})
		}
		tiles, err := data.GetAllRoomTiles(repository.Rdb, roomID)
		if err != nil {
			utils.Error("获取房间所有瓦片失败", utils.F("room_id", roomID), utils.F("error", err))
			continue
		}

		emptyTileCount := 0
		for _, tile := range tiles {
			if tile.Belong == "" {
				emptyTileCount++
			}
		}

		room := dto.RoomInfo{
			RoomID:         roomID,
			OwnerID:        roomConnInfo.Room.OwnerID,
			Status:         roomConnInfo.Room.Status,
			RoomPlayer:     roomPlayers,
			EmptyTileCount: emptyTileCount,
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}
