package roompkg

import (
	"go-game/domain/domain"
	"time"
)

func NewRoomService(roomID, ownerID string) *RoomService {
	r := &domain.Room{
		ID:          roomID,
		Connections: make(map[string]*domain.PlayerConn),
		PlayerSeq:   []string{},
		State: &domain.GameState{
			GameStartTime: time.Time{},
			LastTileKey:   "",
			RoomStatus:    domain.RoomStatusMatch,
			OwnerID:       ownerID,
			MaxPlayers:    6,
			CurrentPlayer: "",

			BoardTiles: make(map[string]*domain.Tile),
			Players:    make(map[string]*domain.PlayerState),
			Companies:  make(map[string]*domain.CompanyState),
		},
		CmdCh:       make(chan domain.Command, 128),
		QuitCh:      make(chan struct{}),
		DeleteTimer: nil,
	}

	return &RoomService{
		Room: r,
	}
}
