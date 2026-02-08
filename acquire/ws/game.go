package ws

import (
	"encoding/json"
	"go-game/domain/data"
	"go-game/domain/game"
	"go-game/domain/room"
	"go-game/dto"
	"go-game/repository"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func transferOwnerOrDelete(r *dto.Room) bool {
	for _, p := range r.Players {
		if !p.AI && p.Online {
			r.OwnerID = p.PlayerID
			return false
		}
	}

	// 没有在线真人了
	delete(room.Rooms, r.ID)
	return true
}

func MarkPlayerOffline(roomID, playerID string, conn interface{}) (roomDeleted bool) {
	room.RoomLock.Lock()
	defer room.RoomLock.Unlock()

	r, ok := room.Rooms[roomID]
	if !ok {
		return true // 房间都没了，当作已删除
	}

	var ownerLeft bool

	for _, p := range r.Players {
		if p.PlayerID == playerID && p.Conn == conn {
			p.Online = false
			p.Conn = nil
			// if p.PlayerID == r.OwnerID {
			// 	ownerLeft = true
			// }
			break
		}
	}

	if ownerLeft {
		return transferOwnerOrDelete(r)
	}

	return false
}
func handleOwnerLeave(r *dto.Room) {
	// 找第一个非 AI 玩家
	for _, p := range r.Players {
		if !p.AI {
			r.OwnerID = p.PlayerID
			return
		}
	}

	// 没有真人了 → 删除房间
	delete(room.Rooms, r.ID)
}

// 玩家断开连接后，从房间中移除该连接
func cleanupOnDisconnect(roomID, playerID string, conn *websocket.Conn) {
	// 1️⃣ 通知 room：这个人掉线了
	roomDeleted := MarkPlayerOffline(roomID, playerID, conn)
	currentRoom := room.Rooms[roomID]

	// 2️⃣ 同步 Redis 房间状态（如果房间还在）
	if !roomDeleted && currentRoom.Status != dto.RoomStatusMatch {
		roomInfo, err := data.GetRoomInfo(repository.Rdb, roomID)
		if err != nil {
			log.Println("❌ 获取房间信息失败:", err)
		} else if roomInfo.RoomStatus {
			data.SetRoomStatus(repository.Rdb, roomID, false)
		}
	}
	// 3️⃣ 广播最新状态
	game.BroadcastToRoom(roomID)
}

func listenAndBroadcastMessages(conn room.ReadWriteConn, roomID, playerID string) {
	ctx := &WSConn{
		Conn:     conn,
		RoomID:   roomID,
		PlayerID: playerID,
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("读取消息失败:", err)
			break
		}

		msgMap := make(map[string]interface{})
		msgMap["playerID"] = playerID

		if err := json.Unmarshal(msg, &msgMap); err != nil {
			log.Println("消息解析失败:", err)
			continue
		}

		Dispatch(ctx, msgMap)
	}
}

func HandleWebSocket(c *gin.Context) {
	conn, err := upgradeConnection(c)
	if err != nil {
		return
	}
	defer conn.Close()

	// 1️⃣ 解析参数
	roomID := c.Query("roomID")
	playerID := c.Query("userID")
	if roomID == "" || playerID == "" {
		log.Println("缺少 roomID 或 userID")
		return
	}

	// 2️⃣ 校验 + 加入房间（唯一一次写 Rooms 的地方）
	ok := room.ValidateAndJoinRoom(roomID, playerID, conn)
	if ok != nil {
		conn.WriteMessage(
			websocket.TextMessage,
			[]byte(`{"type":"error","message":"`+ok.Error()+`"}`),
		)
		return
	}

	if room.Rooms[roomID].Status == dto.RoomStatusMatch {
		game.BroadcastToMatch(roomID)
	} else {
		// 3️⃣ 初始广播（有人上线）
		game.BroadcastToRoom(roomID)
	}

	// 4️⃣ 离开时清理
	defer cleanupOnDisconnect(roomID, playerID, conn)

	// 5️⃣ 进入消息循环（真正的 WS 世界）
	listenAndBroadcastMessages(conn, roomID, playerID)
}
