package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/dto"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/repository"
	"github.com/nciyuan9264/game-backend/pkg/logger"

	"github.com/gorilla/websocket"
)

func getGameLogFilePath(roomID string) string {
	// 建议你在房间初始化时设置一个 startTime 或 gameID
	// 这里假设你用启动时间生成文件名
	startKey := fmt.Sprintf("room:%s:game_start_time", roomID)
	startTimeStr, err := repository.Rdb.Get(repository.Ctx, startKey).Result()
	if err != nil {
		startTimeStr = time.Now().Format("20060102_150405") // fallback
		repository.Rdb.Set(repository.Ctx, startKey, time.Now().Format("20060102_150405"), 0)
	}
	fileName := fmt.Sprintf("%s_%s.json", roomID, startTimeStr)
	return path.Join("./game_logs", fileName)
}

func WriteGameLog(roomID, playerID string, data []byte) {
	go func() {
		logPath := getGameLogFilePath(roomID)

		// 确保目录存在
		if err := os.MkdirAll(path.Dir(logPath), 0755); err != nil {
			logger.Error("创建日志目录失败", logger.F("room_id", roomID), logger.F("log_path", logPath), logger.F("error", err))
			return
		}

		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logger.Error("打开游戏日志文件失败", logger.F("room_id", roomID), logger.F("log_path", logPath), logger.F("error", err))
			return
		}
		defer f.Close()

		data = append(data, ',')

		if _, err := f.Write(data); err != nil {
			logger.Error("写入日志失败", logger.F("room_id", roomID), logger.F("log_path", logPath), logger.F("error", err))
			return
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			logger.Error("写入换行失败", logger.F("room_id", roomID), logger.F("log_path", logPath), logger.F("error", err))
		}
	}()
}

// 向该客户端发送同步消息
func SyncRoomMessage(conn domain.WriteOnlyConn, room *domain.Room, pc *domain.PlayerConn) error {
	playersInfo := make(map[string]interface{})
	for _, p := range room.Connections {
		playersInfo[p.PlayerID] = map[string]interface{}{
			"playerID": p.PlayerID,
			"online":   p.Online,
			"ai":       p.AI,
			"ready":    p.Ready,
		}
	}
	logger.Info("SyncRoomMessage", logger.F("room_id", room.ID), logger.F("player_id", pc.PlayerID))
	boardCards := make(map[string]map[string]interface{})
	if room.State != nil && room.State.BoardCards != nil {
		for id, c := range room.State.BoardCards {
			if c == nil {
				continue
			}
			boardCards[id] = map[string]interface{}{
				"id":    c.ID,
				"color": c.Color,
			}
		}
	}

	selfCards := make([]map[string]interface{}, 0)
	opponents := make([]map[string]interface{}, 0)
	if room.State != nil && room.State.Players != nil {
		// 自己的牌：全量信息
		if me, ok := room.State.Players[pc.PlayerID]; ok && me != nil {
			for _, c := range me.Cards {
				if c == nil {
					continue
				}
				selfCards = append(selfCards, map[string]interface{}{
					"id":         c.ID,
					"color":      c.Color,
					"num":        c.Num,
					"isRevealed": c.IsRevealed,
					"index":      c.Index,
				})
			}
		}
		// 对手的牌：未翻面只给 id/颜色/index，翻面再给数字
		for pid, ps := range room.State.Players {
			if pid == pc.PlayerID || ps == nil {
				continue
			}
			cardList := make([]map[string]interface{}, 0, len(ps.Cards))
			for _, c := range ps.Cards {
				if c == nil {
					continue
				}
				logger.Info("对手牌", logger.F("room_id", room.ID), logger.F("player_id", pid), logger.F("card", c))
				item := map[string]interface{}{
					"id":    c.ID,
					"color": c.Color,
					"index": c.Index,
				}
				if c.IsRevealed {
					item["num"] = c.Num
				}
				cardList = append(cardList, item)
			}
			opponents = append(opponents, cardList...)
		}
	}

	var turnDeadline interface{}
	if !room.Base.TurnDeadline.IsZero() {
		turnDeadline = room.Base.TurnDeadline.UTC().Format(time.RFC3339Nano)
	}

	msg := map[string]interface{}{
		"type":     "ROOM_SYNC",
		"playerId": pc.PlayerID,
		"ownerID":  room.State.OwnerID,
		"roomData": map[string]interface{}{
			"currentPlayer": room.State.CurrentPlayer,
			"gameStatus":    room.State.RoomStatus,
			"players":       playersInfo,
			"boardCards":    boardCards,
			"lastData":      room.State.LastData,
			"turnDeadline":  turnDeadline,
			"turnTimeoutMs": int64(room.Base.TurnTimeout / time.Millisecond),
		},
		"cardData": map[string]interface{}{
			"self":      selfCards,
			"opponents": opponents,
		},
	}

	// ------- 发送 WebSocket 消息 -------
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("❌ 编码 JSON 失败: %w", err)
	}
	if pc.PlayerID == room.State.CurrentPlayer {
		WriteGameLog(room.ID, pc.PlayerID, data)
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

func SyncMatchMessage(conn domain.WriteOnlyConn, r *domain.Room, pc *domain.PlayerConn) error {
	playersInfo := make(map[string]dto.RoomPlayer, 0)
	for _, p := range r.Connections {
		playersInfo[p.PlayerID] = dto.RoomPlayer{
			PlayerID: p.PlayerID,
			Online:   p.Online,
			AI:       p.AI,
			Ready:    p.Ready,
		}
	}

	msg := dto.WsMatchSyncData{
		Type:     "MATCH_SYNC",
		RoomID:   r.ID,
		OwnerID:  r.State.OwnerID,
		Status:   r.State.RoomStatus,
		PlayerID: pc.PlayerID,
		Players:  playersInfo,
	}

	// ------- JSON 编码 -------
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("❌ 编码 JSON 失败: %w", err)
	}

	// ------- 只给当前玩家写日志（如果你有这个需求） -------
	// if playerID == roomInfo.OwnerID {
	// 	WriteGameLog(roomID, playerID, roomInfo, msg)
	// }

	// ------- 发送 WS -------
	return conn.WriteMessage(websocket.TextMessage, data)
}

// 广播消息给房间内所有连接成功的玩家
func BroadcastToRoom(r *domain.Room) {
	for _, pc := range r.Connections {
		if pc.Online {
			if err := SyncRoomMessage(pc.Conn, r, pc); err != nil {
				logger.Error("广播失败，移除连接", logger.F("room_id", r.ID), logger.F("player_id", pc.PlayerID), logger.F("error", err))
				pc.Conn.Close()
			}
		}
	}
}

func BroadcastToMatch(r *domain.Room) {
	for _, pc := range r.Connections {
		if !pc.Online {
			continue
		}

		if err := SyncMatchMessage(
			pc.Conn,
			r,
			pc,
		); err != nil {
			logger.Error("广播失败，关闭连接", logger.F("room_id", r.ID), logger.F("player_id", pc.PlayerID), logger.F("error", err))
			pc.Conn.Close()
			pc.Online = false
		}
	}
}
