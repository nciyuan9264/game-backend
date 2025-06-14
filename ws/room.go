package ws

import (
	"go-game/dto"
	"go-game/repository"
	"log"

	"github.com/gorilla/websocket"
)

// 校验房间是否有空位，并将玩家加入房间
func validateAndJoinRoom(roomID, playerID string, conn *websocket.Conn) bool {
	roomInfo, err := GetRoomInfo(repository.Rdb, roomID)
	if err != nil {
		log.Println("❌ 无法获取房间信息:", err)
		return false
	}
	maxPlayers := roomInfo.MaxPlayers

	// 查找玩家是否已经在房间中（包括掉线状态）
	for i, pc := range Rooms[roomID] {
		if pc.PlayerID == playerID {
			Rooms[roomID][i].Conn = conn
			Rooms[roomID][i].Online = true
			log.Printf("玩家 %s 重连成功\n", playerID)
			return true
		}
	}

	if len(Rooms[roomID]) >= maxPlayers {
		return false
	}

	// 添加新玩家
	Rooms[roomID] = append(Rooms[roomID], dto.PlayerConn{
		PlayerID: playerID,
		Conn:     conn,
		Online:   true,
	})
	log.Printf("玩家 %s 加入房间 %s\n", playerID, roomID)
	return true
}
