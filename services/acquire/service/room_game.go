package service

import (
	"acquire-service/dto"
	"acquire-service/repository"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/exp/rand"
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

// 获取房间中玩家数量
func getRoomPlayerCount(roomID string) int {
	onLineCount := 0
	for _, pc := range Rooms[roomID] {
		if pc.Online {
			onLineCount++
		}
	}
	return onLineCount
}

func (gs *GameService) HandleReady(ctx context.Context, roomID, playerID string, data interface{}) (interface{}, error) {
	roomInfo, err := GetRoomInfo(gs.rdb, roomID)
	if err != nil {
		log.Println("❌ 无法获取房间信息:", err)
		return nil, err
	}
	maxPlayers := roomInfo.MaxPlayers
	InitPlayerData(roomID, playerID)
	// 获取房间当前人数
	playerCount := getRoomPlayerCount(roomID)
	log.Printf("玩家加入 room=%s，ID=%s，当前人数=%d/%d", roomID, playerID, playerCount, maxPlayers)

	if playerCount == maxPlayers {
		err := SetRoomStatus(gs.rdb, roomID, true)
		if err != nil {
			log.Println("❌ 设置房间状态失败:", err)
			return nil, err
		}

		startKey := fmt.Sprintf("room:%s:game_start_time", roomID)
		gs.rdb.Set(ctx, startKey, time.Now().Format("20060102_150405"), 0)

		currentPlayerID, err := GetCurrentPlayer(gs.rdb, ctx, roomID)
		if err != nil {
			log.Println("❌ 获取当前玩家失败:", err)
			return nil, err
		}
		if currentPlayerID == "" {
			randomPlayerID := Rooms[roomID][rand.Intn(maxPlayers)]
			err := SetCurrentPlayer(gs.rdb, ctx, roomID, randomPlayerID.PlayerID)
			if err != nil {
				log.Println("❌ 设置当前玩家失败:", err)
				return nil, err
			}
		}
	}
	return nil, nil
}
