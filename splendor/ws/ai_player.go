package ws

import (
	"go-game/dto"
	"log"
)

func JoinRoomAsAI(roomID, playerID string) bool {
	roomLock.Lock()
	defer roomLock.Unlock()

	roomInfo, err := GetRoomInfo(roomID)
	if err != nil {
		log.Println("❌ 获取房间信息失败:", err)
		return false
	}

	maxPlayers := roomInfo.MaxPlayers

	// 判断房间人数是否已满
	if len(Rooms[roomID]) >= maxPlayers {
		log.Printf("房间 %s 已满，AI %s 无法加入\n", roomID, playerID)
		return false
	}

	InitPlayerData(roomID, playerID)
	// 加入房间，虚拟连接
	Rooms[roomID] = append(Rooms[roomID], dto.PlayerConn{
		PlayerID: playerID,
		Conn:     &VirtualConn{PlayerID: playerID, RoomID: roomID},
		Online:   true,
	})

	log.Printf("AI 玩家 %s 加入房间 %s\n", playerID, roomID)
	return true
}
