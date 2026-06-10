package game

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/gorilla/websocket"

	"github.com/nciyuan9264/game-backend/internal/games/splendor/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/dto"
	"github.com/nciyuan9264/game-backend/internal/games/splendor/entities"
	"github.com/nciyuan9264/game-backend/pkg/timeutil"
)

func getGameLogFilePath(r *domain.Room) string {
	startTimeStr := r.State.GameStartTime.In(timeutil.Beijing).Format("20060102_150405")
	if r.State.GameStartTime.IsZero() {
		startTimeStr = timeutil.Now().Format("20060102_150405")
	}
	fileName := fmt.Sprintf("%s_%s.json", r.ID, startTimeStr)
	return path.Join("./game_logs", fileName)
}

func WriteGameLog(r *domain.Room, playerID string, roomInfo *entities.RoomInfo, msg map[string]interface{}) {
	go func() {
		logPath := getGameLogFilePath(r)
		if err := os.MkdirAll(path.Dir(logPath), 0755); err != nil {
			log.Println("❌ 创建日志目录失败:", err)
			return
		}
		entry := map[string]interface{}{
			"timestamp":  timeutil.Now().Format("2006-01-02 15:04:05"),
			"result":     msg["result"],
			"roomInfo":   roomInfo,
			"playerID":   playerID,
			"playerData": msg["playerData"],
			"roomData":   msg["roomData"],
			"tempData":   msg["tempData"],
		}
		jsonEntry, err := json.Marshal(entry)
		if err != nil {
			log.Println("❌ 序列化日志 entry 失败:", err)
			return
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Println("❌ 打开游戏日志文件失败:", err)
			return
		}
		defer f.Close()
		jsonEntry = append(jsonEntry, ',')
		if _, err := f.Write(jsonEntry); err != nil {
			log.Println("❌ 写入日志失败:", err)
			return
		}
		f.Write([]byte("\n"))
	}()
}

func buildRoomInfo(r *domain.Room) *entities.RoomInfo {
	return &entities.RoomInfo{
		RoomStatus: r.State.RoomStatus != domain.RoomStatusMatch,
		GameStatus: entities.RoomStatus(string(r.State.RoomStatus)),
		MaxPlayers: r.State.MaxPlayers,
		UserID:     r.State.OwnerID,
	}
}

// SyncRoomMessage 给单个客户端发送同步消息（结构与旧版兼容）。
func SyncRoomMessage(conn domain.WriteOnlyConn, r *domain.Room, pc *domain.PlayerConn) error {
	revealedCards := map[int][]entities.NormalCard{}
	for _, c := range r.State.NormalCards {
		if c.State == entities.CardStateRevealed {
			revealedCards[c.Level] = append(revealedCards[c.Level], *c)
		}
	}

	revealedNobles := make([]entities.NobleCard, 0)
	for _, n := range r.State.NobleCards {
		if n.State == entities.CardStateRevealed {
			revealedNobles = append(revealedNobles, *n)
		}
	}

	playersData := make(map[string]dto.SplendorPlayerData, len(r.State.Players))
	for pid, ps := range r.State.Players {
		if ps == nil {
			continue
		}
		playersData[pid] = dto.SplendorPlayerData{
			NormalCard:  ps.NormalCard,
			NobleCard:   ps.NobleCard,
			Gem:         ps.Gem,
			Score:       ps.Score,
			ReserveCard: ps.ReserveCard,
		}
	}

	roomInfo := buildRoomInfo(r)

	turnDeadline := interface{}(nil)
	if !r.Base.TurnDeadline.IsZero() {
		turnDeadline = r.Base.TurnDeadline.UTC().Format(time.RFC3339Nano)
	}
	turnTimeoutMs := int64(r.Base.TurnTimeout / time.Millisecond)

	msg := map[string]interface{}{
		"type":       "sync",
		"playerId":   pc.PlayerID,
		"playerData": playersData,
		"roomData": map[string]interface{}{
			"card":          revealedCards,
			"gems":          r.State.Gems,
			"nobles":        revealedNobles,
			"roomInfo":      roomInfo,
			"currentPlayer": r.State.CurrentPlayer,
			"lastData":      r.State.LastData,
			"turnDeadline":  turnDeadline,
			"turnTimeoutMs": turnTimeoutMs,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("❌ 编码 JSON 失败: %w", err)
	}

	if pc.PlayerID == r.State.CurrentPlayer {
		WriteGameLog(r, pc.PlayerID, roomInfo, msg)
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

// BroadcastToRoom 仅遍历发送（score/状态切换的副作用挪到 HandleBuyCardMessage 末尾）。
func BroadcastToRoom(r *domain.Room) {
	for _, pc := range r.Connections {
		if !pc.Online || pc.Conn == nil {
			continue
		}
		if err := SyncRoomMessage(pc.Conn, r, pc); err != nil {
			log.Printf("广播失败，关闭连接 player=%s err=%v", pc.PlayerID, err)
			pc.Conn.Close()
		}
	}
}

// BroadcastToMatch 大厅阶段同步。
func BroadcastToMatch(r *domain.Room) {
	playersInfo := make(map[string]map[string]interface{}, len(r.Connections))
	for _, p := range r.Connections {
		playersInfo[p.PlayerID] = map[string]interface{}{
			"playerID": p.PlayerID,
			"online":   p.Online,
			"ai":       p.AI,
			"ready":    p.Ready,
		}
	}

	for _, pc := range r.Connections {
		if !pc.Online || pc.Conn == nil {
			continue
		}
		msg := map[string]interface{}{
			"type":     "MATCH_SYNC",
			"roomID":   r.ID,
			"ownerID":  r.State.OwnerID,
			"status":   r.State.RoomStatus,
			"playerID": pc.PlayerID,
			"players":  playersInfo,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		if err := pc.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("广播失败 player=%s err=%v", pc.PlayerID, err)
			pc.Conn.Close()
			pc.Online = false
		}
	}
}

// SwitchToNextPlayer 纯内存切换 r.State.CurrentPlayer 到 PlayerSeq 中的下一个。
func SwitchToNextPlayer(r *domain.Room) {
	if len(r.PlayerSeq) == 0 {
		return
	}
	current := r.State.CurrentPlayer
	idx := -1
	for i, pid := range r.PlayerSeq {
		if pid == current {
			idx = i
			break
		}
	}
	if idx < 0 {
		r.State.CurrentPlayer = r.PlayerSeq[0]
		return
	}
	next := r.PlayerSeq[(idx+1)%len(r.PlayerSeq)]
	r.State.CurrentPlayer = next
}
