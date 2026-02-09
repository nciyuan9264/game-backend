package ws

import (
	"encoding/json"
	"go-game/domain/domain"
	"go-game/domain/roompkg"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// func transferOwnerOrDelete(r *roompkg.Room) bool {
// 	for _, p := range r.Players {
// 		if !p.AI && p.Online {
// 			r.OwnerID = p.PlayerID
// 			return false
// 		}
// 	}

// 	// 没有在线真人了
// 	delete(room.Rooms, r.ID)
// 	return true
// }

// func MarkPlayerOffline(roomID, playerID string, conn interface{}) (roomDeleted bool) {
// 	room.RoomLock.Lock()
// 	defer room.RoomLock.Unlock()

// 	r, ok := room.Rooms[roomID]
// 	if !ok {
// 		return true // 房间都没了，当作已删除
// 	}

// 	var ownerLeft bool

// 	for _, p := range r.Players {
// 		if p.PlayerID == playerID && p.Conn == conn {
// 			p.Online = false
// 			p.Conn = nil
// 			// if p.PlayerID == r.OwnerID {
// 			// 	ownerLeft = true
// 			// }
// 			break
// 		}
// 	}

// 	if ownerLeft {
// 		return transferOwnerOrDelete(r)
// 	}

// 	return false
// }

// func handleOwnerLeave(r *roompkg.Room) {
// 	// 找第一个非 AI 玩家
// 	for _, p := range r.Players {
// 		if !p.AI {
// 			r.OwnerID = p.PlayerID
// 			return
// 		}
// 	}

// 	// 没有真人了 → 删除房间
// 	delete(room.Rooms, r.ID)
// }

// 玩家断开连接后，从房间中移除该连接
// func cleanupOnDisconnect(roomID, playerID string, conn *websocket.Conn) {
// 	// 1️⃣ 通知 room：这个人掉线了
// 	roomDeleted := MarkPlayerOffline(roomID, playerID, conn)
// 	currentRoom := room.Rooms[roomID]

// 	// 2️⃣ 同步 Redis 房间状态（如果房间还在）
// 	if !roomDeleted && currentRoom.Status != dto.RoomStatusMatch {
// 		roomInfo, err := data.GetRoomInfo(repository.Rdb, roomID)
// 		if err != nil {
// 			log.Println("❌ 获取房间信息失败:", err)
// 		} else if roomInfo.RoomStatus {
// 			data.SetRoomStatus(repository.Rdb, roomID, false)
// 		}
// 	}
// 	// 3️⃣ 广播最新状态
// 	game.BroadcastToRoom(roomID)
// }

// func listenAndBroadcastMessages(conn roompkg.ReadWriteConn, roomID, playerID string) {
// 	ctx := &WSConn{
// 		Conn:     conn,
// 		RoomID:   roomID,
// 		PlayerID: playerID,
// 	}

// 	for {
// 		_, msg, err := conn.ReadMessage()
// 		if err != nil {
// 			log.Println("读取消息失败:", err)
// 			break
// 		}

// 		msgMap := make(map[string]interface{})
// 		msgMap["playerID"] = playerID

// 		if err := json.Unmarshal(msg, &msgMap); err != nil {
// 			log.Println("消息解析失败:", err)
// 			continue
// 		}

// 		Dispatch(ctx, msgMap)
// 	}
// }

func readLoop(conn *websocket.Conn, room *domain.Room, playerID string) {
	defer func() {
		// 断线也是 Command
		room.CmdCh <- domain.Command{
			PlayerID: playerID,
			Type:     "disconnect",
		}
		conn.Close()
	}()

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}

		// WS → Command
		room.CmdCh <- domain.Command{
			Type:     msg.Type,
			PlayerID: playerID,
			Payload:  msg.Payload,
			Conn:     conn,
		}
	}
}

func HandleWebSocket(c *gin.Context) {
	conn, err := upgradeConnection(c)
	if err != nil {
		return
	}

	// 1️⃣ 解析参数（保持不变）
	roomID := c.Query("roomID")
	playerID := c.Query("userID")
	if roomID == "" || playerID == "" {
		conn.Close()
		return
	}

	// 2️⃣ 拿房间（只读，不改）
	room := roompkg.Rooms[roomID]
	if room == nil {
		conn.WriteJSON(map[string]string{
			"type":    "error",
			"message": "ROOM_NOT_FOUND",
		})
		conn.Close()
		return
	}

	// 3️⃣ 通知房间：有人 join（Command）
	room.Room.CmdCh <- domain.Command{
		Type:     "connect",
		PlayerID: playerID,
		Conn:     conn,
	}

	// 4️⃣ 启动 WS 读循环（每个连接一个 goroutine）
	go readLoop(conn, room.Room, playerID)
}
