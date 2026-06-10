package roompkg

import (
	"fmt"
	"sort"
	"time"

	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/data"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/domain"
	"github.com/nciyuan9264/game-backend/internal/games/acquire/domain/game"
	"github.com/nciyuan9264/game-backend/pkg/gamehistory"
	"github.com/nciyuan9264/game-backend/pkg/logger"
	"github.com/nciyuan9264/game-backend/pkg/roomcore"
)

type RoomService struct {
	Room *domain.Room
	svc  *roomcore.Service[*RoomService]

	// History recording fields.
	Recorder         *gamehistory.Recorder
	HistorySeq       int
	HistoryStartedAt time.Time
	HistoryEnded     bool
}

const MaxPlayers = 6

var Rooms = roomcore.NewRegistry[*RoomService]()

func (r *RoomService) Run() {
	// 创建宽限期：60s 内无人连接 ws 则自动删除房间，兜底「创建后从未连接」的孤儿房。
	roomcore.StartCreateGrace(r.svc)
	for {
		select {
		case cmd := <-r.Room.CmdCh:
			r.handleCommand(cmd)

			// 开局：MatchBegin 处理后 RoomStatus 切到 Waiting，启动 recorder。
			// 但 startRecording 需要 GameStartTime 已经被赋值，所以放在 game_ready 全员到齐之后更稳妥。
			// 使用 RoomStatusSetTile（handleAllReady 后切到 SetTile）作为开局信号；幂等保护见 startRecording。
			if r.Recorder == nil && !r.HistoryEnded && r.Room.State.RoomStatus != domain.RoomStatusMatch &&
				r.Room.State.RoomStatus != domain.RoomStatusWaiting && !r.Room.State.GameStartTime.IsZero() {
				r.startRecording()
			}

			// 命令落库（白名单内才记录）。
			r.recordEvent(cmd)

			// 命令处理完毕后，根据当前 RoomStatus 重新启动/清空思考超时定时器。
			roomcore.RearmThinkTimer(r.svc)

			if r.Room.State.RoomStatus == domain.RoomStatusMatch {
				game.BroadcastToMatch(r.Room)
			} else {
				game.BroadcastToRoom(r.Room)
				if r.Room.State.RoomStatus == domain.RoomStatusEnd && r.Recorder != nil {
					r.finishRecording()
				}
			}
		case <-r.Room.QuitCh:
			roomcore.StopThinkTimer(r.svc)
			r.stopRecording()
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
		AutoSettleDisconnectedHolder(r.Room, cmd.PlayerID)
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
	case "game_place_tile":
		game.HandlePlaceTileMessage(r.Room, cmd)
	case "game_create_company":
		game.HandleCreateCompanyMessage(r.Room, cmd)
	case "game_merging_settle":
		game.HandleMergingSettleMessage(r.Room, cmd)
	case "game_buy_stock":
		game.HandleBuyStockMessage(r.Room, cmd)
	case "game_merging_selection":
		game.HandleMergingSelectionMessage(r.Room, cmd)
	case "turn_timeout":
		game.HandleTurnTimeoutMessage(r.Room, cmd)
	case "game_play_audio":
		game.HandlePlayAudioMessage(r.Room, cmd)
	case "game_restart_game":
		// 重开局：先把上一局的 recorder 关掉，重置历史状态，再让 handler 重置 game state。
		r.stopRecording()
		r.HistoryEnded = false
		r.HistorySeq = 0
		r.HistoryStartedAt = time.Time{}
		game.HandleRestartGameMessage(r.Room, cmd)
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

	// 初始化公司数据
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

	// 初始化游戏棋盘（12x9 个 tile）
	for col := 1; col <= 12; col++ {
		for row := 'A'; row <= 'I'; row++ {
			id := fmt.Sprintf("%d%c", col, row)
			tile := &domain.Tile{
				ID:     id,
				Belong: "",
			}
			r.State.BoardTiles[id] = tile
		}
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

// handleAllReady acquire 的全员到齐回调：sort.Strings 选第一个 + 切到 SetTile。
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
		r.State.RoomStatus = domain.RoomStatusSetTile
	}
}
