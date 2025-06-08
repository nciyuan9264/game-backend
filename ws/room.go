package ws

import (
	"go-game/repository"
	"log"
	"strconv"

	"github.com/gorilla/websocket"
)

// 校验房间是否有空位，并将玩家加入房间
func validateAndJoinRoom(roomID, playerID string, conn *websocket.Conn) (bool, int) {
	roomInfo, err := GetRoomInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 无法获取房间信息:", err)
		return false, -1
	}

	maxPlayersStr := roomInfo["maxPlayers"]
	maxPlayers, err := strconv.Atoi(maxPlayersStr)
	if err != nil {
		log.Println("❌ maxPlayers 解析失败:", err)
		return false, -1
	}

	roomLock.Lock()
	defer roomLock.Unlock()

	// 如果已满
	if len(rooms[roomID]) >= maxPlayers {
		return false, maxPlayers
	}

	// 加入房间
	rooms[roomID] = append(rooms[roomID], PlayerConn{PlayerID: playerID, Conn: conn})
	return true, maxPlayers
}
