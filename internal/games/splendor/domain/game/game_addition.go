package game

import (
	"encoding/json"
	"log"

	"github.com/gorilla/websocket"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/data"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
)

func HandlePlayAudioMessage(r *domain.Room, cmd domain.Command) {
	var audioType string
	if err := json.Unmarshal(cmd.Payload, &audioType); err != nil {
		log.Printf("❌ play_audio payload 解析失败 room=%s err=%v", r.ID, err)
		return
	}
	msg := map[string]interface{}{
		"type":    "audio",
		"message": audioType,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	for _, pc := range r.Connections {
		if pc.Online && pc.Conn != nil {
			if err := pc.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("❌ 发送音频消息失败 player=%s err=%v", pc.PlayerID, err)
			}
		}
	}
}

func HandleRestartGameMessage(r *domain.Room, cmd domain.Command) {
	r.State.LastData = nil
	r.State.RoomStatus = domain.RoomStatusPlaying

	for playerID := range r.Connections {
		data.InitPlayerData(r, playerID)
	}
	InitRoomData(r)
}
