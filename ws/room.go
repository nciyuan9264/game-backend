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
	if rooms[roomID] == nil {
		rooms[roomID] = []dto.PlayerConn{}
	}

	// 查找玩家是否已经在房间中（包括掉线状态）
	for i, pc := range rooms[roomID] {
		if pc.PlayerID == playerID {
			// 处理断线重连
			if pc.Conn != nil && pc.Conn != conn {
				// 关闭旧连接（可选）
				// pc.Conn.Close()
			}
			rooms[roomID][i].Conn = conn
			rooms[roomID][i].Online = true
			log.Printf("玩家 %s 重连成功\n", playerID)
			return true
		}
	}

	// 如果房间人数已满，不允许新玩家加入（只计算有效连接）
	activeCount := 0
	for _, pc := range rooms[roomID] {
		if pc.Online {
			activeCount++
		}
	}
	if activeCount >= maxPlayers {
		return false
	}

	// 添加新玩家
	rooms[roomID] = append(rooms[roomID], dto.PlayerConn{
		PlayerID: playerID,
		Conn:     conn,
		Online:   true,
	})
	log.Printf("玩家 %s 加入房间 %s\n", playerID, roomID)
	return true
}
