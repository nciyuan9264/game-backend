package roompkg

import (
	"go-game/domain/domain"
)

func NewRoomService(roomID, ownerID string) *RoomService {
	r := &domain.Room{
		ID:      roomID,
		OwnerID: ownerID,
		Status:  domain.RoomStatusMatch,
		Players: make(map[string]*domain.PlayerConn),
		CmdCh:   make(chan domain.Command, 128),
		QuitCh:  make(chan struct{}),
	}

	return &RoomService{
		Room: r,
	}
}
