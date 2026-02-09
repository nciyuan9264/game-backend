package ws

// import (
// 	"log"

// 	"go-game/domain/game"
// 	"go-game/domain/room"
// 	"go-game/dto"
// 	"go-game/repository"
// )

// func Dispatch(ctx *WSConn, msg map[string]interface{}) {
// 	msgType, ok := msg["type"].(string)
// 	if !ok {
// 		return
// 	}

// 	r := room.Rooms[ctx.RoomID]

// 	// ---------- 1. 根据【执行前状态】选择 handler ----------
// 	var handlers map[string]GameHandler

// 	switch r.Status {
// 	case dto.RoomStatusMatch:
// 		handlers = roomHandlers
// 	default:
// 		handlers = GameHandlers
// 		log.Printf("❌ 未知房间状态: %v", r.Status)
// 	}

// 	handler, ok := handlers[msgType]
// 	if !ok {
// 		log.Printf("❌ 当前阶段不支持消息: %s, status=%v", msgType, r.Status)
// 		return
// 	}

// 	// ---------- 2. 执行业务逻辑（这里可能会修改 r.Status） ----------
// 	handler(
// 		ctx.Conn,
// 		repository.Rdb,
// 		r,
// 		ctx.PlayerID,
// 		msg,
// 	)

// 	// ---------- 3. 根据【执行后状态】选择 broadcast ----------
// 	switch r.Status {
// 	case dto.RoomStatusMatch:
// 		game.BroadcastToMatch(ctx.RoomID)
// 	default:
// 		game.BroadcastToRoom(ctx.RoomID)
// 		log.Printf("⚠️ handler 执行后出现未知房间状态: %v", r.Status)
// 	}
// }
