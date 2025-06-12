package ws

import (
	"go-game/dto"
	"go-game/repository"
	"log"

	"github.com/gorilla/websocket"
)

// 校验房间是否有空位，并将玩家加入房间
func validateAndJoinRoom(roomID, playerID string, conn *websocket.Conn) (bool, int) {
	roomInfo, err := GetRoomInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 无法获取房间信息:", err)
		return false, -1
	}
	maxPlayers := roomInfo.MaxPlayers
	roomLock.Lock()
	defer roomLock.Unlock()

	if len(rooms[roomID]) >= maxPlayers {
		return false, maxPlayers
	}

	rooms[roomID] = append(rooms[roomID], dto.PlayerConn{PlayerID: playerID, Conn: conn})
	return true, maxPlayers
}
