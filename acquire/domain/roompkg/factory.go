package roompkg

import (
	"go-game/domain/domain"
	"go-game/dto"
)

func NewRoomService(roomID, ownerID string) *RoomService {
	r := &domain.Room{
		ID:      roomID,
		OwnerID: ownerID,
		Status:  dto.RoomStatusMatch,
		Players: make(map[string]*dto.PlayerConn),
		CmdCh:   make(chan domain.Command, 128),
		QuitCh:  make(chan struct{}),
	}

	return &RoomService{
		Room: r,
	}
}
