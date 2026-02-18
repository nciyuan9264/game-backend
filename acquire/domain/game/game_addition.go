package game

import (
	"context"
	"encoding/json"
	"fmt"
	"go-game/domain/data"
	"go-game/domain/domain"
	"go-game/dto"
	"go-game/repository"
	"go-game/utils"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

func SwitchToNextPlayer(
	rdb *redis.Client,
	ctx context.Context,
	r *domain.Room,
	currentPlayerID string,
) error {
	if r == nil || len(r.Players) == 0 {
		return fmt.Errorf("房间 %s 没有玩家", r.ID)
	}

	// 基于 Players 构建稳定且可重复的循环顺序：按 playerID 排序，仅保留在线玩家
	ids := make([]string, 0, len(r.Players))
	for pid, pc := range r.Players {
		if pc != nil {
			ids = append(ids, pid)
		}
	}
	if len(ids) == 0 {
		return fmt.Errorf("房间 %s 没有在线玩家", r.ID)
	}
	sort.Strings(ids)

	// 找到当前玩家在排序序列中的位置，不存在则视为 -1
	curIdx := -1
	for i, pid := range ids {
		if pid == currentPlayerID {
			curIdx = i
			break
		}
	}

	// 计算下一个玩家（循环），若未找到当前玩家则从序列首位开始
	var nextPlayerID string
	if curIdx == -1 {
		nextPlayerID = ids[0]
	} else {
		nextPlayerID = ids[(curIdx+1)%len(ids)]
	}

	if nextPlayerID == "" {
		return fmt.Errorf("无法确定下一个玩家")
	}

	if _, ok := r.Players[nextPlayerID]; !ok {
		return fmt.Errorf("下一个玩家 %s 不存在", nextPlayerID)
	}

	if err := data.SetCurrentPlayer(rdb, ctx, r.ID, nextPlayerID); err != nil {
		return fmt.Errorf("切换当前玩家失败: %w", err)
	}

	utils.Info("房间 %s 当前玩家切换为: %s", utils.F("room_id", r.ID), utils.F("next_player_id", nextPlayerID))
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

	for _, pc := range r.Players {
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
	if err := data.SetLastTileKey(repository.Rdb, repository.Ctx, r.ID, cmd.PlayerID, ""); err != nil {
		utils.Error("设置最后放置的 tile 失败", utils.F("error", err))
		return
	}
	// 重置游戏状态
	r.Status = dto.RoomStatusMatch
	// 重置tiles
	tile, err := data.GetAllRoomTiles(repository.Rdb, r.ID)
	if err != nil {
		utils.Error("获取所有 tile 失败", utils.F("error", err))
		return
	}
	for tileKey, tileInfo := range tile {
		tileInfo.Belong = ""
		tile[tileKey] = tileInfo
	}
	err = data.SetAllRoomTiles(repository.Rdb, r.ID, tile)
	if err != nil {
		utils.Error("重置 tile 失败", utils.F("error", err))
		return
	}

	for _, pc := range r.Players {
		playerID := pc.PlayerID
		// 2. 设置初始资金
		err = data.SetPlayerInfoField(repository.Rdb, repository.Ctx, r.ID, playerID, "money", 6000)
		if err != nil {
			utils.Error("设置玩家信息失败", utils.F("player_id", playerID), utils.F("error", err))
			return
		}

		allTiles, err := data.GenerateAvailableTiles(r)
		if err != nil {
			utils.Error("生成可用 tile 失败", utils.F("error", err))
			return
		}
		rand.Shuffle(len(allTiles), func(i, j int) { allTiles[i], allTiles[j] = allTiles[j], allTiles[i] })
		playerTiles := utils.SafeSlice(allTiles, 5)
		err = data.SetPlayerTiles(repository.Rdb, repository.Ctx, r.ID, playerID, playerTiles)
		if err != nil {
			utils.Error("添加 tile 失败", utils.F("player_id", playerID), utils.F("error", err))
			return
		}
		companyIDs, err := data.GetCompanyIDs(r.ID)
		if err != nil {
			utils.Error("获取公司ID失败", utils.F("error", err))
			return
		}
		// 3.2 初始化玩家股票为0
		playerStocks := make(map[string]int)
		for _, company := range companyIDs {
			playerStocks[company] = 0
		}
		err = data.SetPlayerStocks(repository.Rdb, repository.Ctx, r.ID, playerID, playerStocks)
		if err != nil {
			utils.Error("写入玩家股票失败", utils.F("player_id", playerID), utils.F("error", err))
			return
		}
	}

	companyData := map[string]map[string]interface{}{
		"Sackson": {
			"name":       "Sackson",
			"stockTotal": 25,
			"tiles":      0,   // 初始数量
			"stockPrice": 200, // 初始参考股价（可调整）
		},
		"Tower": {
			"name":       "Tower",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"American": {
			"name":       "American",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Festival": {
			"name":       "Festival",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Worldwide": {
			"name":       "Worldwide",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Continental": {
			"name":       "Continental",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
		"Imperial": {
			"name":       "Imperial",
			"tiles":      0, // 初始数量
			"stockTotal": 25,
			"stockPrice": 200,
		},
	}

	for id, data := range companyData {
		companyKey := fmt.Sprintf("room:%s:company:%s", r.ID, id)
		if _, err := repository.Rdb.HSet(repository.Ctx, companyKey, data).Result(); err != nil {
			return
		}
		repository.Rdb.SAdd(repository.Ctx, fmt.Sprintf("room:%s:company_ids", r.ID), id)
	}
	startKey := fmt.Sprintf("room:%s:game_start_time", r.ID)
	repository.Rdb.Set(repository.Ctx, startKey, time.Now().Format("20060102_150405"), 0)
}
