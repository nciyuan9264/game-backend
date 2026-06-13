package roompkg

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/data"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/davinci/domain/game"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

type RoomService struct {
	Room *domain.Room
	svc  *roomcore.Service[*RoomService]

	HistoryStartedAt time.Time
	HistoryEnded     bool
}

const MaxPlayers = domain.DefaultMaxPlayers

var Rooms = roomcore.NewRegistry[*RoomService]()

func (r *RoomService) Run() {
	// 周期健康检查：从创建到游戏结束，每 60s 检测一次，连续 3 次无真人则删除房间。
	roomcore.StartHealthCheck(r.svc)
	for {
		select {
		case cmd := <-r.Room.CmdCh:
			r.handleCommand(cmd)
			roomcore.RearmThinkTimer(r.svc)
			if r.Room.State.RoomStatus == domain.RoomStatusMatch {
				game.BroadcastToMatch(r.Room)
			} else {
				game.BroadcastToRoom(r.Room)
				if r.Room.State.RoomStatus == domain.RoomStatusEnd && !r.HistoryEnded {
					r.finishHistory()
				}
			}
		case <-r.Room.Base.HealthTickChan():
			roomcore.HandleHealthTick(r.svc)
		case <-r.Room.QuitCh:
			roomcore.StopHealthCheck(r.svc)
			roomcore.StopThinkTimer(r.svc)
			r.finishAbandonedHistory(time.Now())
			return
		}
	}
}

// handleCommand 处理房间命令
func (r *RoomService) handleCommand(cmd domain.Command) {
	switch cmd.Type {
	case "connect":
		roomcore.HandleConnect(r.svc, cmd)
	case "disconnect":
		roomcore.HandleDisconnect(r.svc, cmd)
	case "match_ready":
		roomcore.HandleMatchReady(r.svc, cmd)
	case "match_begin":
		HandleMatchBegin(r.Room, cmd)
	case "match_add_ai":
		roomcore.HandleAddAI(r.svc, cmd)
	case "match_remove_player":
		roomcore.HandleRemovePlayer(r.svc, cmd)
	case "game_ready":
		roomcore.HandleReadyMessage(r.svc, cmd)
	case "game_get_card":
		game.HandleGetCardMessage(r.Room, cmd)
	case "game_guess_card":
		game.HandleGuessCardMessage(r.Room, cmd)
	case "game_set_card":
		game.HandleSetCardMessage(r.Room, cmd)
	case "turn_timeout":
		game.HandleTurnTimeoutMessage(r.Room, cmd)
	case "game_play_audio":
		game.HandlePlayAudioMessage(r.Room, cmd)
	case "game_restart_game":
		game.HandleRestartGameMessage(r.Room, cmd)
		r.resetHistoryState()
	default:
		logger.Warn("未知命令类型", logger.F("command_type", cmd.Type))
	}
}

func HandleMatchBegin(r *domain.Room, cmd domain.Command) {
	// 检查是否所有玩家都已准备
	allReady := true
	for _, pc := range r.Connections {
		if !pc.Ready {
			allReady = false
			break
		}
	}

	if !allReady {
		logger.Error("不是所有玩家都已准备", logger.F("room_id", r.ID))
		return
	}

	r.State.RoomStatus = domain.RoomStatusWaiting
	r.State.MaxPlayers = len(r.Connections)
	// 初始化牌组
	type TempCard struct {
		Color domain.Color
		Num   domain.CardNumber
	}

	tempCards := make([]TempCard, 0, 26)

	// 添加白色牌（12张，数字-1到11）
	for num := domain.NumMinus1; num <= domain.Num11; num++ {
		tempCards = append(tempCards, TempCard{
			Color: domain.ColorWhite,
			Num:   num,
		})
	}

	// 添加黑色牌（12张，数字-1到11）
	for num := domain.NumMinus1; num <= domain.Num11; num++ {
		tempCards = append(tempCards, TempCard{
			Color: domain.ColorBlack,
			Num:   num,
		})
	}

	// 随机打乱临时牌组
	rand.Shuffle(len(tempCards), func(i, j int) {
		tempCards[i], tempCards[j] = tempCards[j], tempCards[i]
	})

	// 创建最终的卡片，ID从0-23固定
	cards := make([]*domain.Card, 0, 24)
	for cardID, temp := range tempCards {
		cards = append(cards, &domain.Card{
			ID:         fmt.Sprintf("%d", cardID),
			Color:      temp.Color,
			Num:        temp.Num,
			IsRevealed: false,
			Index:      -1,
		})
	}

	r.State.BoardCards = make(map[string]*domain.Card)
	for _, card := range cards {
		r.State.BoardCards[card.ID] = card
	}

	for playerID := range r.Connections {
		data.InitPlayerData(r, playerID)
		logger.Info("玩家进入游戏", logger.F("room_id", r.ID), logger.F("player_id", playerID))
	}

	// 更新房间状态为匹配中
	r.State.RoomStatus = domain.RoomStatusWaiting

	// 开始游戏
	logger.Info("所有玩家都已准备，开始游戏", logger.F("room_id", r.ID))
}

// handleAllReady davinci 的全员到齐回调：sort.Strings 选第一个 + 根据 BoardCards 决定下一阶段。
func handleAllReady(r *domain.Room, cmd domain.Command) {
	r.State.GameStartTime = time.Now()

	if r.State.CurrentPlayer == "" {
		if len(r.Connections) == 0 {
			logger.Error("房间中没有玩家，无法设置当前玩家", logger.F("room_id", r.ID))
			return
		}

		playerIDs := make([]string, 0, len(r.Connections))
		for pid, pc := range r.Connections {
			if pc != nil {
				playerIDs = append(playerIDs, pid)
			}
		}
		if len(playerIDs) == 0 {
			logger.Error("没有在线玩家，无法设置当前玩家", logger.F("room_id", r.ID))
			return
		}
		sort.Strings(playerIDs)
		firstPlayerID := playerIDs[0]
		r.State.CurrentPlayer = firstPlayerID
	}

	if r.State.RoomStatus == domain.RoomStatusWaiting {
		logger.Info("所有玩家进入游戏，开始游戏", logger.F("room_id", r.ID), logger.F("player_id", cmd.PlayerID))
		if len(r.State.BoardCards) == 0 {
			r.State.RoomStatus = domain.RoomStatusGuessCard
		} else {
			r.State.RoomStatus = domain.RoomStatusGetCard
		}
	}
}
