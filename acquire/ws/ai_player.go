package ws

import (
	"go-game/domain/data"
	"go-game/domain/room"
	"go-game/dto"
	"log"
)

func JoinRoomAsAI(roomID, playerID string) bool {
	// room.RoomLock.Lock()
	// defer room.RoomLock.Unlock()

	// roomInfo, err := data.GetRoomInfo(repository.Rdb, roomID)
	// if err != nil {
	// 	log.Println("❌ 获取房间信息失败:", err)
	// 	return false
	// }

	// maxPlayers := roomInfo.MaxPlayers

	// // 判断房间人数是否已满
	// if len(room.Rooms[roomID].Players) >= maxPlayers {
	// 	log.Printf("房间 %s 已满，AI %s 无法加入\n", roomID, playerID)
	// 	return false
	// }

	data.InitPlayerData(room.Rooms[roomID], playerID)
	// 加入房间，虚拟连接
	// room.Rooms[roomID].Players = append(room.Rooms[roomID].Players, &dto.PlayerConn{
	// 	PlayerID: playerID,
	// 	Conn:     &VirtualConn{PlayerID: playerID, RoomID: roomID},
	// 	Online:   true,
	// 	Ready:    true,
	// 	AI:       true,
	// })

	log.Printf("AI 玩家 %s 加入房间 %s\n", playerID, roomID)
	return true
}

func JoinMatchAsAI(roomID, playerID string) bool {
	room.RoomLock.Lock()
	defer room.RoomLock.Unlock()

	room.Rooms[roomID].Players = append(room.Rooms[roomID].Players, &dto.PlayerConn{
		PlayerID: playerID,
		Conn:     &VirtualConn{PlayerID: playerID, RoomID: roomID},
		Online:   true,
		Ready:    true,
		AI:       true,
	})

	log.Printf("AI 玩家 %s 加入房间 %s\n", playerID, roomID)
	return true
}

func RemovePlayer(roomID, removePlayer string) bool {
	room.RoomLock.Lock()
	defer room.RoomLock.Unlock()

	players := room.Rooms[roomID].Players
	for i, p := range players {
		if p.PlayerID == removePlayer {
			// 移除玩家
			room.Rooms[roomID].Players = append(players[:i], players[i+1:]...)
			log.Printf("玩家 %s 从房间 %s 移除\n", removePlayer, roomID)
			return true
		}
	}
	log.Printf("房间 %s 中不存在玩家 %s\n", roomID, removePlayer)
	return false
}
