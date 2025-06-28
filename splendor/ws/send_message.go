package ws

import (
	"encoding/json"
	"fmt"
	"go-game/dto"
	"go-game/entities"
	"go-game/repository"
	"log"
	"os"
	"path"
	"time"

	"github.com/gorilla/websocket"
)

func WriteGameLog(roomID, playerID string, roomInfo *entities.RoomInfo, msg map[string]interface{}) {
	go func() {
		logPath := getGameLogFilePath(roomID)

		// 确保目录存在
		if err := os.MkdirAll(path.Dir(logPath), 0755); err != nil {
			log.Println("❌ 创建日志目录失败:", err)
			return
		}

		entry := map[string]interface{}{
			"timestamp":  time.Now().Format("2006-01-02 15:04:05"),
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
		if _, err := f.Write([]byte("\n")); err != nil {
			log.Println("❌ 写入换行失败:", err)
		}
	}()
}

// 向该客户端发送同步消息
func SyncRoomMessage(conn dto.ConnInterface, roomID string, playerID string) error {
	currentPlayer, err := GetCurrentPlayer(repository.Rdb, repository.Ctx, roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取当前玩家失败: %w", err)
	}

	roomInfo, err := GetRoomInfo(roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取房间信息失败: %w", err)
	}

	allCards, err := GetAllNormalCards(roomID)
	if err != nil {
		return fmt.Errorf("❌ 获取所有卡牌失败: %w", err)
	}

	revealedCards := map[int][]entities.NormalCard{}
	for _, card := range allCards {
		if card.State == entities.CardStateRevealed {
			revealedCards[card.Level] = append(revealedCards[card.Level], card)
		}
	}

	playersData := make(map[string]dto.SplendorPlayerData)
	for _, pc := range Rooms[roomID] {
		playerNormalCard, _ := GetPlayerNormalCard(roomID, pc.PlayerID)
		playerGem, _ := GetPlayerGem(roomID, pc.PlayerID)
		playerScore, _ := GetPlayerScore(roomID, pc.PlayerID)
		reserveCards, _ := GetPlayerReserveCards(roomID, pc.PlayerID)
		nobleCard, _ := GetPlayerNobleCard(roomID, pc.PlayerID)
		playerInfo := dto.SplendorPlayerData{
			NormalCard:  playerNormalCard,
			NobleCard:   nobleCard,
			Gem:         playerGem,
			Score:       playerScore,
			ReserveCard: reserveCards,
		}
		playersData[pc.PlayerID] = playerInfo
	}
	allNobles, _ := GetAllNobleCards(roomID)
	allGems, _ := GetGemCounts(roomID)

	revealedNobles := make([]entities.NobleCard, 0)
	for _, noble := range allNobles {
		if noble.State == entities.CardStateRevealed {
			revealedNobles = append(revealedNobles, noble)
		}
	}
	// ------- 组装消息 -------
	msg := map[string]interface{}{
		"type":       "sync",
		"playerId":   playerID,
		"playerData": playersData,
		"roomData": map[string]interface{}{
			"card":          revealedCards,
			"gems":          allGems,
			"nobles":        revealedNobles,
			"roomInfo":      roomInfo,
			"currentPlayer": currentPlayer,
		},
	}

	// ------- 发送 WebSocket 消息 -------
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("❌ 编码 JSON 失败: %w", err)
	}
	if playerID == currentPlayer {
		WriteGameLog(roomID, playerID, roomInfo, msg)
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// 广播消息给房间内所有连接成功的玩家
func BroadcastToRoom(roomID string) {
	currentPlayer, err := GetCurrentPlayer(repository.Rdb, repository.Ctx, roomID)
	if err != nil {
		log.Println("获取当前玩家失败:", err)
	}
	firstPlayer, err := GetFirstPlayer(repository.Rdb, repository.Ctx, roomID)
	if err != nil {
		log.Println("获取第一个玩家失败:", err)
	}

	roomInfo, err := GetRoomInfo(roomID)
	if err != nil {
		log.Println("获取房间信息失败:", err)
	}
	for _, pc := range Rooms[roomID] {
		playerScore := 0
		playerNormalCard, err := GetPlayerNormalCard(roomID, pc.PlayerID)
		if err != nil {
			log.Println("获取玩家卡牌失败:", err)
		}
		playerNobleCard, err := GetPlayerNobleCard(roomID, pc.PlayerID)
		if err != nil {
			log.Println("获取玩家贵族卡失败:", err)
		}
		for _, noble := range playerNobleCard {
			playerScore += noble.Points
		}
		for _, card := range playerNormalCard {
			playerScore += card.Points
		}
		if err := SetPlayerScore(roomID, pc.PlayerID, playerScore); err != nil {
			log.Println("设置分数失败:", err)
		}

		if playerScore >= 15 && roomInfo.GameStatus == entities.RoomStatusPlaying {
			if currentPlayer != firstPlayer {
				err := SetGameStatus(repository.Rdb, roomID, entities.RoomStatusLastTurn)
				if err != nil {
					log.Println("设置游戏状态失败:", err)
				}
			} else {
				err := SetGameStatus(repository.Rdb, roomID, entities.RoomStatusEnd)
				if err != nil {
					log.Println("设置游戏状态失败:", err)
				}
			}
		}
	}

	if currentPlayer == firstPlayer && roomInfo.GameStatus == entities.RoomStatusLastTurn {
		err := SetGameStatus(repository.Rdb, roomID, entities.RoomStatusEnd)
		if err != nil {
			log.Println("设置游戏状态失败:", err)
		}
	}

	for _, pc := range Rooms[roomID] {
		if pc.Online {
			// 尝试发送消息
			if err := SyncRoomMessage(pc.Conn, roomID, pc.PlayerID); err != nil {
				log.Println("广播失败，移除连接:", pc.PlayerID)
				pc.Conn.Close()
			}
		}
	}
}
