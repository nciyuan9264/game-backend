package game

import (
	"encoding/json"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/utils"
	"sort"
	"time"

	"github.com/gorilla/websocket"
)

func SwitchToNextPlayer(r *domain.Room, currentPlayerID string) error {
	if r == nil {
		return fmt.Errorf("房间为空")
	}

	if len(r.Connections) == 0 {
		return fmt.Errorf("房间 %s 没有玩家", r.ID)
	}

	ids := make([]string, 0, len(r.Connections))
	for pid, pc := range r.Connections {
		if pc != nil {
			ids = append(ids, pid)
		}
	}

	if len(ids) == 0 {
		return fmt.Errorf("房间 %s 没有在线玩家", r.ID)
	}

	sort.Strings(ids)

	// 找当前索引
	curIdx := -1
	for i, pid := range ids {
		if pid == currentPlayerID {
			curIdx = i
			break
		}
	}

	// 计算下一个玩家
	var nextPlayerID string
	if curIdx >= 0 {
		nextPlayerID = ids[(curIdx+1)%len(ids)]
	} else {
		nextPlayerID = ids[0]
	}

	r.State.CurrentPlayer = nextPlayerID

	utils.Info(
		"房间当前玩家切换",
		utils.F("current_player_id", currentPlayerID),
		utils.F("room_id", r.ID),
		utils.F("next_player_id", nextPlayerID),
	)

	return nil
}

type PlayAudioPayload struct {
	AudioType string `json:"audioType"`
}

func HandlePlayAudioMessage(r *domain.Room, cmd domain.Command) {
	var p PlayAudioPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		utils.Error("消息格式错误", utils.F("room_id", r.ID), utils.F("player_id", cmd.PlayerID), utils.F("error", err))
		return
	}
	audioType := p.AudioType

	msg := map[string]interface{}{
		"type":    "audio",
		"message": audioType,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		utils.Error("编码 JSON 失败", utils.F("error", err))
		return
	}

	for _, pc := range r.Connections {
		if pc.Online && pc.Conn != nil {
			err := pc.Conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				utils.Error("向玩家 %s 发送音频消息失败", utils.F("player_id", pc.PlayerID), utils.F("error", err))
			}
		}
	}
}

func HandleRestartGameMessage(r *domain.Room, cmd domain.Command) {
	// 重置上次落子
	r.State.LastTileKey = ""
	// 重置游戏状态
	r.State.RoomStatus = domain.RoomStatusMatch
	// 重置tiles
	for tileKey, tileInfo := range r.State.BoardTiles {
		tileInfo.Belong = ""
		r.State.BoardTiles[tileKey] = tileInfo
	}
	for _, pc := range r.Connections {
		playerID := pc.PlayerID
		// 2. 设置初始资金
		playerTiles, err := data.GenerateAvailableTiles(r, 5)
		if err != nil {
			utils.Error("生成可用 tile 失败", utils.F("error", err))
			return
		}

		r.State.Players[playerID] = &domain.PlayerState{
			Money: 6000,
			Tiles: playerTiles,
			Stocks: map[string]int{
				"Sackson":     0,
				"Tower":       0,
				"American":    0,
				"Festival":    0,
				"Worldwide":   0,
				"Continental": 0,
				"Imperial":    0,
			},
		}
	}

	r.State.Companies = map[string]*domain.CompanyState{
		"Sackson": {
			Name:       "Sackson",
			StockTotal: 25,
			Tiles:      0,   // 初始数量
			StockPrice: 200, // 初始参考股价（可调整）
		},
		"Tower": {
			Name:       "Tower",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"American": {
			Name:       "American",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"Festival": {
			Name:       "Festival",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"Worldwide": {
			Name:       "Worldwide",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"Continental": {
			Name:       "Continental",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
		"Imperial": {
			Name:       "Imperial",
			Tiles:      0, // 初始数量
			StockTotal: 25,
			StockPrice: 200,
		},
	}
	r.State.GameStartTime = time.Time{}
}
