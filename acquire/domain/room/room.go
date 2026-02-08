package room

import (
	"fmt"

	"go-game/domain/data"
	"go-game/dto"
	"go-game/repository"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"golang.org/x/exp/rand"
)

const MaxPlayers = 6

var Rooms = map[string]*dto.Room{}
var RoomLock sync.Mutex

// 持续监听客户端消息，并将其广播给房间内其他玩家
type WriteOnlyConn interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// 读写接口，供真实客户端连接用，支持读取消息
type ReadWriteConn interface {
	WriteOnlyConn
	ReadMessage() (messageType int, p []byte, err error)
}

// 校验房间是否有空位，并将玩家加入房间
func ValidateAndJoinRoom(roomID, playerID string, conn *websocket.Conn) error {
	RoomLock.Lock()
	defer RoomLock.Unlock()

	room, ok := Rooms[roomID]
	if !ok {
		return fmt.Errorf("room not found")
	}

	// 查找玩家是否已经在房间中（包括掉线状态）
	for i, pc := range room.Players {
		if pc.PlayerID == playerID {
			if !pc.Online {
				room.Players[i].Conn = conn
				room.Players[i].Online = true
				log.Printf("玩家 %s 重连成功\n", playerID)
				return nil
			} else {
				log.Printf("玩家 %s 已在房间中\n", playerID)
				return fmt.Errorf("player already in room")
			}
		} else {
			if room.Status != dto.RoomStatusMatch {
				return fmt.Errorf("room status not match")
			}
		}
	}

	if len(room.Players) >= MaxPlayers {
		return fmt.Errorf("room full")
	}

	// 添加新玩家
	room.Players = append(room.Players, &dto.PlayerConn{
		PlayerID: playerID,
		Conn:     conn,
		Online:   true,
		Ready:    room.OwnerID == playerID,
		AI:       false,
	})

	log.Printf("玩家 %s 加入房间 %s\n", playerID, roomID)
	return nil
}

// 获取房间中玩家数量
func getRoomPlayerCount(room *dto.Room) int {
	onLineCount := 0
	for _, pc := range room.Players {
		if pc.Online {
			onLineCount++
		}
	}
	return onLineCount
}

func HandleReadyMessage(conn ReadWriteConn, rdb *redis.Client, room *dto.Room, playerID string, msgMap map[string]interface{}) {
	roomInfo, err := data.GetRoomInfo(repository.Rdb, room.ID)
	if err != nil {
		log.Println("❌ 无法获取房间信息:", err)
		return
	}
	maxPlayers := roomInfo.MaxPlayers
	data.InitPlayerData(room, playerID)
	// 获取房间当前人数
	playerCount := getRoomPlayerCount(room)
	log.Printf("玩家加入 room=%s，ID=%s，当前人数=%d/%d", room.ID, playerID, playerCount, maxPlayers)

	if playerCount == maxPlayers {
		err := data.SetRoomStatus(repository.Rdb, room.ID, true)
		if err != nil {
			log.Println("❌ 设置房间状态失败:", err)
			return
		}

		startKey := fmt.Sprintf("room:%s:game_start_time", room.ID)
		repository.Rdb.Set(repository.Ctx, startKey, time.Now().Format("20060102_150405"), 0)

		playerID, err := data.GetCurrentPlayer(repository.Rdb, repository.Ctx, room.ID)
		if err != nil {
			log.Println("❌ 获取当前玩家失败:", err)
			return
		}
		if playerID == "" {
			randomPlayerID := room.Players[rand.Intn(maxPlayers)]
			err := data.SetCurrentPlayer(repository.Rdb, repository.Ctx, room.ID, randomPlayerID.PlayerID)
			if err != nil {
				log.Println("❌ 设置当前玩家失败:", err)
				return
			}
		}
		// 更新房间状态为匹配中
		err = SetRoomStatusCache(room.ID, dto.RoomStatusSetTile)
		if err != nil {
			log.Printf("❌ 内存设置房间状态失败: %v\n", err)
			return
		}
		err = data.SetGameStatus(rdb, room.ID, dto.RoomStatusSetTile)
		if err != nil {
			log.Printf("❌ redis设置游戏状态失败: %v\n", err)
			return
		}
	}
}

func HandlePlayerReady(conn ReadWriteConn, rdb *redis.Client, currentRoom *dto.Room, playerID string, msgMap map[string]interface{}) {
	ready, ok := msgMap["payload"].(bool)
	if !ok {
		log.Println("无效的 payload")
		return
	}

	// 更新玩家准备状态
	for _, pc := range currentRoom.Players {
		if pc.PlayerID == playerID {
			pc.Ready = ready
			break
		}
	}
}

func SetRoomStatusCache(roomID string, status dto.RoomStatus) error {
	// 1️⃣ 更新内存状态（权威）
	r, ok := Rooms[roomID]
	if !ok {
		return fmt.Errorf("room not found in memory: %s", roomID)
	}
	r.Status = status
	return nil
}
