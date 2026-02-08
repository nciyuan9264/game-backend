package ws

import "go-game/domain/room"

type WSConn struct {
	Conn     room.ReadWriteConn
	RoomID   string
	PlayerID string
}
