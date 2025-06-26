package ws

import (
	"encoding/json"
	"go-game/entities"
	"go-game/repository"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

func handlePlayAudioMessage(conn ReadWriteConn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	currentPlayer, err := GetCurrentPlayer(rdb, repository.Ctx, roomID)
	if err != nil {
		log.Println("❌ 获取当前玩家失败:", err)
		return
	}
	if currentPlayer != playerID {
		log.Println("❌ 不是当前玩家的回合")
	}

	audioType, ok := msgMap["payload"].(string)
	if !ok {
		log.Println("❌ 消息格式错误")
		return
	}

	msg := map[string]interface{}{
		"type":    "audio",
		"message": audioType,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("❌ 编码 JSON 失败:", err)
		return
	}

	for _, pc := range Rooms[roomID] {
		if pc.Online && pc.Conn != nil {
			err := pc.Conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				log.Printf("❌ 向玩家 %s 发送音频消息失败: %v\n", pc.PlayerID, err)
			}
		}
	}
}

func handleRestartGameMessage(conn ReadWriteConn, rdb *redis.Client, roomID string, playerID string, msgMap map[string]interface{}) {
	// 重置上次操作
	if err := SetLastData(roomID, playerID, "", nil); err != nil {
		log.Println("❌ 设置最后放置的 tile 失败:", err)
		return
	}
	// 重置游戏状态
	SetGameStatus(rdb, roomID, entities.RoomStatusPlaying)

	for _, pc := range Rooms[roomID] {
		err := InitPlayerDataToRedis(roomID, pc.PlayerID)
		if err != nil {
			log.Println("❌ 初始化玩家数据失败:", err)
			return
		}
	}

	err := InitRoomData(roomID)
	if err != nil {
		log.Println("❌ 初始化房间数据失败:", err)
		return
	}

	time.Sleep(2 * time.Second)
	BroadcastToRoom(roomID)
}
